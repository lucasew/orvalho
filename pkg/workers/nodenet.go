package workers

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

// nodeNetBinding materializes require("net") / require("node:net").
// connect uses Options.Dial; nil Dial is denied. isIP is local.
type nodeNetBinding struct{}

var _ Binding = nodeNetBinding{}

func (nodeNetBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"net", "node:net"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeNet(iso), nil
}

type nodeNet struct {
	iso *Isolate
}

func newNodeNet(iso *Isolate) *goja.Object {
	n := &nodeNet{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "isIP", n.jsIsIP)
	mustSet(obj, "isIPv4", n.jsIsIPv4)
	mustSet(obj, "isIPv6", n.jsIsIPv6)
	mustSet(obj, "connect", n.jsConnect)
	mustSet(obj, "createConnection", n.jsConnect)
	mustSet(obj, "Socket", n.jsSocket)
	mustSet(obj, "createServer", n.jsCreateServer)
	mustSet(obj, "default", obj)
	return obj
}

func (n *nodeNet) jsIsIP(call goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(nodeIsIP(jsToString(call.Argument(0))))
}

func (n *nodeNet) jsIsIPv4(call goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(nodeIsIP(jsToString(call.Argument(0))) == 4)
}

func (n *nodeNet) jsIsIPv6(call goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(nodeIsIP(jsToString(call.Argument(0))) == 6)
}

func jsToString(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return v.String()
}

func nodeIsIP(s string) int {
	if s == "" {
		return 0
	}
	if i := strings.LastIndex(s, "%"); i >= 0 && strings.Contains(s[:i], ":") {
		zone := s[i+1:]
		if zone == "" || strings.ContainsAny(zone, "@") {
			return 0
		}
		s = s[:i]
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return 0
	}
	if strings.Contains(s, ":") {
		return 6
	}
	return 4
}

func (n *nodeNet) jsConnect(call goja.FunctionCall) goja.Value {
	sock := n.newSocket()
	n.startConnect(sock, call)
	return sock
}

func (n *nodeNet) jsSocket(call goja.ConstructorCall) *goja.Object {
	return n.initSocket(call.This)
}

func (n *nodeNet) newSocket() *goja.Object {
	return n.initSocket(n.iso.vm.NewObject())
}

func (n *nodeNet) initSocket(sock *goja.Object) *goja.Object {
	listeners := map[string][]goja.Callable{}
	mustSet(sock, "connecting", false)
	mustSet(sock, "destroyed", false)
	mustSet(sock, "on", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return sock
		}
		fn, ok := goja.AssertFunction(call.Argument(1))
		if ok {
			ev := call.Argument(0).String()
			listeners[ev] = append(listeners[ev], fn)
		}
		return sock
	})
	mustSet(sock, "once", sock.Get("on"))
	mustSet(sock, "emit", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		ev := call.Argument(0).String()
		var args []goja.Value
		if len(call.Arguments) > 1 {
			args = call.Arguments[1:]
		}
		for _, fn := range listeners[ev] {
			_, _ = fn(sock, args...)
		}
		return goja.Undefined()
	})
	mustSet(sock, "write", func(goja.FunctionCall) goja.Value { return n.iso.vm.ToValue(true) })
	mustSet(sock, "end", func(goja.FunctionCall) goja.Value { return sock })
	mustSet(sock, "destroy", func(goja.FunctionCall) goja.Value {
		mustSet(sock, "destroyed", true)
		return sock
	})
	mustSet(sock, "setTimeout", func(goja.FunctionCall) goja.Value { return sock })
	mustSet(sock, "setNoDelay", func(goja.FunctionCall) goja.Value { return sock })
	mustSet(sock, "setKeepAlive", func(goja.FunctionCall) goja.Value { return sock })
	mustSet(sock, "connect", func(call goja.FunctionCall) goja.Value {
		n.startConnect(sock, call)
		return sock
	})
	return sock
}

func (n *nodeNet) startConnect(sock *goja.Object, call goja.FunctionCall) {
	network, address := dialArgs(call)
	cb := lastFunc(call)
	if cb != nil {
		on := sock.Get("on")
		if fn, ok := goja.AssertFunction(on); ok {
			_, _ = fn(sock, n.iso.vm.ToValue("connect"), n.iso.vm.ToValue(cb))
		}
	}
	if n.iso.opts.Dial == nil {
		n.throwDenied(address)
	}
	ctx := n.iso.activeCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if network == "" {
		network = "tcp"
	}
	mustSet(sock, "connecting", true)
	conn, err := n.iso.opts.Dial(ctx, DialReq{Network: network, Address: address})
	if err != nil {
		panic(n.iso.vm.NewGoError(err))
	}
	mustSet(sock, "connecting", false)
	if ra := conn.RemoteAddr(); ra != nil {
		host, port, _ := net.SplitHostPort(ra.String())
		mustSet(sock, "remoteAddress", host)
		mustSet(sock, "remotePort", atoiPort(port))
	}
	_ = conn
	var fire goja.Callable
	fire = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		n.emit(sock, "connect")
		return goja.Undefined(), nil
	}
	n.iso.timers.schedule(fire, nil, 0, 0, n.iso.now())
}

