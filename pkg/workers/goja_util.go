package workers

import (
	"fmt"

	"github.com/dop251/goja"
)

// mustSet installs a property on a goja object. Host setup failures are fatal:
// a half-installed isolate is not usable.
func mustSet(obj *goja.Object, name string, val any) {
	if err := obj.Set(name, val); err != nil {
		panic(fmt.Sprintf("goja set %q: %v", name, err))
	}
}

// mustRuntimeSet installs a global on the runtime.
func mustRuntimeSet(rt *goja.Runtime, name string, val any) {
	if err := rt.Set(name, val); err != nil {
		panic(fmt.Sprintf("goja runtime set %q: %v", name, err))
	}
}

// mustAccessor installs a read-only accessor on a prototype.
func mustAccessor(obj *goja.Object, name string, getter goja.Value) {
	if err := obj.DefineAccessorProperty(name, getter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		panic(fmt.Sprintf("goja accessor %q: %v", name, err))
	}
}
