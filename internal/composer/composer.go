package composer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/SWC-GEKO/beaver/internal/docker"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
)

type Composer struct {
	Projects map[string]types.Project
	Config
	Service api.Compose

	mu sync.Mutex
}

type Config struct {
	GlobalNet             string
	NatsImage             string
	RouterImage           string
	GlobalNatsServiceName string
	GlobalNatsStream      string
}

func DefaultConfig() Config {
	return Config{
		GlobalNet:             "global-net",
		NatsImage:             "nats:latest",
		RouterImage:           "stateless-router:latest",
		GlobalNatsServiceName: "nats-global",
		GlobalNatsStream:      "FUNCTIONS",
	}
}

func NewComposer(config Config) (*Composer, error) {
	dockerCli, err := command.NewDockerCli()
	if err != nil {
		return nil, err
	}

	log.Printf("DockerCLI: %+v", dockerCli)

	opts := flags.NewClientOptions()
	log.Printf("ClientOptions: %+v", opts)

	if err = dockerCli.Initialize(opts); err != nil {
		return nil, err
	}

	service := compose.NewComposeService(dockerCli)

	return &Composer{
		Projects: make(map[string]types.Project),
		Config:   config,
		Service:  service,
		mu:       sync.Mutex{},
	}, nil
}

func (c *Composer) Add(f *docker.Function) error {
	// TODO: implement functionality that users can have their own env-vars
	localBaseTopic := "function"

	localNetName := fmt.Sprintf("%s-local-net", f.UniqueName)
	localNet := types.NetworkConfig{
		Name:   localNetName,
		Driver: "bridge",
	}

	globalNet := types.NetworkConfig{
		Name:     c.GlobalNet,
		External: true,
	}

	natsVolumeName := fmt.Sprintf("%s-nats-vol", f.UniqueName)
	natsVolume := types.VolumeConfig{
		Name: natsVolumeName,
	}

	services := make(map[string]types.ServiceConfig)

	localNatsName := fmt.Sprintf("%s-local-nats", f.UniqueName)
	services[localNatsName] = types.ServiceConfig{
		Name:          localNatsName,
		ContainerName: localNatsName,
		Environment:   nil, // TODO: check if needed
		Image:         c.NatsImage,
		Command:       types.ShellCommand{"-js"},
		Networks: map[string]*types.ServiceNetworkConfig{
			localNetName: {},
		},
		Restart: "unless-stopped",
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:     "volume",
				Source:   natsVolumeName,
				Target:   "/data",
				ReadOnly: false,
			},
		},
	}

	processors := make([]types.ServiceConfig, f.Replication)
	for i := 0; i < f.Replication; i++ {
		name := fmt.Sprintf("%s-processor-%d", f.UniqueName, i)

		subTopics := calcTopics(f.MaxShards, i, f.Replication, localBaseTopic)

		processorEnv := types.NewMappingWithEquals([]string{
			fmt.Sprintf("NAME=%s", name),
			fmt.Sprintf("NATS_ADDR=nats://%s:4222", services[localNatsName].Name),
			fmt.Sprintf("SUB_TOPICS=%s", strings.Join(subTopics, ",")),
			fmt.Sprintf("PUB_TOPIC=%s.out", localBaseTopic),
			fmt.Sprintf("DLQ_TOPIC=%s.dlq", localBaseTopic),
		})

		processors[i] = types.ServiceConfig{
			Name:          name,
			ContainerName: name,
			DependsOn: types.DependsOnConfig{
				// Does this reference the service in the compose or the container-name?
				localNatsName: types.ServiceDependency{
					Condition: "service_started",
				},
			},
			Environment: processorEnv,
			Image:       f.ImageTag,
			Networks: map[string]*types.ServiceNetworkConfig{
				localNetName: {},
			},
			Restart: "unless-stopped",
		}

		services[name] = processors[i]
	}

	routersDependencies := make(map[string]types.ServiceDependency)
	for _, p := range processors {
		routersDependencies[p.Name] = types.ServiceDependency{
			Condition: "service_started",
		}
	}
	routersDependencies[localNatsName] = types.ServiceDependency{
		Condition: "service_started",
	}

	log.Printf("Routers Dependencies: %v", routersDependencies)

	log.Println(services[localNatsName].Name)

	routerEnv := types.NewMappingWithEquals(
		[]string{
			fmt.Sprintf("NAME=%s-router", f.UniqueName),
			fmt.Sprintf("GLOBAL_NATS=nats://%s:4222", c.GlobalNatsServiceName),
			fmt.Sprintf("GLOBAL_STREAM=%s", c.GlobalNatsStream),
			fmt.Sprintf("GLOBAL_TOPIC=%s.%s", c.GlobalNatsStream, f.UniqueName),
			fmt.Sprintf("LOCAL_NATS=nats://%s:4222", services[localNatsName].Name),
			fmt.Sprintf("LOCAL_TOPIC=%s", localBaseTopic),
			fmt.Sprintf("SHARDS=%s", strconv.Itoa(f.MaxShards)),
		},
	)

	routerName := fmt.Sprintf("%s-router", f.UniqueName)
	services[routerName] = types.ServiceConfig{
		Name:          routerName,
		ContainerName: routerName,
		DependsOn:     routersDependencies,
		Environment:   routerEnv,
		PullPolicy:    types.PullPolicyMissing,
		Image:         c.RouterImage,
		Networks: map[string]*types.ServiceNetworkConfig{
			"global-net": {},
			localNetName: {},
		},
		Restart: "unless-stopped",
	}

	project := types.Project{
		Name:     f.UniqueName,
		Services: services,
		Networks: map[string]types.NetworkConfig{
			localNetName: localNet,
			"global-net": globalNet,
		},
		Volumes: map[string]types.VolumeConfig{
			natsVolumeName: natsVolume,
		},
	}

	log.Printf("Project: %v", project)

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.Projects[f.UniqueName]; ok {
		return errors.New("function with uniqueName already exists in map")
	}
	c.Projects[f.UniqueName] = project
	return nil
}

