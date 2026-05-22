package sdk

import (
	"fmt"
	"go/types"
	"log"
	"strings"

	"github.com/SWC-GEKO/beaver/spec/contracts"
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

func (rt *Runtime) Start() error {
	cnc, err := connect("localhost", "8080")
	if err != nil {
		return err
	}

	if err = cnc.upload(rt); err != nil {
		return err
	}

	return nil
}

func (rt *Runtime) StatelessFunction(name, path string) *Runtime {

	if err := validateFunction(path, contracts.STATELESS); err != nil {
		log.Fatalf("%s invalid runtime definition: %v", name, err)
	}

	fn := function{
		name:         name,
		path:         path,
		functionType: contracts.STATELESS,
	}

	rt.function = fn

	return rt
}

func (rt *Runtime) StatefulFunction(name, path string) *Runtime {

	if err := validateFunction(path, contracts.STATEFUL); err != nil {
		log.Fatalf("%s invalid runtime definition: %v", name, err)
	}

	fn := function{
		name:         name,
		path:         path,
		functionType: contracts.STATEFUL,
	}

	// TODO: Add logic that if the fn already exists that it throws an error
	rt.function = fn

	return rt
}

func validateFunction(path string, functionType int) error {
	var ifaceFuncName string
	switch functionType {
	case contracts.STATELESS:
		ifaceFuncName = "StatelessFunction"
	case contracts.STATEFUL:
		ifaceFuncName = "StatefulFunction"
	default:
		return fmt.Errorf("fn type is unknown")
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
			if !(strings.Compare(i.Path(), "github.com/SWC-GEKO/beaver/spec/api") == 0) {
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
