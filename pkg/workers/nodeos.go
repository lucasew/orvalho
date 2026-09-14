package workers

import (
	"encoding/binary"
	"os"
	"runtime"
	"strings"

	"github.com/dop251/goja"
)

// nodeOSBinding materializes guest require("os") / require("node:os").
// platform/arch match process (wasi/wasm32 unless Options). homedir/tmpdir
// stay in the guest (HOME/TMPDIR or "/" and "/tmp"). Other facts are host
// (Hostname, NumCPU).
type nodeOSBinding struct{}

var _ Binding = nodeOSBinding{}

func (nodeOSBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"os", "node:os"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	return newNodeOS(iso), nil
}

type nodeOS struct {
	iso *Isolate
}

func newNodeOS(iso *Isolate) *goja.Object {
	n := &nodeOS{iso: iso}
	obj := iso.vm.NewObject()
	mustSet(obj, "arch", n.jsArch)
	mustSet(obj, "availableParallelism", n.jsAvailableParallelism)
	mustSet(obj, "cpus", n.jsCpus)
	mustSet(obj, "endianness", n.jsEndianness)
	mustSet(obj, "freemem", n.jsFreemem)
	mustSet(obj, "homedir", n.jsHomedir)
	mustSet(obj, "hostname", n.jsHostname)
	mustSet(obj, "loadavg", n.jsLoadavg)
	mustSet(obj, "networkInterfaces", n.jsNetworkInterfaces)
	mustSet(obj, "platform", n.jsPlatform)
	mustSet(obj, "release", n.jsRelease)
	mustSet(obj, "tmpdir", n.jsTmpdir)
	mustSet(obj, "totalmem", n.jsTotalmem)
	mustSet(obj, "type", n.jsType)
	mustSet(obj, "uptime", n.jsUptime)
	mustSet(obj, "userInfo", n.jsUserInfo)
	mustSet(obj, "constants", osConstants(iso.vm))
	mustSet(obj, "devNull", "/dev/null")

	// Configurable accessor: assignment throws TypeError; test-os-eol.js redefines it.
	getEOL := iso.vm.ToValue(func() string { return "\n" })
	setEOL := iso.vm.ToValue(func(goja.FunctionCall) goja.Value {
		panic(iso.vm.NewTypeError("Cannot assign to read only property 'EOL' of object"))
	})
	if err := obj.DefineAccessorProperty("EOL", getEOL, setEOL, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
		panic("goja accessor EOL: " + err.Error())
	}
	return obj
}

// POSIX signal numbers (Linux). human-signals destructures os.constants.signals.
var osSignalNumbers = []struct {
	name string
	num  int
}{
	{"SIGHUP", 1}, {"SIGINT", 2}, {"SIGQUIT", 3}, {"SIGILL", 4}, {"SIGTRAP", 5},
	{"SIGABRT", 6}, {"SIGIOT", 6}, {"SIGBUS", 7}, {"SIGFPE", 8}, {"SIGKILL", 9},
	{"SIGUSR1", 10}, {"SIGSEGV", 11}, {"SIGUSR2", 12}, {"SIGPIPE", 13}, {"SIGALRM", 14},
	{"SIGTERM", 15}, {"SIGSTKFLT", 16}, {"SIGCHLD", 17}, {"SIGCLD", 17}, {"SIGCONT", 18},
	{"SIGSTOP", 19}, {"SIGTSTP", 20}, {"SIGTTIN", 21}, {"SIGTTOU", 22}, {"SIGURG", 23},
	{"SIGXCPU", 24}, {"SIGXFSZ", 25}, {"SIGVTALRM", 26}, {"SIGPROF", 27}, {"SIGWINCH", 28},
	{"SIGIO", 29}, {"SIGPOLL", 29}, {"SIGPWR", 30}, {"SIGSYS", 31}, {"SIGUNUSED", 31},
}

func osConstants(vm *goja.Runtime) *goja.Object {
	c := vm.NewObject()
	signals := vm.NewObject()
	for _, s := range osSignalNumbers {
		mustSet(signals, s.name, s.num)
	}
	mustSet(c, "signals", signals)
	mustSet(c, "errno", vm.NewObject())
	mustSet(c, "priority", vm.NewObject())
	mustSet(c, "dlopen", vm.NewObject())
	return c
}

func (n *nodeOS) jsPlatform(goja.FunctionCall) goja.Value {
	p := n.iso.opts.Platform
	if p == "" {
		p = "wasi"
	}
	return n.iso.vm.ToValue(p)
}

