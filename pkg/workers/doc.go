// Package workers implements an embeddable Cloudflare Module Worker–shaped
// host on goja: one isolate, host-driven timers, and a minimal WinterTC web
// surface (Headers, Request, Response).
//
// # Capability model
//
// What is not injected is not allowed. Outbound fetch, bindings (e.g. assets
// over fs.FS), and string env are host-provided via [Options]. The Workers
// kernel (web types + timers) is ambient so guest handlers can run.
//
// # Host control and freeze-by-default
//
// The host creates an [Isolate], then drives it with [Isolate.Tick] or
// request-scoped [Isolate.Fetch]. There is no free-running guest event loop.
// Between requests the isolate is frozen (no ambient CPU) unless the host
// opts into a pulse via Tick/Run. Cancellation of the context passed to Tick
// or Fetch interrupts the VM.
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
// # HTTP embed
//
// [Handler] maps net/http to default.fetch under the isolate lock (single-
// threaded). [Server] returns a ready *http.Server with that handler. The
// host owns listen lifecycle.
//
// Entry convention: global `default.fetch(request, env, ctx)`. Use
// [PrepareGuestScript] to rewrite a leading `export default` for goja.
// Multi-file ESM graphs use the optional [orvalho/pkg/workers/bundle] package.
//
// Outbound guest `fetch` exists only when [Options.Fetch] is set (e.g.
// [HTTPFetch] with an allowlist). Guest env is built from Options.Env and
// Options.Bindings.
package workers