func (c *Composer) Del(uniqueName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// TODO: should you do something?
	delete(c.Projects, uniqueName)
}

func (c *Composer) Up(uniqueName string) error {
	var p types.Project
	var ok bool

	c.mu.Lock()
	if p, ok = c.Projects[uniqueName]; !ok {
		// TODO: Add Fallback if ControlPlane crashed -> Maybe External DB?
		return fmt.Errorf("function with name: %s not found", uniqueName)
	}
	c.mu.Unlock()

	yaml, err := p.MarshalYAML()
	if err != nil {
		return err
	}
	log.Printf("%s", yaml)

	return docker.RunProject(&p)
}

func (c *Composer) Down(uniqueName string) error {
	var p types.Project
	var ok bool

	c.mu.Lock()
	if p, ok = c.Projects[uniqueName]; !ok {
		return fmt.Errorf("function with name: %s not found", uniqueName)
	}
	c.mu.Unlock()

	return c.Service.Down(context.Background(), p.Name, api.DownOptions{
		RemoveOrphans: true,
		Project:       &p,
		Volumes:       false,
	})
}

func calcTopics(shards, currReplicaCount, totalReplicas int, baseTopic string) []string {
	base := shards / totalReplicas
	remainder := shards % totalReplicas

	extraStart := totalReplicas - remainder

	count := base
	if currReplicaCount >= extraStart {
		count++
	}

	start := currReplicaCount * base
	if currReplicaCount >= extraStart {
		start += currReplicaCount - extraStart
	}

	topics := make([]string, count)
	for i := 0; i < count; i++ {
		topics[i] = fmt.Sprintf("%s.%d", baseTopic, start+i)
	}

	return topics
}

func deepCopyProject(p types.Project) (*types.Project, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal project: %w", err)
	}
	var copy types.Project
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal project: %w", err)
	}
	return &copy, nil
}
