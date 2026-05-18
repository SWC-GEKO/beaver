package runtime

import (
	"go/types"
	"log"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Runtime struct {
	Host     string
	Port     string
	function function
}

func NewRuntime(host, port string) *Runtime {
	return &Runtime{
		Host: host,
		Port: port,
	}
}

func (rt *Runtime) StatelessFunction(name, path string) *Runtime {

	if !validateFunction(path, STATELESS) {
		log.Println("invalid runtime definition: ", name)
	}

	fn := function{
		name:         name,
		path:         path,
		functionType: STATELESS,
	}

	// TODO: Add logic that if the function already exists that it throws an error
	rt.function = fn

	return rt
}

func (rt *Runtime) StatefulFunction(name, path string) *Runtime {
	if !validateFunction(path, STATEFUL) {
		log.Println("invalid runtime definition: ", name)
	}

	fn := function{
		name:         name,
		path:         path,
		functionType: STATEFUL,
	}

	// TODO: Add logic that if the function already exists that it throws an error
	rt.function = fn

	return rt
}

func validateFunction(path string, functionType int) bool {
	var ifaceFuncName string
	switch functionType {
	case STATELESS:
		ifaceFuncName = "StatelessFunction"
	case STATEFUL:
		ifaceFuncName = "StatefulFunction"
	default:
		log.Println("function type is unknown")
		return false
	}

	cfg := &packages.Config{
		Mode: packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedSyntax |
			packages.NeedImports,
		Dir: path,
	}

	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		log.Fatalln(err)
	}

	for _, pkg := range pkgs {

		var iface *types.Interface
		for _, i := range pkg.Types.Imports() {
			if !(strings.Compare(i.Path(), "github.com/SWC-GEKO/beaver/sdk/api") == 0) {
				continue
			}

			o := i.Scope().Lookup(ifaceFuncName)

			var ok bool
			iface, ok = o.Type().Underlying().(*types.Interface)
			if !ok {
				log.Printf("interface \"%n\" is not implemented", ifaceFuncName)
			}
			iface = iface.Complete()
		}
		if iface == nil {
			log.Println("given file does not implement interface: ", ifaceFuncName)
			return false
		}

		scope := pkg.Types.Scope()

		for _, n := range scope.Names() {
			o := scope.Lookup(n)

			if types.Implements(o.Type(), iface) {
				return true
			}
		}
	}

	return false
}
