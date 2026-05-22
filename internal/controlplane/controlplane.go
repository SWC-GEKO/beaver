package controlplane

import (
	"encoding/base64"
	"log"
	"os"
	"path"
	"sync"

	"github.com/SWC-GEKO/beaver/internal/docker"
	"github.com/SWC-GEKO/beaver/internal/fn"
	"github.com/SWC-GEKO/beaver/internal/utils"
	"github.com/google/uuid"
)

const (
	TmpDir = "./tmp"
)

type ControlPlane struct {
	id        string
	functions map[string]fn.Function
	fnMtx     sync.Mutex
	docker    docker.Docker
}

func (cp *ControlPlane) UploadStateless(name string, fnZip string) error {
	// TODO: implement name checks
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

	cp.fnMtx.Lock()
	var oldF fn.Function
	if exF, ok := cp.functions[name]; ok {
		oldF = exF
	}
	cp.fnMtx.Unlock()

	f, err := cp.docker.Create() // TODO: implement a proper create function
	if err != nil {
		return err
	}

	cp.fnMtx.Lock()
	cp.functions[name] = f
	if err = cp.functions[name].Start(); err != nil {
		return err
	}
	cp.fnMtx.Unlock()

	// TODO: implement function to tell RProxy about new function

	if oldF != nil {
		if err = oldF.Stop(); err != nil {
			return err
		}
	}

	return nil
}