func atoiPort(s string) int {
	p, _ := strconv.Atoi(s)
	return p
}

func (n *nodeNet) emit(sock *goja.Object, ev string, args ...goja.Value) {
	emit := sock.Get("emit")
	fn, ok := goja.AssertFunction(emit)
	if !ok {
		return
	}
	all := append([]goja.Value{n.iso.vm.ToValue(ev)}, args...)
	_, _ = fn(sock, all...)
}

func (n *nodeNet) jsCreateServer(call goja.FunctionCall) goja.Value {
	srv := n.iso.vm.NewObject()
	listeners := map[string][]goja.Callable{}
	if cb := lastFunc(call); cb != nil {
		listeners["connection"] = append(listeners["connection"], cb)
	}
	var ln net.Listener
	var closeOnce sync.Once
	emit := func(ev string, args ...goja.Value) {
		for _, fn := range listeners[ev] {
			_, _ = fn(srv, args...)
		}
	}
	mustSet(srv, "on", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return srv
		}
		fn, ok := goja.AssertFunction(call.Argument(1))
		if ok {
			ev := call.Argument(0).String()
			listeners[ev] = append(listeners[ev], fn)
		}
		return srv
	})
	mustSet(srv, "once", srv.Get("on"))
	mustSet(srv, "unref", func(goja.FunctionCall) goja.Value { return srv })
	mustSet(srv, "ref", func(goja.FunctionCall) goja.Value { return srv })
	mustSet(srv, "listen", func(call goja.FunctionCall) goja.Value {
		n.startListen(srv, &ln, &closeOnce, call, emit)
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
	return srv
}

func (n *nodeNet) startListen(srv *goja.Object, ln *net.Listener, once *sync.Once, call goja.FunctionCall, emit func(string, ...goja.Value)) {
	port, host, cb := listenArgs(call)
	if host == "" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	n.iso.trace("net listen %s", addr)
	fail := func(code string) {
		n.iso.trace("net listen fail %s %s", addr, code)
		err := n.listenErr(code, addr)
		emit("error", err)
	}
	if n.iso.opts.Listen == nil {
		fail("EADDRNOTAVAIL")
		return
	}
	ctx := n.iso.activeCtx
	if ctx == nil {
		ctx = context.Background()
	}
	l, err := n.iso.opts.Listen(ctx, ListenReq{Network: "tcp", Address: addr})
	if err != nil {
		code := "EADDRINUSE"
		if errors.Is(err, ErrListenDenied) {
			code = "EADDRNOTAVAIL"
		}
		fail(code)
		return
	}
	n.iso.trace("net listen ok %s", l.Addr())
	*ln = l
	n.iso.listeners++
	if cb != nil {
		n.iso.timers.schedule(cb, nil, 0, 0, n.iso.now())
	}
	emit("listening")
}

func (n *nodeNet) listenErr(code, addr string) *goja.Object {
	ctor, ok := goja.AssertConstructor(n.iso.vm.Get("Error"))
	if !ok {
		e := n.iso.vm.NewGoError(errors.New(code + " " + addr))
		_ = e.Set("code", code)
		return e
	}
	o, err := ctor(nil, n.iso.vm.ToValue(code+" listen "+addr))
	if err != nil {
		panic(err)
	}
	_ = o.Set("code", code)
	_ = o.Set("syscall", "listen")
	_ = o.Set("address", addr)
	return o
}

func (n *nodeNet) throwDenied(addr string) {
	ctor, ok := goja.AssertConstructor(n.iso.vm.Get("Error"))
	if !ok {
		panic(n.iso.vm.NewGoError(ErrDialDenied))
	}
	o, err := ctor(nil, n.iso.vm.ToValue("connect EPERM "+addr))
	if err != nil {
		panic(err)
	}
	_ = o.Set("code", "EPERM")
	_ = o.Set("syscall", "connect")
	_ = o.Set("address", addr)
	panic(o)
}

func dialArgs(call goja.FunctionCall) (network, address string) {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) {
		return "tcp", ""
	}
	arg := call.Argument(0)
	if obj, ok := arg.(*goja.Object); ok {
		if _, isFn := goja.AssertFunction(arg); !isFn {
			if v := obj.Get("path"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) && v.String() != "" {
				return "unix", v.String()
			}
			host := "localhost"
			if v := obj.Get("host"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) && v.String() != "" {
				host = v.String()
			}
			port := "0"
			if v := obj.Get("port"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				port = strconv.Itoa(int(v.ToInteger()))
			}
			return "tcp", net.JoinHostPort(host, port)
		}
	}
	port := arg.String()
	host := "localhost"
	if len(call.Arguments) > 1 {
		a1 := call.Argument(1)
		if _, isFn := goja.AssertFunction(a1); !isFn && !goja.IsUndefined(a1) && !goja.IsNull(a1) {
			host = a1.String()
		}
	}
	return "tcp", net.JoinHostPort(host, port)
}
