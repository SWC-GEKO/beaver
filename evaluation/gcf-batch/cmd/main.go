package main

import (
	_ "gcf-batch/pkg"
	"log"

	"github.com/GoogleCloudPlatform/functions-framework-go/funcframework"
)

func main() {
	hostname := "127.0.0.1"
	port := "8082"

	if err := funcframework.StartHostPort(hostname, port); err != nil {
		log.Printf("error when running function-framework")
	}
}
