package workers

import "github.com/dop251/goja"

// RuntimeObject builds a guest object on a goja runtime. hostobject.Object
// satisfies this; workers does not import that package.
type RuntimeObject interface {
	Build(rt *goja.Runtime) (*goja.Object, error)
}

// Bind adapts a RuntimeObject into a Binding. Materialize calls Build on
// this isolate's runtime. The recipe is not installed globally.
func Bind(o RuntimeObject) Binding {
	return runtimeBinding{o: o}
}

type runtimeBinding struct {
	o RuntimeObject
}

func (b runtimeBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	if b.o == nil {
		return nil, ErrBindNilObject
	}
	return b.o.Build(iso.vm)
}
