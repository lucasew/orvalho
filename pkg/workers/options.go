package workers

import (
	"context"
	"io"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/lucasew/orvalho/pkg/imports"
)

// SpawnReq is one guest child_process spawn.
type SpawnReq struct {
	File string
	Args []string
	Cwd  string
	Env  []string
	// Stdin/Stdout/Stderr are guest pipes. The host attaches them to the
	// child; the isolate never calls os. Nil means the host may inherit.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// SpawnWait is the child's exit.
type SpawnWait struct {
	Code int
}

// Spawned is one running child. Done closes after Wait.
type Spawned interface {
	PID() int
	Done() <-chan SpawnWait
	Kill() error
}

// SpawnFunc starts a child. Nil Options.Spawn means spawn is denied.
type SpawnFunc func(ctx context.Context, req SpawnReq) (Spawned, error)

// DialReq is one guest net.connect.
type DialReq struct {
	Network string
	Address string
}

// DialFunc opens a connection. Nil Options.Dial means connect is denied.
// The isolate never calls net.Dial; the host supplies the conn.
type DialFunc func(ctx context.Context, req DialReq) (net.Conn, error)

// LookupReq is one guest dns.lookup.
type LookupReq struct {
	Hostname string
	Network  string
}

// LookupAddr is one resolved address.
type LookupAddr struct {
	Address string
	Family  int
}

// LookupFunc resolves a name. Nil Options.Lookup means lookup is denied
// unless the name is already an IP. The isolate never calls Resolver.
type LookupFunc func(ctx context.Context, req LookupReq) ([]LookupAddr, error)

// ListenReq is one guest http.Server.listen / net.Server.listen.
type ListenReq struct {
	Network string
	Address string
}

// ListenFunc binds a listener. Nil Options.Listen means listen is denied.
// The isolate never calls net.Listen; the host supplies the listener.
type ListenFunc func(ctx context.Context, req ListenReq) (net.Listener, error)

// Default resource caps. Documented for hosts and enforced in the isolate.
const (
	// DefaultMaxPendingTimers is the default hard cap on concurrent
	// setTimeout/setInterval entries per isolate.
	DefaultMaxPendingTimers = 10_000

	// DefaultMaxTimersPerTick is the default maximum number of due timer
	// callbacks executed in a single Tick.
	DefaultMaxTimersPerTick = 1_000
)

// FetchFunc is the host-injected implementation of guest global fetch.
// When Options.Fetch is nil, the guest has no outbound fetch capability.
type FetchFunc func(ctx context.Context, req *http.Request) (*http.Response, error)

// Options configures isolate resource limits and host capabilities.
// Zero-valued fields receive the documented defaults in [New].
//
// Capability rule: what is not injected is not allowed. Outbound network,
// bindings, string env, and import specifiers are all host-provided; the
// Workers kernel (Request/Response/Headers, timers) is ambient.
type Options struct {
	// MaxPendingTimers is the hard cap on concurrent scheduled timers.
	// Scheduling past this limit throws a JS TypeError from setTimeout /
	// setInterval. Zero means DefaultMaxPendingTimers.
	MaxPendingTimers int

	// MaxTimersPerTick limits how many due timer callbacks fire in one Tick.
	// Excess remain queued and Tick returns more=true. Zero means
	// DefaultMaxTimersPerTick.
	MaxTimersPerTick int

	// Fetch installs guest global fetch when non-nil. Nil means the guest
	// has no fetch (not injected ⇒ not allowed). Use [HTTPFetch] for
	// allowlisted net/http-backed fetch.
	Fetch FetchFunc

	// FetchTimeout bounds each outbound fetch when using [HTTPFetch]
	// defaults (also honored by the built-in fetch wrapper). Zero means
	// DefaultFetchTimeout.
	FetchTimeout time.Duration

	// Env is the CF-style string bag on guest env.
	// Keys must not clash with Bindings names.
	Env map[string]string

	// Bindings are named host objects on guest env (assets drivers, later HAL).
	// Materialized on each Fetch into the env object passed to default.fetch.
	// They are not visible to require; add an Imports handler that
	// returns a Binding to expose a specifier.
	Bindings map[string]Binding

	// Imports is the require / getBuiltinModule resolve chain.
	// Handlers return a Binding or an [imports.Script]. Not claimed
	// means not found. Guest JS MUST NOT reach host I/O except through
	// a Binding the chain returned.
	Imports []imports.Handler[any]

	// FS is the guest node:fs / fs mount (Go fs.FS only). Nil means no
	// tree: require("fs") still exists and I/O fails (ENOENT). Never
	// falls through to the host disk.
	FS fs.FS

	// Argv is process.argv for ScriptMain. Empty means []string{"orvalho"}.
	Argv []string

	// ProcessEnv is process.env. Nil means an empty env (not os.Environ).
	ProcessEnv map[string]string

	// Cwd is process.cwd(). Guest chdir updates this copy only.
	// Empty means ".".
	Cwd string

	// Platform is process.platform and os.platform(). Empty means "wasi"
	// (not the host GOOS). Native addons are not a goal.
	Platform string

	// Arch is process.arch and os.arch(). Empty means "wasm32"
	// (INV-18). Native addons are not a goal.
	Arch string

	// PID is process.pid. Zero is a valid injected pid.
	PID int

	// ExecPath is process.execPath. Empty means "orvalho".
	ExecPath string

	// Spawn is child_process.spawn / exec / fork. Nil means the module
	// exists and spawn fails (not injected).
	Spawn SpawnFunc

	// Dial is net.connect / createConnection / Socket.connect.
	// Nil means the module exists and connect fails (not injected).
	Dial DialFunc

	// Lookup is dns.lookup / dns.promises.lookup. Nil means the module
	// exists and lookup fails unless the name is already an IP.
	Lookup LookupFunc

	// Listen is http.Server.listen / https.Server.listen. Nil means the
	// module exists and listen fails (not injected).
	Listen ListenFunc

	// PrepareSource rewrites guest source after shebang strip and before
	// the CommonJS wrap (ESM downlevel). Nil means no extra rewrite.
	PrepareSource func(source, file string) (string, error)

	// Trace, if set, receives one-line progress (require, eval, listen,
	// spawn). Nil is silent. The CLI wires this from --verbose / -v.
	Trace func(format string, args ...any)
}

func (o Options) withDefaults() Options {
	if o.MaxPendingTimers <= 0 {
		o.MaxPendingTimers = DefaultMaxPendingTimers
	}
	if o.MaxTimersPerTick <= 0 {
		o.MaxTimersPerTick = DefaultMaxTimersPerTick
	}
	if o.FetchTimeout <= 0 {
		o.FetchTimeout = DefaultFetchTimeout
	}
	if o.Imports != nil {
		o.Imports = append([]imports.Handler[any](nil), o.Imports...)
	}
	if o.Argv != nil {
		o.Argv = append([]string(nil), o.Argv...)
	}
	if o.ProcessEnv != nil {
		env := make(map[string]string, len(o.ProcessEnv))
		for k, v := range o.ProcessEnv {
			env[k] = v
		}
		o.ProcessEnv = env
	}
	return o
}
