package workers

import (
	"context"
	"errors"

	"github.com/dop251/goja"
)

// nodeDNSBinding materializes require("dns") / require("node:dns").
// lookup uses Options.Lookup; nil Lookup is denied unless the name is an IP.
type nodeDNSBinding struct{}

var _ Binding = nodeDNSBinding{}

func (nodeDNSBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"dns", "node:dns"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	obj := newNodeDNS(iso)
	// Official test-dns-promises-exists.js requires dns/promises first.
	iso.moduleCache["dns"] = obj
	iso.moduleCache["node:dns"] = obj
	if p := obj.Get("promises"); p != nil {
		iso.moduleCache["dns/promises"] = p
		iso.moduleCache["node:dns/promises"] = p
	}
	return obj, nil
}

// nodeDNSPromisesBinding materializes require("dns/promises").
type nodeDNSPromisesBinding struct{}

var _ Binding = nodeDNSPromisesBinding{}

func (nodeDNSPromisesBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	dns, err := (nodeDNSBinding{}).Materialize(iso)
	if err != nil {
		return nil, err
	}
	v := dns.Get("promises")
	o, ok := v.(*goja.Object)
	if !ok {
		return iso.vm.NewObject(), nil
	}
	return o, nil
}

type nodeDNS struct {
	iso     *Isolate
	order   string
	servers []string
}

func newNodeDNS(iso *Isolate) *goja.Object {
	n := &nodeDNS{iso: iso, order: "verbatim"}
	obj := iso.vm.NewObject()
	setDNSConstants(obj)
	mustSet(obj, "lookup", n.jsLookup)
	mustSet(obj, "setDefaultResultOrder", n.jsSetDefaultResultOrder)
	mustSet(obj, "getDefaultResultOrder", n.jsGetDefaultResultOrder)
	mustSet(obj, "setServers", n.jsSetServers)
	mustSet(obj, "getServers", n.jsGetServers)
	prom := newNodeDNSPromises(n)
	mustSet(obj, "promises", prom)
	mustSet(obj, "default", obj)
	return obj
}

func newNodeDNSPromises(n *nodeDNS) *goja.Object {
	obj := n.iso.vm.NewObject()
	setDNSConstants(obj)
	mustSet(obj, "lookup", n.jsPromisesLookup)
	mustSet(obj, "setDefaultResultOrder", n.jsSetDefaultResultOrder)
	mustSet(obj, "getDefaultResultOrder", n.jsGetDefaultResultOrder)
	mustSet(obj, "setServers", n.jsSetServers)
	mustSet(obj, "getServers", n.jsGetServers)
	mustSet(obj, "default", obj)
	return obj
}

func setDNSConstants(obj *goja.Object) {
	for k, v := range dnsErrorCodes {
		mustSet(obj, k, v)
	}
}

var dnsErrorCodes = map[string]string{
	"NODATA":               "ENODATA",
	"FORMERR":              "EFORMERR",
	"SERVFAIL":             "ESERVFAIL",
	"NOTFOUND":             "ENOTFOUND",
	"NOTIMP":               "ENOTIMP",
	"REFUSED":              "EREFUSED",
	"BADQUERY":             "EBADQUERY",
	"BADNAME":              "EBADNAME",
	"BADFAMILY":            "EBADFAMILY",
	"BADRESP":              "EBADRESP",
	"CONNREFUSED":          "ECONNREFUSED",
	"TIMEOUT":              "ETIMEOUT",
	"EOF":                  "EOF",
	"FILE":                 "EFILE",
	"NOMEM":                "ENOMEM",
	"DESTRUCTION":          "EDESTRUCTION",
	"BADSTR":               "EBADSTR",
	"BADFLAGS":             "EBADFLAGS",
	"NONAME":               "ENONAME",
	"BADHINTS":             "EBADHINTS",
	"NOTINITIALIZED":       "ENOTINITIALIZED",
	"LOADIPHLPAPI":         "ELOADIPHLPAPI",
	"ADDRGETNETWORKPARAMS": "EADDRGETNETWORKPARAMS",
	"CANCELLED":            "ECANCELLED",
}

