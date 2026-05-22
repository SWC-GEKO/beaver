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
	var buf []byte
	_, err := r.Body.Read(buf)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
	}
	defer r.Body.Close()

	if err := json.Unmarshal(buf, &data); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
	}

	switch data.Type {
	case contracts.STATELESS:
		if err = s.cp.UploadStateless(data.Name, data.Zip); err != nil {
			// TODO: figure out which type of http-status is required => match error?
			rw.WriteHeader(http.StatusBadRequest)
			_, _ = rw.Write([]byte(err.Error()))
		}
		// TODO: implement proper responses
	case contracts.STATEFUL:
		rw.WriteHeader(http.StatusNotImplemented)
	default:
		rw.WriteHeader(http.StatusBadRequest)
	}

	rw.WriteHeader(http.StatusOK)
}
