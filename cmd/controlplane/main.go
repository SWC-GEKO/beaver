package main

import (
	"log"
	"net/http"

	"github.com/SWC-GEKO/beaver/internal/controlplane"
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

	rw.WriteHeader(http.StatusOK)
}
