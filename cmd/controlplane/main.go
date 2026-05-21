package main

import (
	"log"

	"github.com/SWC-GEKO/beaver/pkg/controlplane"
)

type server struct {
	cp *controlplane.ControlPlane
}

const addr = ":8080"

func main() {
	log.SetPrefix("controlplane: ")
	log.SetFlags(log.Lshortfile | log.LstdFlags)

}
