package main

import "github.com/SWC-GEKO/beaver/sdk"

func main() {
	rt := sdk.NewRuntime("localhost", "8080")

	rt.Add("stats", "/Users/stahlco/GolandProjects/beaver/evaluation/beaver", 4, 128)

	if err := rt.Start(); err != nil {
		panic(err)
	}
}
