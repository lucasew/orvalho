package workers

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

type httpJob struct {
	w    http.ResponseWriter
	r    *http.Request
	body []byte
	srv  *goja.Object
	done chan struct{}
}

// nodeHTTPBinding materializes require("http") / require("node:http").
// listen uses Options.Listen; nil Listen is denied.
type nodeHTTPBinding struct{}

var _ Binding = nodeHTTPBinding{}

func (nodeHTTPBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"http", "node:http"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeHTTP(iso, false), nil
}

// nodeHTTPSBinding materializes require("https"). Same server; TLS is host Listen.
type nodeHTTPSBinding struct{}

var _ Binding = nodeHTTPSBinding{}

func (nodeHTTPSBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"https", "node:https"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeHTTP(iso, true), nil
}

type nodeHTTP struct {
	iso   *Isolate
	https bool
}

func withPrototype(iso *Isolate, fn func(goja.FunctionCall) goja.Value) *goja.Object {
	obj := iso.vm.ToValue(fn).ToObject(iso.vm)
	if p := obj.Get("prototype"); p == nil || goja.IsUndefined(p) || goja.IsNull(p) {
		proto := iso.vm.NewObject()
		mustSet(obj, "prototype", proto)
		mustSet(proto, "constructor", obj)
	}
	return obj
}

func newNodeHTTP(iso *Isolate, https bool) *goja.Object {
	n := &nodeHTTP{iso: iso, https: https}
	obj := iso.vm.NewObject()
	server := withPrototype(iso, n.jsCreateServer)
	mustSet(obj, "createServer", server)
	mustSet(obj, "Server", server)
	mustSet(obj, "IncomingMessage", n.jsIncoming)
	mustSet(obj, "ServerResponse", n.jsOutgoing)
	agent := iso.vm.ToValue(n.jsAgent).ToObject(iso.vm)
	if p := agent.Get("prototype"); p != nil {
		if proto, ok := p.(*goja.Object); ok {
			n.installAgentProto(proto)
		}
	}
	mustSet(obj, "Agent", agent)
	ga := iso.vm.NewObject()
	n.initAgent(ga, goja.Undefined())
	mustSet(obj, "globalAgent", ga)
	mustSet(obj, "METHODS", httpMethods)
	mustSet(obj, "STATUS_CODES", statusCodesObj(iso.vm))
	mustSet(obj, "get", n.jsDeniedOut)
	mustSet(obj, "request", n.jsDeniedOut)
	mustSet(obj, "default", obj)
	return obj
}

var httpMethods = []string{
	"ACL", "BIND", "CHECKOUT", "CONNECT", "COPY", "DELETE", "GET", "HEAD",
	"LINK", "LOCK", "M-SEARCH", "MERGE", "MKACTIVITY", "MKCALENDAR", "MKCOL",
	"MOVE", "NOTIFY", "OPTIONS", "PATCH", "POST", "PROPFIND", "PROPPATCH",
	"PURGE", "PUT", "QUERY", "REBIND", "REPORT", "SEARCH", "SOURCE",
	"SUBSCRIBE", "TRACE", "UNBIND", "UNLINK", "UNLOCK", "UNSUBSCRIBE",
}

func statusCodesObj(vm *goja.Runtime) *goja.Object {
	o := vm.NewObject()
	for _, pair := range httpStatusPairs {
		mustSet(o, strconv.Itoa(pair.code), pair.text)
	}
	return o
}

var httpStatusPairs = []struct {
	code int
	text string
}{
	{100, "Continue"}, {101, "Switching Protocols"},
	{200, "OK"}, {201, "Created"}, {202, "Accepted"}, {204, "No Content"},
	{206, "Partial Content"},
	{301, "Moved Permanently"}, {302, "Found"}, {304, "Not Modified"},
	{307, "Temporary Redirect"}, {308, "Permanent Redirect"},
	{400, "Bad Request"}, {401, "Unauthorized"}, {403, "Forbidden"},
	{404, "Not Found"}, {405, "Method Not Allowed"}, {409, "Conflict"},
	{410, "Gone"}, {413, "Payload Too Large"}, {418, "I'm a teapot"},
	{429, "Too Many Requests"},
	{500, "Internal Server Error"}, {501, "Not Implemented"},
	{502, "Bad Gateway"}, {503, "Service Unavailable"},
}

func (n *nodeHTTP) jsDeniedOut(call goja.FunctionCall) goja.Value {
	n.throwDenied("request")
	return goja.Undefined()
}

func (n *nodeHTTP) jsIncoming(call goja.ConstructorCall) *goja.Object {
	return n.iso.vm.NewObject()
}

func (n *nodeHTTP) jsOutgoing(call goja.ConstructorCall) *goja.Object {
	return n.iso.vm.NewObject()
}

func (n *nodeHTTP) jsAgent(call goja.ConstructorCall) *goja.Object {
	n.initAgent(call.This, call.Argument(0))
	return call.This
}

