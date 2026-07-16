package js

import (
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

// Options configures isolate resource limits and host bindings.
// Zero-valued fields receive the documented defaults in [New].
type Options struct {
	// MaxPendingTimers is the hard cap on concurrent scheduled timers.
	// Scheduling past this limit throws a JS TypeError from setTimeout /
	// setInterval. Zero means DefaultMaxPendingTimers.
	MaxPendingTimers int

	// MaxTimersPerTick limits how many due timer callbacks fire in one Tick.
	// Excess remain queued and Tick returns more=true. Zero means
	// DefaultMaxTimersPerTick.
	MaxTimersPerTick int

	// Egress is the outbound fetch allowlist. Empty denies all destinations.
	Egress EgressList

	// HTTPClient is used for guest fetch. nil uses a default client with
	// redirect checks against Egress.
	HTTPClient *http.Client

	// FetchTimeout bounds each outbound fetch (default DefaultFetchTimeout).
	FetchTimeout time.Duration

	// Env is the CF-style string bag on guest env (from agents.<name>.env after CUE).
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
