package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/SWC-GEKO/beaver/internal/controlplane"
	"github.com/SWC-GEKO/beaver/spec/contracts"
)

type server struct {
	cp *controlplane.ControlPlane
}

const addr = ":8080"
const dir = "internal/controlplane/evaluationtest-registry"
const stream = "FUNCTIONS"
const natsUrl = ":4222"

func main() {
	log.SetPrefix("controlplane: ")
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	_, err := controlplane.NewRegistry(dir)
	if err != nil {
		panic(err)
	}

	cp, err := controlplane.New(stream, natsUrl, dir)
	if err != nil {
		panic(err)
	}

	s := server{
		cp: cp,
	}

	if err := s.cp.Start(context.Background()); err != nil {
		panic(err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/upload", s.upload)

	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}

func (s *server) health(rw http.ResponseWriter, _ *http.Request) {
	rw.WriteHeader(http.StatusOK)
}

func (s *server) upload(rw http.ResponseWriter, r *http.Request) {
	var data contracts.UploadRequest
	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		log.Printf("decoding incoming request: %v failed with err: %v", r.Body, err)
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	var uniqueName string
	var err error
	if uniqueName, err = s.cp.Upload(data.Name, data.Zip, data.Replication, data.VirtualShards); err != nil {
		log.Println("uploading function failed with err: ", err)
		rw.WriteHeader(http.StatusBadRequest)
		rw.Write([]byte(err.Error()))
		return
	}
	rw.WriteHeader(http.StatusOK)
	// TODO: add a Upload-Response
	rw.Write([]byte(uniqueName))
}
