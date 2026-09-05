// Package hostobject builds guest objects for a goja runtime.
//
// It does not know about Isolates or WinterTC. A host that wants a
// workers.Binding wraps [Object] with workers.Bind.
package hostobject

import (
	"fmt"
	"reflect"
	"unicode"
	"unicode/utf8"

	"github.com/dop251/goja"
)

// Object is a recipe for a guest object. Guest properties are exactly
// the ones the host sets. Fields of a Go struct are never copied.
type Object struct {
	items []item
}

type item struct {
	name string
	val  any
}

// New returns an empty recipe.
func New() *Object {
	return &Object{}
}

// Set adds a guest property. The name is used as written.
// A later Set of the same name fails at Build.
func (o *Object) Set(name string, val any) *Object {
	if o == nil {
		return New().Set(name, val)
	}
	o.items = append(o.items, item{name: name, val: val})
	return o
}

// Methods adds exported methods of ptr. The receiver is bound; struct
// fields are not exposed. Names are uncapitalised (Fetch → fetch).
// ptr MUST be a non-nil pointer.
func (o *Object) Methods(ptr any) *Object {
	if o == nil {
		return New().Methods(ptr)
	}
	o.items = append(o.items, item{val: methodsMark{ptr}})
	return o
}

type methodsMark struct{ ptr any }

// Build creates a new object on rt. rt MUST be the runtime that will
// hold the object. This is not a global and does not install anything
// on rt except the returned object.
func (o *Object) Build(rt *goja.Runtime) (*goja.Object, error) {
	if o == nil {
		return nil, ErrNil
	}
	if rt == nil {
		return nil, ErrNoRuntime
	}
	obj := rt.NewObject()
	seen := map[string]struct{}{}
	for _, it := range o.items {
		if mark, ok := it.val.(methodsMark); ok {
			if err := addMethods(obj, seen, mark.ptr); err != nil {
				return nil, err
			}
			continue
		}
		if it.name == "" {
			return nil, ErrEmptyName
		}
		if _, clash := seen[it.name]; clash {
			return nil, fmt.Errorf("%w: %q", ErrNameClash, it.name)
		}
		if err := obj.Set(it.name, it.val); err != nil {
			return nil, fmt.Errorf("hostobject %q: %w", it.name, err)
		}
		seen[it.name] = struct{}{}
	}
	return obj, nil
}

func addMethods(obj *goja.Object, seen map[string]struct{}, ptr any) error {
	if ptr == nil {
		return ErrMethodsNil
	}
	v := reflect.ValueOf(ptr)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return ErrMethodsNeedPointer
	}
	t := v.Type()
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if m.PkgPath != "" {
			continue
		}
		name := uncapitalise(m.Name)
		if name == "" {
			continue
		}
		if _, clash := seen[name]; clash {
			return fmt.Errorf("%w: %q", ErrNameClash, name)
		}
		if err := obj.Set(name, v.Method(i).Interface()); err != nil {
			return fmt.Errorf("hostobject method %q: %w", name, err)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func uncapitalise(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToLower(r)) + s[size:]
}
