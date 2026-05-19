package runtime

import (
	"fmt"
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

	if err := validateFunction(path, STATELESS); err != nil {
		log.Printf("%s invalid runtime definition: %v", name, err)
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

	if err := validateFunction(path, STATEFUL); err != nil {
		log.Printf("%s invalid runtime definition: %v", name, err)
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

func validateFunction(path string, functionType int) error {
	var ifaceFuncName string
	switch functionType {
	case STATELESS:
		ifaceFuncName = "StatelessFunction"
	case STATEFUL:
		ifaceFuncName = "StatefulFunction"
	default:
		return fmt.Errorf("function type is unknown")
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
		return err
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
				return fmt.Errorf("interface %q is not implemented", ifaceFuncName)
			}

			iface = iface.Complete()
		}
		if iface == nil {
			return fmt.Errorf("given file does not implement interface: %s", ifaceFuncName)
		}

		scope := pkg.Types.Scope()

		for _, n := range scope.Names() {
			o := scope.Lookup(n)

			if types.Implements(o.Type(), iface) ||
				types.Implements(types.NewPointer(o.Type()), iface) {
				return nil
			}
		}
	}

	return fmt.Errorf("interface is not implemented")
}
