package main

import (
	"log"

	"github.com/SWC-GEKO/beaver/sdk"
)

func main() {
	rt := sdk.NewRuntime("localhost", "8080")

	rt.StatelessFunction("echo", "/Users/stahlco/GolandProjects/beaver/test/echo/")
	//rt.StatefulFunction("echo2", "/Users/stahlco/GolandProjects/beaver/test/echo2/")

	if err := rt.Start(); err != nil {
		log.Println(err)
	}
}
