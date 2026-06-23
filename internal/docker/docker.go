package docker

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"

	"github.com/SWC-GEKO/beaver/internal/utils"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/go-sdk/client"
	"github.com/docker/go-sdk/image"
	uuid2 "github.com/google/uuid"
	moby "github.com/moby/moby/client"
)

const TmpDir = "./tmp"

type Docker struct {
	client client.SDKClient
}

func NewDocker() Docker {
	// TODO: implement a feature that the router is going to be built!
	return Docker{}
}

func (d *Docker) Create(ctx context.Context, name string, filedir string) (*Function, error) {
	uuid, err := uuid2.NewRandom()
	if err != nil {
		return nil, err
	}

	uniqueName := fmt.Sprintf("%s-%s", name, uuid.String())

	filePath := path.Join(TmpDir, uniqueName)
	if err = os.MkdirAll(filePath, 0777); err != nil {
		return nil, fmt.Errorf("creating unique-function directory failed with error: %v", err)
	}

	processorPath := "internal/docker/runtime/processor"

	if err = utils.CopyAll(processorPath, filePath); err != nil {
		return nil, fmt.Errorf("copying the handler-code into the unique directory failed with error: %v", err)
	}

	if err = utils.CopyAll(filedir, filePath); err != nil {
		return nil, fmt.Errorf("copying function-code into the directory failed with err: %v", err)
	}

	// Building the Processor
	// TODO: version should be passed by the user
	tag, err := d.BuildImage(ctx, uniqueName, "", filePath)
	if err != nil {
		return nil, fmt.Errorf("building image failed with err: %v", err)
	}

	log.Println("Created processor image with tag: ", tag)

	f := Function{
		UniqueName: uniqueName,
		ImageTag:   tag,
	}

	return &f, nil
}

func (d *Docker) BuildImage(ctx context.Context, name, version, dir string) (string, error) {
	if version == "" {
		version = "latest"
	}

	log.Println(dir)

	r, err := image.ArchiveBuildContext(dir, "Dockerfile")
	if err != nil {
		return "", err
	}

	opts := image.WithBuildOptions(moby.ImageBuildOptions{
		Dockerfile: "Dockerfile",
	})

	tag, err := image.Build(ctx, r, fmt.Sprintf("%s:%s", name, version), opts)
	if err != nil {
		return "", err
	}

	return tag, nil
}

func RunProject(ctx context.Context, p *types.Project) error {
	f, err := os.Create("compose.yaml")
	if err != nil {
		return err
	}
	defer func() {
		err = f.Close()
		log.Println(err)

		err = os.Remove(f.Name())
		log.Println(err)
	}()

	yaml, err := p.MarshalYAML()
	if err != nil {
		return err
	}

	_, err = f.Write(yaml)
	if err != nil {
		return err
	}

	cmd := exec.Command("docker", "compose", "-f", f.Name(), "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	return nil
}
