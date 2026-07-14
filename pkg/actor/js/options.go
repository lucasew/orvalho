package js

// Default resource caps. Documented for hosts and enforced in the isolate.
const (
	// DefaultMaxPendingTimers is the default hard cap on concurrent
	// setTimeout/setInterval entries per isolate.
	DefaultMaxPendingTimers = 10_000

	// DefaultMaxTimersPerTick is the default maximum number of due timer
	// callbacks executed in a single Tick.
	DefaultMaxTimersPerTick = 1_000
)

// Options configures isolate resource limits.
// Zero-valued fields receive the documented defaults in [New].
type Options struct {
	// MaxPendingTimers is the hard cap on concurrent scheduled timers.
	// Scheduling past this limit throws a JS RangeError from setTimeout /
	// setInterval. Zero means DefaultMaxPendingTimers.
	MaxPendingTimers int

	// MaxTimersPerTick limits how many due timer callbacks fire in one Tick.
	// Excess remain queued and Tick returns more=true. Zero means
	// DefaultMaxTimersPerTick.
	MaxTimersPerTick int
}

func (o Options) withDefaults() Options {
	if o.MaxPendingTimers <= 0 {
		o.MaxPendingTimers = DefaultMaxPendingTimers
	}
	if o.MaxTimersPerTick <= 0 {
		o.MaxTimersPerTick = DefaultMaxTimersPerTick
	}
	return o
}
