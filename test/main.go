package main

import (
	sdk "github.com/SWC-GEKO/beaver/sdk/runtime"
)

func main() {
	rt := sdk.NewRuntime("localhost", "8080")

	rt.StatelessFunction("echo", "/Users/stahlco/GolandProjects/beaver/test/echo/")
}