func (n *nodeOS) jsArch(goja.FunctionCall) goja.Value {
	arch := n.iso.opts.Arch
	if arch == "" {
		arch = "wasm32"
	}
	return n.iso.vm.ToValue(arch)
}

func (n *nodeOS) jsHomedir(goja.FunctionCall) goja.Value {
	if home, ok := n.envString("HOME"); ok {
		return n.iso.vm.ToValue(home)
	}
	return n.iso.vm.ToValue(nodeHomedir())
}

func nodeHomedir() string {
	return "/"
}

func (n *nodeOS) jsTmpdir(goja.FunctionCall) goja.Value {
	dir, ok := n.envString("TMPDIR")
	if !ok {
		dir, ok = n.envString("TMP")
	}
	if !ok {
		dir, ok = n.envString("TEMP")
	}
	if !ok || dir == "" {
		dir = "/tmp"
	}
	if len(dir) > 1 && strings.HasSuffix(dir, "/") {
		dir = dir[:len(dir)-1]
	}
	return n.iso.vm.ToValue(dir)
}

func (n *nodeOS) envString(key string) (string, bool) {
	proc, ok := n.iso.vm.Get("process").(*goja.Object)
	if !ok {
		return "", false
	}
	env, ok := proc.Get("env").(*goja.Object)
	if !ok {
		return "", false
	}
	v := env.Get(key)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return "", false
	}
	s := v.String()
	if s == "" {
		return "", false
	}
	return s, true
}

func (n *nodeOS) jsHostname(goja.FunctionCall) goja.Value {
	h, err := os.Hostname()
	if err != nil || h == "" {
		h = "localhost"
	}
	return n.iso.vm.ToValue(h)
}

func (n *nodeOS) jsType(goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(nodeType())
}

func nodeType() string {
	switch runtime.GOOS {
	case "linux":
		return "Linux"
	case "darwin":
		return "Darwin"
	case "windows":
		return "Windows_NT"
	default:
		return runtime.GOOS
	}
}

func (n *nodeOS) jsRelease(goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue("0.0.0")
}

func (n *nodeOS) jsEndianness(goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(nodeEndianness())
}

func nodeEndianness() string {
	var buf [2]byte
	binary.NativeEndian.PutUint16(buf[:], 1)
	if buf[0] == 1 {
		return "LE"
	}
	return "BE"
}

func (n *nodeOS) jsCpus(goja.FunctionCall) goja.Value {
	count := runtime.NumCPU()
	if count < 1 {
		count = 1
	}
	items := make([]any, count)
	for i := 0; i < count; i++ {
		times := n.iso.vm.NewObject()
		mustSet(times, "user", 0)
		mustSet(times, "nice", 0)
		mustSet(times, "sys", 0)
		mustSet(times, "idle", 0)
		mustSet(times, "irq", 0)
		cpu := n.iso.vm.NewObject()
		mustSet(cpu, "model", "")
		mustSet(cpu, "speed", 0)
		mustSet(cpu, "times", times)
		items[i] = cpu
	}
	return n.iso.vm.ToValue(items)
}

func (n *nodeOS) jsNetworkInterfaces(goja.FunctionCall) goja.Value {
	return n.iso.vm.NewObject()
}

func (n *nodeOS) jsLoadavg(goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue([]float64{0, 0, 0})
}

func (n *nodeOS) jsTotalmem(goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(1)
}

func (n *nodeOS) jsFreemem(goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(1)
}

func (n *nodeOS) jsUptime(goja.FunctionCall) goja.Value {
	return n.iso.vm.ToValue(1)
}

func (n *nodeOS) jsAvailableParallelism(goja.FunctionCall) goja.Value {
	ncpu := runtime.NumCPU()
	if ncpu < 1 {
		ncpu = 1
	}
	return n.iso.vm.ToValue(ncpu)
}

func (n *nodeOS) jsUserInfo(goja.FunctionCall) goja.Value {
	username, _ := n.envString("USER")
	if username == "" {
		username, _ = n.envString("LOGNAME")
	}
	shell, _ := n.envString("SHELL")
	home := nodeHomedir()
	if h, ok := n.envString("HOME"); ok {
		home = h
	}
	obj := n.iso.vm.NewObject()
	mustSet(obj, "username", username)
	mustSet(obj, "uid", -1)
	mustSet(obj, "gid", -1)
	mustSet(obj, "shell", shell)
	mustSet(obj, "homedir", home)
	return obj
}
