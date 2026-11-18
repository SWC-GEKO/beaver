// lg is the single load generator used by all benchmarks.
//
//	lg nats-stream   Beaver, one stream at a fixed rate (Exp. 1/2)
//	lg http-single   FaaS-Single, one HTTP request per event (Exp. 1/2)
//	lg http-batch    FaaS-Batched, all events in one request after the stream (Exp. 1/2)
//	lg nats-scale    Beaver, N concurrent streams (Exp. 3)
//	lg beaver-upload upload the function to the Beaver control plane, prints its unique name
//
// Every mode writes a JSON meta file (-meta) with the t0 / counts the analysis needs.
// All timestamps are unix nanoseconds taken on the machine running lg.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: lg <nats-stream|http-single|http-batch|nats-scale|beaver-upload> [flags]")
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "nats-stream":
		err = runNatsStream(args)
	case "http-single":
		err = runHTTPSingle(args)
	case "http-batch":
		err = runHTTPBatch(args)
	case "nats-scale":
		err = runNatsScale(args)
	case "beaver-upload":
		err = runBeaverUpload(args)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lg:", err)
		os.Exit(1)
	}
}

func writeJSON(path string, v any) error {
	if path == "" {
		return nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