func (n *nodeDNS) jsLookup(call goja.FunctionCall) goja.Value {
	hostname, family, all, order, cb := n.parseLookup(call, true)
	n.scheduleLookup(hostname, family, all, order, func(err goja.Value, addrs []LookupAddr) {
		if err != nil {
			_, _ = cb(goja.Undefined(), err)
			return
		}
		if all {
			_, _ = cb(goja.Undefined(), goja.Null(), n.addrsValue(addrs))
			return
		}
		_, _ = cb(goja.Undefined(), goja.Null(), n.iso.vm.ToValue(addrs[0].Address), n.iso.vm.ToValue(addrs[0].Family))
	})
	return goja.Undefined()
}

func (n *nodeDNS) jsPromisesLookup(call goja.FunctionCall) goja.Value {
	hostname, family, all, order, _ := n.parseLookup(call, false)
	p, resolve, reject := n.iso.vm.NewPromise()
	n.scheduleLookup(hostname, family, all, order, func(err goja.Value, addrs []LookupAddr) {
		if err != nil {
			reject(err)
			return
		}
		if all {
			resolve(n.addrsValue(addrs))
			return
		}
		resolve(n.addrObj(addrs[0]))
	})
	return n.iso.vm.ToValue(p)
}

func (n *nodeDNS) jsSetDefaultResultOrder(call goja.FunctionCall) goja.Value {
	s := jsToString(call.Argument(0))
	switch s {
	case "ipv4first", "ipv6first", "verbatim":
		n.order = s
		return goja.Undefined()
	}
	e := n.iso.vm.NewTypeError("The argument 'order' must be one of: 'ipv4first', 'ipv6first', 'verbatim'. Received " + s)
	_ = e.Set("code", "ERR_INVALID_ARG_VALUE")
	panic(e)
}

func (n *nodeDNS) jsGetDefaultResultOrder(goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(n.order)
}

func (n *nodeDNS) jsSetServers(call goja.FunctionCall) goja.Value {
	exported := call.Argument(0).Export()
	arr, ok := exported.([]any)
	if !ok {
		e := n.iso.vm.NewTypeError(`The "servers" argument must be an instance of Array`)
		_ = e.Set("code", "ERR_INVALID_ARG_TYPE")
		panic(e)
	}
	servers := make([]string, 0, len(arr))
	for _, a := range arr {
		s, ok := a.(string)
		if !ok {
			e := n.iso.vm.NewTypeError(`The "servers" argument must be an instance of Array`)
			_ = e.Set("code", "ERR_INVALID_ARG_TYPE")
			panic(e)
		}
		servers = append(servers, s)
	}
	n.servers = servers
	return goja.Undefined()
}

func (n *nodeDNS) jsGetServers(goja.FunctionCall) goja.Value {
	out := make([]any, len(n.servers))
	for i, s := range n.servers {
		out[i] = s
	}
	return n.iso.vm.ToValue(out)
}

func (n *nodeDNS) parseLookup(call goja.FunctionCall, needCB bool) (hostname string, family int, all bool, order string, cb goja.Callable) {
	order = n.order
	arg0 := call.Argument(0)
	if arg0 == nil || goja.IsUndefined(arg0) || goja.IsNull(arg0) {
		n.throwArgType("hostname")
	}
	if _, ok := arg0.Export().(string); !ok {
		n.throwArgType("hostname")
	}
	hostname = arg0.String()

	if len(call.Arguments) > 1 {
		a1 := call.Argument(1)
		if fn, ok := goja.AssertFunction(a1); ok {
			cb = fn
		} else if obj, ok := a1.(*goja.Object); ok {
			if v := obj.Get("family"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				family = int(v.ToInteger())
			}
			if v := obj.Get("all"); v != nil && !goja.IsUndefined(v) {
				all = v.ToBoolean()
			}
			if v := obj.Get("order"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) && v.String() != "" {
				order = v.String()
			} else if v := obj.Get("verbatim"); v != nil && !goja.IsUndefined(v) && v.ToBoolean() {
				order = "verbatim"
			}
		} else if !goja.IsUndefined(a1) && !goja.IsNull(a1) {
			family = int(a1.ToInteger())
		}
	}
	if cb == nil {
		cb = lastFunc(call)
	}
	if needCB && cb == nil {
		n.throwArgType("callback")
	}
	if family != 0 && family != 4 && family != 6 {
		e := n.iso.vm.NewTypeError("The property 'options.family' must be one of: 0, 4, 6. Received " + jsToString(call.Argument(1)))
		_ = e.Set("code", "ERR_INVALID_ARG_VALUE")
		panic(e)
	}
	return hostname, family, all, order, cb
}

