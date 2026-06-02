package main

import (
	"log"

	"github.com/SWC-GEKO/beaver/sdk"
	"github.com/SWC-GEKO/beaver/spec/contracts"
)

func main() {
	rt := sdk.NewRuntime("localhost", "8080")

	rt.Add("my-func", "/Users/stahlco/GolandProjects/beaver/test/echo", contracts.STATELESS)

	if err := rt.Start(); err != nil {
		log.Println(err)
	}
}
