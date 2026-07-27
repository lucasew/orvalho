package workers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/dop251/goja"
)

// HTTPRequest is the host-side view of an inbound request to inject into JS.
type HTTPRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
}

// HTTPResponse is the host-side view extracted from a JS Response object.
type HTTPResponse struct {
	Status     int
	StatusText string
	Headers    map[string]string
	Body       string
}

type headerBag struct {
	// keys are canonicalized to lower-case; values are single strings
	// (append joins with ", " like the Fetch Headers combine rule).
	m map[string]string
}

type requestBag struct {
	method  string
	url     string
	headers *headerBag
	body    string
}

type responseBag struct {
	status     int
	statusText string
	headers    *headerBag
	body       string
	// bodyStream is a guest ReadableStream when the Response body is not a plain string.
	// Host drains it via text() / collectBody before returning to HTTP.
	bodyStream *goja.Object
}

func (iso *Isolate) installWebTypes() {
	iso.headersReg = make(map[*goja.Object]*headerBag)
	iso.requestReg = make(map[*goja.Object]*requestBag)
	iso.responseReg = make(map[*goja.Object]*responseBag)

	iso.vm.Set("Headers", iso.headersConstructor)
	iso.vm.Set("Request", iso.requestConstructor)
	iso.vm.Set("Response", iso.responseConstructor)

	headersProto := iso.vm.Get("Headers").ToObject(iso.vm).Get("prototype").ToObject(iso.vm)
	headersProto.Set("get", iso.headersGet)
	headersProto.Set("set", iso.headersSet)
	headersProto.Set("has", iso.headersHas)
	headersProto.Set("delete", iso.headersDelete)
	headersProto.Set("append", iso.headersAppend)

	reqProto := iso.vm.Get("Request").ToObject(iso.vm).Get("prototype").ToObject(iso.vm)
	_ = reqProto.DefineAccessorProperty("method", iso.vm.ToValue(iso.requestGetMethod), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = reqProto.DefineAccessorProperty("url", iso.vm.ToValue(iso.requestGetURL), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = reqProto.DefineAccessorProperty("headers", iso.vm.ToValue(iso.requestGetHeaders), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	reqProto.Set("text", iso.requestText)

	resProto := iso.vm.Get("Response").ToObject(iso.vm).Get("prototype").ToObject(iso.vm)
	_ = resProto.DefineAccessorProperty("status", iso.vm.ToValue(iso.responseGetStatus), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = resProto.DefineAccessorProperty("statusText", iso.vm.ToValue(iso.responseGetStatusText), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = resProto.DefineAccessorProperty("ok", iso.vm.ToValue(iso.responseGetOK), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = resProto.DefineAccessorProperty("headers", iso.vm.ToValue(iso.responseGetHeaders), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	resProto.Set("text", iso.responseText)
	resProto.Set("arrayBuffer", iso.responseArrayBuffer)
	resProto.Set("json", iso.responseJSON)
}

func newHeaderBag() *headerBag {
	return &headerBag{m: make(map[string]string)}
}

func (h *headerBag) get(name string) (string, bool) {
	v, ok := h.m[canonicalHeader(name)]
	return v, ok
}

func (h *headerBag) set(name, value string) {
	h.m[canonicalHeader(name)] = value
}

func (h *headerBag) has(name string) bool {
	_, ok := h.m[canonicalHeader(name)]
	return ok
}

func (h *headerBag) del(name string) {
	delete(h.m, canonicalHeader(name))
}

func (h *headerBag) append(name, value string) {
	key := canonicalHeader(name)
	if prev, ok := h.m[key]; ok && prev != "" {
		h.m[key] = prev + ", " + value
		return
	}
	h.m[key] = value
}

func (h *headerBag) clone() *headerBag {
	out := newHeaderBag()
	for k, v := range h.m {
		out.m[k] = v
	}
	return out
}

func (h *headerBag) asMap() map[string]string {
	out := make(map[string]string, len(h.m))
	for k, v := range h.m {
		out[k] = v
	}
	return out
}

func canonicalHeader(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (iso *Isolate) headersConstructor(call goja.ConstructorCall) *goja.Object {
	h := newHeaderBag()
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		if err := iso.fillHeaders(h, call.Argument(0)); err != nil {
			panic(iso.vm.NewTypeError(err.Error()))
		}
	}
	iso.headersReg[call.This] = h
	return nil
}

func (iso *Isolate) fillHeaders(h *headerBag, init goja.Value) error {
	if obj, ok := init.(*goja.Object); ok {
		if existing, ok := iso.headersReg[obj]; ok {
			for k, v := range existing.m {
				h.m[k] = v
			}
			return nil
		}
	}
	o := init.ToObject(iso.vm)
	for _, key := range o.Keys() {
		v := o.Get(key)
		if v == nil || goja.IsUndefined(v) {
			continue
		}
		h.set(key, v.String())
	}
	return nil
}

func (iso *Isolate) headerBagOf(call goja.FunctionCall) *headerBag {
	obj := call.This.ToObject(iso.vm)
	h, ok := iso.headersReg[obj]
	if !ok {
		panic(iso.vm.NewTypeError("Headers method called on incompatible receiver"))
	}
	return h
}

func (iso *Isolate) headersGet(call goja.FunctionCall) goja.Value {
	h := iso.headerBagOf(call)
	if len(call.Arguments) < 1 {
		return goja.Null()
	}
	v, ok := h.get(call.Argument(0).String())
	if !ok {
		return goja.Null()
	}
	return iso.vm.ToValue(v)
}

func (iso *Isolate) headersSet(call goja.FunctionCall) goja.Value {
	h := iso.headerBagOf(call)
	if len(call.Arguments) < 2 {
		panic(iso.vm.NewTypeError("Headers.set requires name and value"))
	}
	h.set(call.Argument(0).String(), call.Argument(1).String())
	return goja.Undefined()
}

func (iso *Isolate) headersHas(call goja.FunctionCall) goja.Value {
	h := iso.headerBagOf(call)
	if len(call.Arguments) < 1 {
		return iso.vm.ToValue(false)
	}
	return iso.vm.ToValue(h.has(call.Argument(0).String()))
}

func (iso *Isolate) headersDelete(call goja.FunctionCall) goja.Value {
	h := iso.headerBagOf(call)
	if len(call.Arguments) >= 1 {
		h.del(call.Argument(0).String())
	}
	return goja.Undefined()
}

func (iso *Isolate) headersAppend(call goja.FunctionCall) goja.Value {
	h := iso.headerBagOf(call)
	if len(call.Arguments) < 2 {
		panic(iso.vm.NewTypeError("Headers.append requires name and value"))
	}
	h.append(call.Argument(0).String(), call.Argument(1).String())
	return goja.Undefined()
}

func (iso *Isolate) newHeadersObject(h *headerBag) *goja.Object {
	ctor, ok := goja.AssertConstructor(iso.vm.Get("Headers"))
	if !ok {
		panic("Headers constructor missing")
	}
	obj, err := ctor(nil)
	if err != nil {
		panic(err)
	}
	// ctor already registered an empty headerBag; replace with the provided one.
	iso.headersReg[obj] = h
	return obj
}

func (iso *Isolate) requestConstructor(call goja.ConstructorCall) *goja.Object {
	if len(call.Arguments) < 1 {
		panic(iso.vm.NewTypeError("Request requires a URL or Request input"))
	}
	r := &requestBag{
		method:  http.MethodGet,
		headers: newHeaderBag(),
	}

	input := call.Argument(0)
	if obj, ok := input.(*goja.Object); ok {
		if src, ok := iso.requestReg[obj]; ok {
			r.method = src.method
			r.url = src.url
			r.headers = src.headers.clone()
			r.body = src.body
		} else {
			r.url = input.String()
		}
	} else {
		r.url = input.String()
	}

	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		init := call.Argument(1).ToObject(iso.vm)
		if v := init.Get("method"); v != nil && !goja.IsUndefined(v) {
			r.method = strings.ToUpper(v.String())
		}
		if v := init.Get("headers"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			r.headers = newHeaderBag()
			if err := iso.fillHeaders(r.headers, v); err != nil {
				panic(iso.vm.NewTypeError(err.Error()))
			}
		}
		if v := init.Get("body"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			r.body = v.String()
		}
	}

	iso.requestReg[call.This] = r
	return nil
}

func (iso *Isolate) requestBagOf(call goja.FunctionCall) *requestBag {
	obj := call.This.ToObject(iso.vm)
	r, ok := iso.requestReg[obj]
	if !ok {
		panic(iso.vm.NewTypeError("Request method called on incompatible receiver"))
	}
	return r
}

func (iso *Isolate) requestGetMethod(call goja.FunctionCall) goja.Value {
	return iso.vm.ToValue(iso.requestBagOf(call).method)
}

func (iso *Isolate) requestGetURL(call goja.FunctionCall) goja.Value {
	return iso.vm.ToValue(iso.requestBagOf(call).url)
}

func (iso *Isolate) requestGetHeaders(call goja.FunctionCall) goja.Value {
	r := iso.requestBagOf(call)
	return iso.newHeadersObject(r.headers)
}

func (iso *Isolate) requestText(call goja.FunctionCall) goja.Value {
	r := iso.requestBagOf(call)
	return iso.resolvedStringPromise(r.body)
}

func (iso *Isolate) responseConstructor(call goja.ConstructorCall) *goja.Object {
	r := &responseBag{
		status:     200,
		statusText: "",
		headers:    newHeaderBag(),
	}
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		arg := call.Argument(0)
		if s, ok := arg.Export().(string); ok {
			r.body = s
		} else if obj, ok := arg.(*goja.Object); ok {
			// Prefer keeping the stream so the host can drain after async produce.
			if obj.Get("_orvalhoCollect") != nil && !goja.IsUndefined(obj.Get("_orvalhoCollect")) {
				r.bodyStream = obj
				// Best-effort sync snapshot (async start may still be pending).
				r.body = bodyArgToString(iso, arg)
			} else {
				r.body = bodyArgToString(iso, arg)
			}
		} else {
			r.body = bodyArgToString(iso, arg)
		}
	}
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		init := call.Argument(1).ToObject(iso.vm)
		if v := init.Get("status"); v != nil && !goja.IsUndefined(v) {
			status := int(v.ToInteger())
			if status < 200 || status > 599 {
				panic(iso.vm.NewTypeError(fmt.Sprintf("invalid status %d", status)))
			}
			r.status = status
		}
		if v := init.Get("statusText"); v != nil && !goja.IsUndefined(v) {
			r.statusText = v.String()
		}
		if v := init.Get("headers"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			if err := iso.fillHeaders(r.headers, v); err != nil {
				panic(iso.vm.NewTypeError(err.Error()))
			}
		}
	}
	iso.responseReg[call.This] = r
	return nil
}

func (iso *Isolate) responseBagOf(call goja.FunctionCall) *responseBag {
	obj := call.This.ToObject(iso.vm)
	r, ok := iso.responseReg[obj]
	if !ok {
		panic(iso.vm.NewTypeError("Response method called on incompatible receiver"))
	}
	return r
}

func (iso *Isolate) responseGetStatus(call goja.FunctionCall) goja.Value {
	return iso.vm.ToValue(iso.responseBagOf(call).status)
}

func (iso *Isolate) responseGetStatusText(call goja.FunctionCall) goja.Value {
	return iso.vm.ToValue(iso.responseBagOf(call).statusText)
}

func (iso *Isolate) responseGetOK(call goja.FunctionCall) goja.Value {
	s := iso.responseBagOf(call).status
	return iso.vm.ToValue(s >= 200 && s <= 299)
}

func (iso *Isolate) responseGetHeaders(call goja.FunctionCall) goja.Value {
	r := iso.responseBagOf(call)
	return iso.newHeadersObject(r.headers)
}

func (iso *Isolate) responseText(call goja.FunctionCall) goja.Value {
	r := iso.responseBagOf(call)
	if r.bodyStream != nil {
		if fn, ok := goja.AssertFunction(r.bodyStream.Get("_orvalhoCollect")); ok {
			v, err := fn(r.bodyStream)
			if err != nil {
				panic(iso.vm.ToValue(err.Error()))
			}
			// Collect returns a Promise<string>; update body when it settles (best-effort).
			return v
		}
	}
	return iso.resolvedStringPromise(r.body)
}

func (iso *Isolate) responseArrayBuffer(call goja.FunctionCall) goja.Value {
	r := iso.responseBodyString(call)
	// Expose as ArrayBuffer of UTF-8 bytes (good enough for binary if body was stored as Latin-1/bytes-in-string).
	buf := iso.vm.NewArrayBuffer([]byte(r))
	return iso.resolvedValuePromise(iso.vm.ToValue(buf))
}

func (iso *Isolate) responseJSON(call goja.FunctionCall) goja.Value {
	r := iso.responseBodyString(call)
	p, resolve, reject := iso.vm.NewPromise()
	parse, ok := goja.AssertFunction(iso.vm.Get("JSON").ToObject(iso.vm).Get("parse"))
	if !ok {
		_ = reject(iso.vm.NewTypeError("JSON.parse unavailable"))
		return iso.vm.ToValue(p)
	}
	parsed, err := parse(goja.Undefined(), iso.vm.ToValue(r))
	if err != nil {
		_ = reject(iso.vm.NewTypeError(err.Error()))
		return iso.vm.ToValue(p)
	}
	_ = resolve(parsed)
	return iso.vm.ToValue(p)
}

func (iso *Isolate) responseBodyString(call goja.FunctionCall) string {
	r := iso.responseBagOf(call)
	if r.bodyStream != nil {
		// Sync snapshot only; callers of arrayBuffer/json on streams should prefer text() first.
		if fn, ok := goja.AssertFunction(r.bodyStream.Get("_orvalhoText")); ok {
			if v, err := fn(r.bodyStream); err == nil && v != nil {
				return v.String()
			}
		}
	}
	return r.body
}

func (iso *Isolate) resolvedValuePromise(v goja.Value) goja.Value {
	p, resolve, _ := iso.vm.NewPromise()
	_ = resolve(v)
	return iso.vm.ToValue(p)
}

// bodyArgToString coerces a Response body init to a host string.
// Handles strings and our minimal ReadableStream (_orvalhoText).
func bodyArgToString(iso *Isolate, v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	if s, ok := v.Export().(string); ok {
		return s
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		// Avoid "[object Object]" for plain objects without stream helper.
		if exp := v.Export(); exp != nil {
			if b, ok := exp.([]byte); ok {
				return string(b)
			}
		}
		s := v.String()
		if s == "[object Object]" {
			return ""
		}
		return s
	}
	if fn, ok := goja.AssertFunction(obj.Get("_orvalhoText")); ok {
		out, err := fn(obj)
		if err == nil && out != nil {
			return out.String()
		}
	}
	s := obj.String()
	if s == "[object Object]" {
		return ""
	}
	return s
}

func (iso *Isolate) resolvedStringPromise(s string) goja.Value {
	p, resolve, _ := iso.vm.NewPromise()
	_ = resolve(s)
	return iso.vm.ToValue(p)
}

// MakeRequest builds a JS Request object from host data. Caller must not hold
// guest locks across network I/O; this only mutates isolate registry state.
func (iso *Isolate) MakeRequest(req HTTPRequest) (*goja.Object, error) {
	iso.mu.Lock()
	defer iso.mu.Unlock()
	return iso.makeRequestLocked(req)
}

func (iso *Isolate) makeRequestLocked(req HTTPRequest) (*goja.Object, error) {
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	h := newHeaderBag()
	for k, v := range req.Headers {
		h.set(k, v)
	}
	r := &requestBag{
		method:  strings.ToUpper(method),
		url:     req.URL,
		headers: h,
		body:    req.Body,
	}
	ctor, ok := goja.AssertConstructor(iso.vm.Get("Request"))
	if !ok {
		return nil, ErrRequestCtorMissing
	}
	// Construct with URL only, then overwrite registry entry with full state.
	obj, err := ctor(nil, iso.vm.ToValue(req.URL))
	if err != nil {
		return nil, err
	}
	iso.requestReg[obj] = r
	return obj, nil
}

// ReadResponse extracts status, headers, and body from a JS Response value.
func (iso *Isolate) ReadResponse(v goja.Value) (HTTPResponse, error) {
	iso.mu.Lock()
	defer iso.mu.Unlock()
	return iso.readResponseLocked(v)
}

func (iso *Isolate) readResponseLocked(v goja.Value) (HTTPResponse, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return HTTPResponse{}, ErrResponseNull
	}
	obj := v.ToObject(iso.vm)
	r, ok := iso.responseReg[obj]
	if !ok {
		return HTTPResponse{}, ErrNotAResponse
	}
	return HTTPResponse{
		Status:     r.status,
		StatusText: r.statusText,
		Headers:    r.headers.asMap(),
		Body:       r.body,
	}, nil
}
