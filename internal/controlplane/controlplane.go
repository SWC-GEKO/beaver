package controlplane

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/SWC-GEKO/beaver/internal/docker"
	"github.com/SWC-GEKO/beaver/internal/utils"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

const (
	TmpDir = "./tmp"
)

type ControlPlane struct {
	stream   string
	natsUrl  string
	docker   docker.Docker
	registry *Registry
	observer *Observer
}

func New(stream, natsUrl, registryDir string) (*ControlPlane, error) {
	registry, err := NewRegistry(registryDir)
	if err != nil {
		return nil, err
	}

	observer, err := NewObserver(context.Background(), natsUrl, stream)
	if err != nil {
		return nil, err
	}

	return &ControlPlane{
		stream:   stream,
		natsUrl:  natsUrl,
		registry: registry,
		observer: observer,
		docker:   docker.NewDocker(),
	}, nil
}

func (cp *ControlPlane) Start(ctx context.Context) error {
	log.Println("starting the control-plane")
	nc, err := nats.Connect(cp.natsUrl)
	if err != nil {
		return fmt.Errorf("connecting to global nats failed with err: %v", err)
	}
	if nc.Status() != nats.CONNECTED {
		return fmt.Errorf("nats status is not CONNECTED, aborting control-plane start-up, status is: %v", nc.Status())
	}
	nc.Close()

	functionRecs, err := cp.registry.List()
	if err != nil {
		return err
	}

	// 4. Start Observer -> Maybe do some additional stuff here
	o, err := NewObserver(ctx, cp.natsUrl, cp.stream)
	if err != nil {
		return fmt.Errorf("creating new observer failed with err: %v", err)
	}

	// 5. Register all Functions in Observer
	for _, f := range functionRecs {
		if !o.RegisterFunction(f) {
			log.Printf("function with name: %s, not able to be registered", f.UniqueName)
		}
	}

	go o.Start()

	return nil
}

func (cp *ControlPlane) Upload(name string, fnZip string, replication, vShards int) (string, error) {
	zip, err := base64.StdEncoding.DecodeString(fnZip)
	if err != nil {
		return "", err
	}

	u, err := uuid.NewV7()
	if err != nil {
		return "", err
	}

	p := path.Join(TmpDir, u.String())
	err = os.MkdirAll(p, 0777)
	if err != nil {
		return "", err
	}
	log.Println("created folder: ", p)

	zipPath := path.Join(TmpDir, u.String()+".zip")
	err = os.WriteFile(zipPath, zip, 0777)
	if err != nil {
		return "", err
	}

	err = utils.Unzip(zipPath, p)
	if err != nil {
		return "", err
	}

	defer func() {
		if err = os.RemoveAll(p); err != nil {
			log.Printf("not able to delete %s: %v, please remove file manually", p, err)
		}

		if err = os.RemoveAll(zipPath); err != nil {
			log.Printf("not able to delete %s: %v, please remove file manually", p, err)
		}
	}()

	ctx := context.Background()

	uniqueName, imageTag, err := cp.docker.Create(ctx, name, p)
	if err != nil {
		return "", err
	}

	rec := DefaultRecord(uniqueName, imageTag, replication, vShards)

	log.Println(rec)

	if err := cp.registry.Save(rec); err != nil {
		return "", err
	}

	log.Println(rec)

	if !cp.observer.RegisterFunction(rec) {
		cp.registry.Delete(rec.UniqueName)
		return "", fmt.Errorf("not able to register function at observer, aborting")
	}

	return rec.UniqueName, nil
}
