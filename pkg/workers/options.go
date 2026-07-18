package workers

import (
	"context"
	"net/http"
	"time"
)

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
// bindings, and string env are all host-provided; the Workers kernel
// (Request/Response/Headers, timers) is ambient.
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
	Bindings map[string]Binding
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
	return o
}
