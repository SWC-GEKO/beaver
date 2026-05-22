package main

import (
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

func main() {
	log.SetPrefix("controlplane: ")
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	s := server{}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/upload", s.upload)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalln(err)
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

	switch data.Type {
	case contracts.STATELESS:
		if err := s.cp.UploadStateless(data.Name, data.Zip); err != nil {
			log.Println("uploading stateless fn failed with err: ", err)
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
	case contracts.STATEFUL:
		rw.WriteHeader(http.StatusNotImplemented)
		return
	default:
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	rw.WriteHeader(http.StatusOK)
}
