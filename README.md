<p>
   <img src="resources/img.png" alt="drawing"/>
</p>

<h1 align="center">Beaver - a Serverless Platform</h1>

<p align="center">An open-source FaaS (Function-as-a-Service) framework for writing portable Go-functions on the Beaver-Platform.</p>


## Quickstart
1. **Install Go**
2. **Create a Go module:**
   ```Bash
   go mod init <module-name>
3. **Create a `function.go` file with following contents**
   ```Go
   package main
   
   import (
        "log"
        "context"
   
        beaver "github.com/SWC-GEKO/beaver/sdk"
        "github.com/SWC-GEKO/beaver/spec/api"
   )
   

   func init() {
        beaver.Stateless("my-func", &MyFunction{})
   }
   
   type MyFunction struct {}
   
   func (m *MyFunction) Exec (ctx context.Context, e *api.Event) (*api.Event, error) {
        // write your function-code here...
   }
   ```
   
   > ⚠️ **Warning**
   > 
   > It is required to write the core function-code in `package main` and you must write a `init()`-Function,
   > as it allows the platform to register the function and execute the code.
   
4. **Create a `main.go`, in a different directory, to upload the function - make sure that the ControlPlane is up and running.**
   ```Go
   package main
   
   import (
        beaver "github.com/SWC-GEKO/beaver/sdk"
        "github.com/SWC-GEKO/beaver/spec/contracts"
   )
   
   func main() {
      runtime := beaver.NewRuntime("127.0.0.1", "8080")
      
      runtime.Add("name", "fullpath/to/fn", contracts.STATELESS)
      
      if err := runtime.Start(); err != nil { 
         panic(err)
      }
   }
   ```
   Once the connection to the `Control-Plane` is established, the client-sdk uploads the Function-Code.
   It wraps the User-Code in a Transport-Layer Component using `NATS` and builds it as a Docker-Container.
   After building and deploying the Function, the `Control-Plane` returns the function's address.


5. **Create a `client.go`** that publishes Messages.
   ```Go
   package client
   
   import (
        "github.com/nats-io/nats.go"
   )
   
   var url = "" // URL the Control-Plane returned
   
   func main() {
       nc, err := nats.Connect(url, opts...)
       if err != nil {
           log.Fatal(err)
       }
       defer nc.Close()
   
	   // TODO: Define how users should send a Message
   }
   ```
   
   