func (n *nodeHTTP) initAgent(this *goja.Object, optsVal goja.Value) {
	attachEmitter(this)
	opts := n.iso.vm.NewObject()
	if o, ok := optsVal.(*goja.Object); ok {
		opts = o
	}
	mustSet(this, "options", opts)
	mustSet(this, "sockets", n.iso.vm.NewObject())
	mustSet(this, "freeSockets", n.iso.vm.NewObject())
	mustSet(this, "requests", n.iso.vm.NewObject())
}

func (n *nodeHTTP) installAgentProto(proto *goja.Object) {
	attachEmitter(proto)
	mustSet(proto, "keepSocketAlive", func(goja.FunctionCall) goja.Value {
		return n.iso.vm.ToValue(true)
	})
	mustSet(proto, "reuseSocket", func(goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	mustSet(proto, "destroy", func(goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	mustSet(proto, "getName", func(goja.FunctionCall) goja.Value {
		return n.iso.vm.ToValue("")
	})
	mustSet(proto, "createConnection", n.jsDeniedOut)
	mustSet(proto, "addRequest", func(goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
}

func (n *nodeHTTP) jsCreateServer(call goja.FunctionCall) goja.Value {
	handler := lastFunc(call)
	srv := n.iso.vm.NewObject()
	listeners := map[string][]goja.Callable{}
	if handler != nil {
		listeners["request"] = append(listeners["request"], handler)
	}
	var ln net.Listener
	var closeOnce sync.Once
	mustSet(srv, "on", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				ev := call.Argument(0).String()
				listeners[ev] = append(listeners[ev], fn)
			}
		}
		return srv
	})
	mustSet(srv, "prependListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				ev := call.Argument(0).String()
				listeners[ev] = append([]goja.Callable{fn}, listeners[ev]...)
			}
		}
		return srv
	})
	mustSet(srv, "once", srv.Get("on"))
	mustSet(srv, "removeListener", func(goja.FunctionCall) goja.Value { return srv })
	mustSet(srv, "emit", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		ev := call.Argument(0).String()
		var args []goja.Value
		if len(call.Arguments) > 1 {
			args = call.Arguments[1:]
		}
		for _, fn := range listeners[ev] {
			_, _ = fn(srv, args...)
		}
		return goja.Undefined()
	})
	mustSet(srv, "unref", func(goja.FunctionCall) goja.Value { return srv })
	mustSet(srv, "ref", func(goja.FunctionCall) goja.Value { return srv })
	mustSet(srv, "listen", func(call goja.FunctionCall) goja.Value {
		n.startListen(srv, &ln, &closeOnce, call)
		return srv
	})
	mustSet(srv, "close", func(call goja.FunctionCall) goja.Value {
		if ln != nil {
			closeOnce.Do(func() {
				_ = ln.Close()
				n.iso.listeners--
			})
		}
		cb := lastFunc(call)
		if cb != nil {
			n.iso.timers.schedule(cb, nil, 0, 0, n.iso.now())
		}
		return srv
	})
	mustSet(srv, "address", func(goja.FunctionCall) goja.Value {
		if ln == nil {
			return goja.Null()
		}
		a := ln.Addr()
		if a == nil {
			return goja.Null()
		}
		host, port, _ := net.SplitHostPort(a.String())
		o := n.iso.vm.NewObject()
		mustSet(o, "address", host)
		mustSet(o, "family", "IPv4")
		p, _ := strconv.Atoi(port)
		mustSet(o, "port", p)
		return o
	})
	mustSet(srv, "listening", false)
	mustSet(srv, "unref", func(goja.FunctionCall) goja.Value { return srv })
	mustSet(srv, "ref", func(goja.FunctionCall) goja.Value { return srv })
	return srv
}

func (n *nodeHTTP) startListen(srv *goja.Object, ln *net.Listener, once *sync.Once, call goja.FunctionCall) {
	if n.iso.opts.Listen == nil {
		n.throwDenied("listen")
	}
	port, host, cb := listenArgs(call)
	if host == "" {
		host = "127.0.0.1"
	}
	ctx := n.iso.activeCtx
	if ctx == nil {
		ctx = context.Background()
	}
	l, err := n.iso.opts.Listen(ctx, ListenReq{
		Network: "tcp",
		Address: net.JoinHostPort(host, strconv.Itoa(port)),
	})
	if err != nil {
		panic(n.iso.vm.NewGoError(err))
	}
	*ln = l
	n.iso.listeners++
	mustSet(srv, "listening", true)
	go n.serve(l, srv)
	if cb != nil {
		n.iso.timers.schedule(cb, nil, 0, 0, n.iso.now())
	}
	n.emit(srv, "listening")
}

func (n *nodeHTTP) serve(ln net.Listener, srv *goja.Object) {
	_ = http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		job := &httpJob{w: w, r: r, body: body, srv: srv, done: make(chan struct{})}
		select {
		case n.iso.httpCh <- job:
			<-job.done
		case <-r.Context().Done():
		}
	}))
}

