// Package js implements one goja VM isolate shell per actor with host-driven
// timers (setTimeout / setInterval / clearTimeout / clearInterval).
//
// # Host control
//
// The host creates an [Isolate], then drives it with [Isolate.Tick]. Script
// body and timer callbacks only run inside Tick. There is no free-running
// guest event loop; the host decides when to advance time and how often to
// poll. Cancellation of the context passed to Tick interrupts the VM.
//
// # Resource caps (abuse limits)
//
// Hostile or buggy scripts must not freeze the host process. Defaults:
//
//   - MaxPendingTimers (default 10_000): hard cap on concurrent scheduled
//     timers. Scheduling beyond the cap throws a JS RangeError.
//   - MaxTimersPerTick (default 1_000): max timer callbacks fired in a single
//     Tick. Remaining due timers stay queued; Tick returns more=true so the
//     host can yield and resume.
//
// Override via [Options]. WinterTC fetch and other platform APIs are out of
// scope for this package.
package js
