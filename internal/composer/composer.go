package composer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/SWC-GEKO/beaver/internal/docker"
	"github.com/compose-spec/compose-go/v2/types"
)

type Composer struct {
	Projects map[string]types.Project
	Config

	mu sync.Mutex
}

type Config struct {
	GlobalNet             string
	NatsImage             string
	RouterImage           string
	GlobalNatsServiceName string
	GlobalNatsStream      string
}

func (c *Composer) Add(f *docker.Function) error {
	localBaseTopic := "function."

	localNet := types.NetworkConfig{
		Name:   "local-net",
		Driver: "bridge",
	}

	globalNet := types.NetworkConfig{
		Name:     c.GlobalNet,
		External: true,
	}

	natsVolume := types.VolumeConfig{
		Name: "nats",
	}

	services := make(map[string]types.ServiceConfig)

	services["local-nats"] = types.ServiceConfig{
		Name:          "local-nats",
		ContainerName: "local-nats",
		Environment:   nil, // TODO: check if needed
		Image:         c.NatsImage,
		Command:       types.ShellCommand{"-js", "-sd /data"},
		Networks: map[string]*types.ServiceNetworkConfig{
			"local-net": {},
		},
		Restart: "unless-stopped",
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:     "volume",
				Source:   "nats",
				Target:   "/data",
				ReadOnly: false,
			},
		},
	}

	processors := make([]types.ServiceConfig, f.Replication)
	for i := 0; i < f.Replication; i++ {
		name := fmt.Sprintf("processor-%d", i)

		subTopics := calcTopics(f.MaxShards, i, f.Replication, localBaseTopic)

		processorEnv := types.NewMappingWithEquals([]string{
			fmt.Sprintf("NAME=%s", name),
			fmt.Sprintf("NATS_ADDR=nats://%s:4222", services["nats"].Name),
			fmt.Sprintf("SUB_TOPICS=%s", strings.Join(subTopics, ",")),
			fmt.Sprintf("PUB_TOPIC=%s.out", localBaseTopic),
			fmt.Sprintf("DLQ_TOPIC=%s.dlq", localBaseTopic),
		})

		processors[i] = types.ServiceConfig{
			Name:          name,
			ContainerName: name,
			DependsOn: types.DependsOnConfig{
				"local-nats": types.ServiceDependency{
					Condition: "service_started",
				},
			},
			Environment: processorEnv,
			Image:       f.ImageTag,
			Networks: map[string]*types.ServiceNetworkConfig{
				"local-net": {},
			},
			Restart: "unless-stopped",
		}

		services[name] = processors[i]
	}

	dependencies := make(map[string]types.ServiceDependency)
	for _, p := range processors {
		dependencies[p.Name] = types.ServiceDependency{
			Condition: "service_started",
		}
	}
	dependencies["local-nats"] = types.ServiceDependency{
		Condition: "service_started",
	}

	routerEnv := types.NewMappingWithEquals(
		[]string{
			fmt.Sprintf("NAME=%s-router", f.UniqueName),
			fmt.Sprintf("GLOBAL_NATS=nats://%s:4222", c.GlobalNatsServiceName),
			fmt.Sprintf("GLOBAL_STREAM=%s", c.GlobalNatsStream),
			fmt.Sprintf("GLOBAL_TOPIC=%s.%s", c.GlobalNatsStream, f.UniqueName),
			fmt.Sprintf("LOCAL_NATS=nats://%s:4222", services["nats"].Name),
			fmt.Sprintf("LOCAL_TOPIC=%s", localBaseTopic),
			fmt.Sprintf("SHARDS=%s", strconv.Itoa(f.MaxShards)),
		},
	)

	services["router"] = types.ServiceConfig{
		Name:          "router",
		ContainerName: "router",
		DependsOn:     dependencies,
		Environment:   routerEnv,
		Image:         c.RouterImage,
		Networks: map[string]*types.ServiceNetworkConfig{
			"global-net": {},
			"local-net":  {},
		},
		Restart: "unless-stopped",
	}

	project := types.Project{
		Name:     f.UniqueName,
		Services: services,
		Networks: map[string]types.NetworkConfig{
			"local-net":  localNet,
			"global-net": globalNet,
		},
		Volumes: map[string]types.VolumeConfig{
			"nats": natsVolume,
		},
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.Projects[f.UniqueName]; ok {
		return errors.New("function with uniqueName already exists in map")
	}
	c.Projects[f.UniqueName] = project
	return nil
}

func (c *Composer) Del(uniqueName string) error {
	panic("implement me...")
}

func (c *Composer) Up(uniqueName string) error {
	panic("implement me...")
}

func (c *Composer) Down(uniqueName string) error {
	panic("implement me...")
}

func calcTopics(shards, currReplicaCount, totalReplicas int, baseTopic string) []string {
	return nil
}