func (iso *Isolate) dispatchHTTP(job *httpJob) {
	if job == nil {
		return
	}
	n := &nodeHTTP{iso: iso}
	req := n.makeReq(job.r, job.body)
	res := n.makeRes(job.w, job.done)
	n.emit(job.srv, "request", req, res)
}

func (n *nodeHTTP) makeReq(r *http.Request, body []byte) *goja.Object {
	req := n.iso.vm.NewObject()
	attachEmitter(req)
	mustSet(req, "method", r.Method)
	mustSet(req, "url", r.URL.RequestURI())
	mustSet(req, "httpVersion", r.Proto)
	hdr := n.iso.vm.NewObject()
	for k, vs := range r.Header {
		mustSet(hdr, strings.ToLower(k), strings.Join(vs, ", "))
	}
	mustSet(req, "headers", hdr)
	mustSet(req, "socket", n.iso.vm.NewObject())
	var fire goja.Callable
	fire = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		if len(body) > 0 {
			n.emit(req, "data", n.iso.vm.ToValue(string(body)))
		}
		n.emit(req, "end")
		return goja.Undefined(), nil
	}
	n.iso.timers.schedule(fire, nil, 0, 0, n.iso.now())
	return req
}

type httpResState struct {
	w      http.ResponseWriter
	status int
	hdr    http.Header
	wrote  bool
	ended  bool
	mu     sync.Mutex
}

func (n *nodeHTTP) makeRes(w http.ResponseWriter, done chan struct{}) *goja.Object {
	st := &httpResState{w: w, status: 200, hdr: make(http.Header)}
	var finish sync.Once
	res := n.iso.vm.NewObject()
	attachEmitter(res)
	mustSet(res, "statusCode", 200)
	mustSet(res, "setHeader", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			st.hdr.Set(call.Argument(0).String(), call.Argument(1).String())
		}
		return goja.Undefined()
	})
	mustSet(res, "getHeader", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		v := st.hdr.Get(call.Argument(0).String())
		if v == "" {
			return goja.Undefined()
		}
		return n.iso.vm.ToValue(v)
	})
	mustSet(res, "writeHead", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			st.status = int(call.Argument(0).ToInteger())
			mustSet(res, "statusCode", st.status)
		}
		if len(call.Arguments) > 1 {
			if obj, ok := call.Argument(1).(*goja.Object); ok {
				for _, k := range obj.Keys() {
					st.hdr.Set(k, obj.Get(k).String())
				}
			}
		}
		return res
	})
	flush := func(chunk string) {
		st.mu.Lock()
		defer st.mu.Unlock()
		if !st.wrote {
			for k, vs := range st.hdr {
				for _, v := range vs {
					st.w.Header().Add(k, v)
				}
			}
			st.w.WriteHeader(st.status)
			st.wrote = true
		}
		if chunk != "" {
			_, _ = io.WriteString(st.w, chunk)
		}
	}
	mustSet(res, "write", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			flush(call.Argument(0).String())
		}
		return n.iso.vm.ToValue(true)
	})
	mustSet(res, "end", func(call goja.FunctionCall) goja.Value {
		chunk := ""
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			chunk = call.Argument(0).String()
		}
		flush(chunk)
		st.ended = true
		finish.Do(func() { close(done) })
		n.emit(res, "finish")
		return res
	})
	return res
}

func (n *nodeHTTP) emit(obj *goja.Object, ev string, args ...goja.Value) {
	emit := obj.Get("emit")
	fn, ok := goja.AssertFunction(emit)
	if !ok {
		return
	}
	all := append([]goja.Value{n.iso.vm.ToValue(ev)}, args...)
	_, _ = fn(obj, all...)
}

func (n *nodeHTTP) throwDenied(op string) {
	ctor, ok := goja.AssertConstructor(n.iso.vm.Get("Error"))
	if !ok {
		panic(n.iso.vm.NewGoError(ErrListenDenied))
	}
	o, err := ctor(nil, n.iso.vm.ToValue(op+" EPERM"))
	if err != nil {
		panic(err)
	}
	_ = o.Set("code", "EPERM")
	_ = o.Set("syscall", op)
	panic(o)
}

func listenArgs(call goja.FunctionCall) (port int, host string, cb goja.Callable) {
	cb = lastFunc(call)
	if len(call.Arguments) == 0 {
		return 0, "", cb
	}
	a0 := call.Argument(0)
	if obj, ok := a0.(*goja.Object); ok {
		if _, isFn := goja.AssertFunction(a0); !isFn {
			if v := obj.Get("port"); v != nil && !goja.IsUndefined(v) {
				port = int(v.ToInteger())
			}
			if v := obj.Get("host"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				host = v.String()
			}
			return port, host, cb
		}
	}
	port = int(a0.ToInteger())
	if len(call.Arguments) > 1 {
		a1 := call.Argument(1)
		if _, isFn := goja.AssertFunction(a1); !isFn && !goja.IsUndefined(a1) && !goja.IsNull(a1) {
			if s, ok := a1.Export().(string); ok {
				host = s
			}
		}
	}
	return port, host, cb
}
