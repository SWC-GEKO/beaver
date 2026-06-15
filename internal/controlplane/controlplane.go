package controlplane

import (
	"context"
	"encoding/base64"
	"log"
	"os"
	"path"
	"sync"
	"time"

	"github.com/SWC-GEKO/beaver/internal/docker"
	"github.com/SWC-GEKO/beaver/internal/utils"
	"github.com/google/uuid"
)

const (
	TmpDir = "./tmp"
)

type ControlPlane struct {
	id        string
	functions map[string]docker.Function
	fnMtx     sync.Mutex
	docker    docker.Docker
}

func New(id string) *ControlPlane {
	return &ControlPlane{
		id:        id,
		functions: make(map[string]docker.Function),
		fnMtx:     sync.Mutex{},
		docker:    docker.NewDocker(),
	}
}

func (cp *ControlPlane) Start() error {
	// 1. We need to connect to the global NATS!
	// 2. We need to build the images -> first check if router for example is already built!
	// 3. Start HTTP-Server and listen to incoming Upload-Requests
	// 4. Start NATS-Observer

	return nil
}

func (cp *ControlPlane) UploadStateless(name string, fnZip string) error {
	zip, err := base64.StdEncoding.DecodeString(fnZip)
	if err != nil {
		return err
	}

	u, err := uuid.NewV7()
	if err != nil {
		return err
	}

	p := path.Join(TmpDir, u.String())
	err = os.MkdirAll(p, 0777)
	if err != nil {
		return err
	}
	log.Println("created folder: ", p)

	zipPath := path.Join(TmpDir, u.String()+".zip")
	err = os.WriteFile(zipPath, zip, 0777)
	if err != nil {
		return err
	}

	err = utils.Unzip(zipPath, p)
	if err != nil {
		return err
	}

	defer func() {
		if err = os.RemoveAll(p); err != nil {
			log.Printf("not able to delete %s: %v, please remove file manually", p, err)
		}

		if err = os.RemoveAll(zipPath); err != nil {
			log.Printf("not able to delete %s: %v, please remove file manually", p, err)
		}
	}()

	//TODO: check if context is correct here!
	ctx := context.Background()
	f, err := cp.docker.Create(ctx, name, p)
	if err != nil {
		return err
	}

	log.Println(f.UniqueName)

	time.Sleep(10 * time.Second)
	return nil
}
