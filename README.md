# Beaver 
An open-source FaaS (Function-as-a-Service) framework for writing portable Go-functions on the Beaver-Platform.

Quickstart
---
1. Install Go
2. Create a Go module:
   > Note: You can use a different module name.
   ```Bash
   go mod init example.com/test
   ```
3. Create a `main.go` file with following contents
   ```Go
   package main
   
   import (
        "log"
        "context"
   
        "github.com/SWC-GEKO/beaver/sdk"
        "github.com/SWC-GEKO/beaver/spec/api"
   )
   
   // MyFunction must implement StatelessFunction
   type MyFunction struct {}
   
   func (m *MyFunction) Exec (ctx context.Context, e *api.Event) (*api.Event, error) {
        // write your function-code here
   }
   
   func main() {
        r := runtime.New("localhost", "8080") // to connect with control-plane
        r.StatelessFunction("test", "full/path/to/fn", &MyFunction{}) // TODO
   
        if err := r.Start(); err != nil {
            panic(err)
        }
   }
   ```
   