package workers

import (
	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
)

// nodeLightningCSSBinding is require("lightningcss-<platform>") — the
// native addon lightningcss's JS loader wants. There is no .node file;
// transform is in-process esbuild CSS (Tailwind's optimize step).
type nodeLightningCSSBinding struct{}

var _ Binding = nodeLightningCSSBinding{}

func (nodeLightningCSSBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	if v, ok := iso.moduleCache["lightningcss-native"]; ok {
		if o, ok := v.(*goja.Object); ok {
			return o, nil
		}
	}
	obj := newNodeLightningCSS(iso)
	iso.moduleCache["lightningcss-native"] = obj
	return obj, nil
}

func newNodeLightningCSS(iso *Isolate) *goja.Object {
	n := &nodeLightning{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "transform", n.jsTransform)
	mustSet(obj, "transformStyleAttribute", n.jsTransform)
	mustSet(obj, "bundle", n.jsTransform)
	mustSet(obj, "bundleAsync", n.jsBundleAsync)
	mustSet(obj, "default", obj)
	return obj
}

type nodeLightning struct {
	iso *Isolate
}

func (n *nodeLightning) jsTransform(call goja.FunctionCall) goja.Value {
	src := ""
	filename := "input.css"
	minify := false
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		opts := call.Argument(0).ToObject(n.iso.vm)
		src = string(valueBytes(opts.Get("code")))
		if f := jsToString(opts.Get("filename")); f != "" {
			filename = f
		}
		if v := opts.Get("minify"); v != nil && !goja.IsUndefined(v) {
			minify = v.ToBoolean()
		}
	}
	r := api.Transform(src, api.TransformOptions{
		Loader:           api.LoaderCSS,
		Sourcefile:       filename,
		MinifyWhitespace: minify,
		MinifySyntax:     minify,
	})
	code := r.Code
	if len(r.Errors) > 0 {
		code = []byte(src)
	}
	out := n.iso.vm.NewObject()
	mustSet(out, "code", jsBytes(n.iso, code))
	mustSet(out, "warnings", n.iso.vm.NewArray())
	return out
}

func (n *nodeLightning) jsBundleAsync(call goja.FunctionCall) goja.Value {
	p, resolve, _ := n.iso.vm.NewPromise()
	resolve(n.jsTransform(call))
	return n.iso.vm.ToValue(p)
}
