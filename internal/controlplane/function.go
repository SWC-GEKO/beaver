package controlplane

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/SWC-GEKO/beaver/internal/docker"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
)

type Function struct {
	UniqueName string
	Status     Status

	record         FunctionRecord
	composeService api.Compose
	project        types.Project
}

type Status int

const (
	Idle Status = iota
	Active
)

func NewFunction(f FunctionRecord) (*Function, error) {
	composeService, err := createComposeService()
	if err != nil {
		return nil, err
	}
	project := createFunctionProject(f)

	return &Function{
		UniqueName:     f.UniqueName,
		Status:         Idle,
		record:         f,
		composeService: composeService,
		project:        project,
	}, nil
}

func createComposeService() (api.Compose, error) {
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

	return compose.NewComposeService(dockerCli), nil
}

func createFunctionProject(f FunctionRecord) types.Project {
	localBaseTopic := "function"

	localNetName := fmt.Sprintf("%s-local-net", f.UniqueName)
	localNet := types.NetworkConfig{
		Name:   localNetName,
		Driver: "bridge",
	}

	globalNet := types.NetworkConfig{
		Name:     f.GlobalNet,
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
		Environment:   nil,
		Image:         f.NatsImage,
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

		subTopics := calcTopics(f.VirtualShards, i, f.Replication, localBaseTopic)

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

	routerEnv := types.NewMappingWithEquals(
		[]string{
			fmt.Sprintf("NAME=%s-router", f.UniqueName),
			fmt.Sprintf("GLOBAL_NATS=nats://%s:4222", f.GlobalNatsServiceName),
			fmt.Sprintf("GLOBAL_STREAM=%s", f.GlobalNatsStream),
			fmt.Sprintf("GLOBAL_TOPIC=%s.%s", f.GlobalNatsStream, f.UniqueName),
			fmt.Sprintf("LOCAL_NATS=nats://%s:4222", services[localNatsName].Name),
			fmt.Sprintf("LOCAL_TOPIC=%s", localBaseTopic),
			fmt.Sprintf("SHARDS=%s", strconv.Itoa(f.VirtualShards)),
		},
	)

	routerName := fmt.Sprintf("%s-router", f.UniqueName)
	services["router"] = types.ServiceConfig{
		Name:          routerName,
		ContainerName: routerName,
		DependsOn:     routersDependencies,
		Environment:   routerEnv,
		PullPolicy:    types.PullPolicyMissing,
		Image:         f.RouterImage,
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

	return project
}

func (f *Function) Start(ctx context.Context, seq uint64) error {
	r := f.project.Services["router"]
	r.Environment = r.Environment.OverrideBy(
		types.NewMappingWithEquals(
			[]string{fmt.Sprintf("SEQ=%d", seq)},
		),
	)
	f.project.Services["router"] = r

	return docker.RunProject(ctx, &f.project)
}

func (f *Function) Stop(ctx context.Context) error {
	return f.composeService.Down(ctx, f.project.Name, api.DownOptions{
		RemoveOrphans: true,
		Project:       &f.project,
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
