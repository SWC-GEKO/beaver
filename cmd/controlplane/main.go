package main

import (
	"log"
	"net/http"

	"github.com/SWC-GEKO/beaver/pkg/controlplane"
)

type server struct {
	cp *controlplane.ControlPlane
}

const addr = ":8080"

func main() {
	log.SetPrefix("controlplane: ")
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	mux := http.NewServeMux()

	mux.HandleFunc("/test", controlplane.UploadFunction)

	log.Println("starting to listen on: ", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalln(err)
	}
}