func (n *nodeDNS) throwArgType(name string) {
	e := n.iso.vm.NewTypeError(`The "` + name + `" argument must be of type string`)
	if name == "callback" {
		e = n.iso.vm.NewTypeError(`The "callback" argument must be of type function`)
	}
	_ = e.Set("code", "ERR_INVALID_ARG_TYPE")
	panic(e)
}

func (n *nodeDNS) scheduleLookup(hostname string, family int, all bool, order string, fn func(err goja.Value, addrs []LookupAddr)) {
	var fire goja.Callable
	fire = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		err, addrs := n.runLookup(hostname, family, all, order)
		fn(err, addrs)
		return goja.Undefined(), nil
	}
	n.iso.timers.schedule(fire, nil, 0, 0, n.iso.now())
}

func (n *nodeDNS) runLookup(hostname string, family int, all bool, order string) (goja.Value, []LookupAddr) {
	if ipFam := nodeIsIP(hostname); ipFam != 0 {
		if family != 0 && family != ipFam {
			return n.lookupErr("ENOTFOUND", hostname), nil
		}
		return nil, []LookupAddr{{Address: hostname, Family: ipFam}}
	}
	if n.iso.opts.Lookup == nil {
		return n.lookupErr("EPERM", hostname), nil
	}
	ctx := n.iso.activeCtx
	if ctx == nil {
		ctx = context.Background()
	}
	addrs, err := n.iso.opts.Lookup(ctx, LookupReq{Hostname: hostname, Network: lookupNetwork(family)})
	if err != nil {
		code := "ENOTFOUND"
		if errors.Is(err, ErrLookupDenied) {
			code = "EPERM"
		}
		return n.lookupErr(code, hostname), nil
	}
	addrs = filterFamily(addrs, family)
	addrs = orderAddrs(addrs, order)
	if len(addrs) == 0 {
		return n.lookupErr("ENOTFOUND", hostname), nil
	}
	if !all {
		return nil, addrs[:1]
	}
	return nil, addrs
}

func (n *nodeDNS) lookupErr(code, hostname string) goja.Value {
	ctor, ok := goja.AssertConstructor(n.iso.vm.Get("Error"))
	if !ok {
		return n.iso.vm.NewGoError(ErrLookupDenied)
	}
	o, err := ctor(nil, n.iso.vm.ToValue("getaddrinfo "+code+" "+hostname))
	if err != nil {
		panic(err)
	}
	_ = o.Set("code", code)
	_ = o.Set("syscall", "getaddrinfo")
	_ = o.Set("hostname", hostname)
	return o
}

func (n *nodeDNS) addrObj(a LookupAddr) *goja.Object {
	o := n.iso.vm.NewObject()
	mustSet(o, "address", a.Address)
	mustSet(o, "family", a.Family)
	return o
}

func (n *nodeDNS) addrsValue(addrs []LookupAddr) goja.Value {
	out := make([]any, len(addrs))
	for i, a := range addrs {
		out[i] = n.addrObj(a)
	}
	return n.iso.vm.ToValue(out)
}

func lookupNetwork(family int) string {
	switch family {
	case 4:
		return "ip4"
	case 6:
		return "ip6"
	default:
		return "ip"
	}
}

func filterFamily(addrs []LookupAddr, family int) []LookupAddr {
	if family != 4 && family != 6 {
		return addrs
	}
	out := make([]LookupAddr, 0, len(addrs))
	for _, a := range addrs {
		if a.Family == family {
			out = append(out, a)
		}
	}
	return out
}

func orderAddrs(addrs []LookupAddr, order string) []LookupAddr {
	if order == "verbatim" || order == "" {
		return addrs
	}
	v4 := make([]LookupAddr, 0, len(addrs))
	v6 := make([]LookupAddr, 0, len(addrs))
	for _, a := range addrs {
		if a.Family == 6 {
			v6 = append(v6, a)
		} else {
			v4 = append(v4, a)
		}
	}
	if order == "ipv6first" {
		return append(v6, v4...)
	}
	return append(v4, v6...)
}
