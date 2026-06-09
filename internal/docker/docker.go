package docker

import (
	"fmt"
	"os"
	"path"

	"github.com/SWC-GEKO/beaver/internal/utils"
	"github.com/docker/go-sdk/client"
	uuid2 "github.com/google/uuid"
)

const TmpDir = "./tmp"

type Docker struct {
	client client.SDKClient
}

func NewDocker() Docker {
	return Docker{}
}

func (d *Docker) Create(name string, filedir string) (*Function, error) {
	uuid, err := uuid2.NewRandom()
	if err != nil {
		return nil, err
	}

	uniqueName := fmt.Sprintf("%s-%s", name, uuid.String())

	filePath := path.Join(TmpDir, uniqueName)
	if err = os.MkdirAll(filePath, 0777); err != nil {
		return nil, fmt.Errorf("creating unique-function directory failed with error: %v", err)
	}

	processorPath := path.Join("runtimes", "processor")

	if err = utils.CopyDir(processorPath, filePath); err != nil {
		return nil, fmt.Errorf("copying the handler-code into the unique directory failed with error: %v", err)
	}

	if err = utils.CopyAll(filedir, processorPath); err != nil {
		return nil, fmt.Errorf("copying function-code into the directory failed with err: %v", err)
	}

	return &Function{
		UniqueName: uniqueName,
	}, nil
}

func (d *Docker) BuildImage() (*Image, error) {
	panic("implement me...")
}
