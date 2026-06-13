package main

import (
	"github.com/SWC-GEKO/beaver/sdk"
	"github.com/SWC-GEKO/beaver/spec/contracts"
)

func main() {
	rt := sdk.NewRuntime("localhost", "8080")

	rt.Add("echo", "/Users/stahlco/GolandProjects/beaver/test/echo", contracts.STATELESS)

	if err := rt.Start(); err != nil {
		panic(err)
	}
}
