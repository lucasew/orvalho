// Package js implements one goja VM isolate shell per actor with host-driven
// timers (setTimeout / setInterval / clearTimeout / clearInterval) and a
// minimal WinterTC web surface (Headers, Request, Response).
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
//     timers. Scheduling beyond the cap throws a JS TypeError.
//   - MaxTimersPerTick (default 1_000): max timer callbacks fired in a single
//     Tick. Remaining due timers stay queued; Tick returns more=true so the
//     host can yield and resume.
//
// Override via [Options].
//
// # Web types (no network)
//
// Globals Headers, Request, and Response are installed at [New]. Bodies are
// strings only; text() returns a resolved Promise. The host injects requests
// with [Isolate.MakeRequest] / [Isolate.Fetch] and reads responses with
// [Isolate.ReadResponse]. [Handler] maps net/http to default.fetch.
//
// Entry convention: global `default.fetch(request, env, ctx)`. Use
// [PrepareGuestScript] to rewrite a leading `export default` for goja.
//
// Outbound guest `fetch` is host-backed and gated by [Options.Egress]
// (empty allowlist denies all). Env asset/secret bindings are out of scope here.
package js
