# workers

Embeddable [Cloudflare Module Worker](https://developers.cloudflare.com/workers/runtime-apis/handlers/fetch/)-shaped host on [goja](https://github.com/dop251/goja).

Run a front + back website (or any `default.fetch` guest) inside a Go process. The host injects capabilities; the guest never sees ambient filesystem, network, or process env.

Policy and product context: [SPEC.md](../../SPEC.md) (*Embeddable workers library*).

## Packages

| Path | Role |
|------|------|
| `orvalho/pkg/workers` | Core isolate: web types, timers, `Fetch` / `Tick` / `Handler` |
| `orvalho/pkg/workers/bundle` | Optional: esbuild multi-file entry + CF/node stubs |

Core does **not** depend on CUE, zip packages, or Orvalho mesh code. Orvalho `serve` is one consumer of this API.

## Quick embed

Guest (`worker.js`):

```js
export default {
  async fetch(request, env, ctx) {
    const path = new URL(request.url).pathname;
    if (path === "/hello") {
      return new Response("hi from " + (env.SITE || "worker"), { status: 200 });
    }
    // Static files via injected ASSETS binding
    return env.ASSETS.fetch(request);
  },
};
```

Host:

```go
package main

import (
	"net/http"
	"os"

	"orvalho/pkg/workers"
)

func main() {
	src, err := os.ReadFile("worker.js")
	if err != nil {
		panic(err)
	}
	script := workers.PrepareGuestScript(string(src))

	iso := workers.New(script, workers.Options{
		Env: map[string]string{
			"SITE": "demo",
		},
		Bindings: map[string]workers.Binding{
			// Any fs.FS: os.DirFS, embed.FS, package zip, …
			"ASSETS": workers.NewAssetBinding(os.DirFS("public"), "."),
		},
		// Omit Fetch ⇒ guest has no global fetch (not injected ⇒ not allowed).
		// Allow one host (or EgressList{"*"} for explicit open egress):
		Fetch: workers.HTTPFetch(workers.EgressList{"api.example.com"}, nil, 0),
	})

	// Host owns listen / TLS / middleware.
	http.ListenAndServe("127.0.0.1:8787", workers.Handler(iso))
	// or: workers.Server(iso, "127.0.0.1:8787").ListenAndServe()
}
```

## Capability model

**Not injected ⇒ not allowed.**

| Ambient (always on) | Dependency injection only |
|---------------------|---------------------------|
| `Headers`, `Request`, `Response` (minimal kernel) | Outbound `fetch` — only if `Options.Fetch` is set |
| `setTimeout` / `setInterval` / clear* | String `Options.Env` |
| | Named `Options.Bindings` (`Binding.Materialize`) |
| | Assets: `NewAssetBinding(fs.FS, root, paths...)` |

### Outbound fetch

```go
// Deny all destinations (guest still has fetch; every URL fails the allowlist).
Fetch: workers.HTTPFetch(nil, nil, 0)

// Allow listed hosts / origins / wildcards (see EgressList docs).
Fetch: workers.HTTPFetch(workers.EgressList{"api.example.com", "*.cdn.example"}, nil, 0)

// Explicit open http(s) egress (dev / trusted scripts only).
Fetch: workers.HTTPFetch(workers.EgressList{"*"}, nil, 0)
```

Pass a custom `*http.Client` as the second argument when you need timeouts, proxies, or tests.

### Bindings

- **`Binding`** — goja-native DI. Host implements `Materialize(iso *Isolate) (*goja.Object, error)`.
- **`NewAssetBinding(fsys, root, paths...)`** — CF-like `env.NAME.fetch(request|url)` over a pluggable `fs.FS`. Optional path allowlist.
- Custom host objects (app config, HAL, queues): implement `Binding` the same way as `AssetBinding` in `env.go`. Prefer that file as the template for method wiring.

String env keys must not clash with binding names.

## Lifetime and concurrency

- **One isolate, one lock.** Concurrent HTTP requests serialize through the isolate.
- **Freeze-by-default.** Guest code advances inside `Isolate.Fetch` (handler + promise/timer drain for that request). When idle, no ambient CPU.
- **`Tick(ctx)`** — public, locked; for tests or an optional background pulse.
- **`Run(ctx, every)`** — optional ticker that calls `Tick`; not started automatically.
- Cross-request `setInterval` is not a supported app pattern under freeze. Durable work belongs in **Go bindings**.
- **`ctx.waitUntil`** — present for signature compatibility; v1 is a stub (no post-response isolate work).

## Guest script

- Entry after load: `default.fetch(request, env, ctx)`.
- **`PrepareGuestScript`** rewrites a leading `export default` → `globalThis.default =` for simple single-file workers.
- Multi-file ESM (Astro / CF adapter graphs): use **`orvalho/pkg/workers/bundle`** (`BundleEntry`, needs `esbuild` on `PATH`), then pass the bundled string to `New`.

Bodies are **buffered strings/bytes** with hard size caps (no real streaming in v1).

## HTTP surface

| API | Role |
|-----|------|
| `Handler(iso)` | `http.Handler` → `default.fetch` under the lock |
| `Server(iso, addr)` | `*http.Server` with `ReadHeaderTimeout`; host still calls Serve/Shutdown |
| `iso.Fetch(ctx, HTTPRequest)` | Direct host invocation (tests, non-HTTP triggers) |

## Resource caps

Defaults (override via `Options`):

| Cap | Default |
|-----|---------|
| `MaxPendingTimers` | 10_000 concurrent scheduled timers |
| `MaxTimersPerTick` | 1_000 timer callbacks per `Tick` |
| Inbound body | `MaxRequestBody` (1 MiB) |
| Outbound fetch body | `MaxOutboundBody` (2 MiB) |
| Outbound fetch timeout | `DefaultFetchTimeout` (15s) |

## Non-goals (v1)

- Full WinterTC / workerd parity  
- Real streaming bodies or full CF `waitUntil` after the response  
- Isolate pools / multi-threaded guest  
- Package/CUE/signing/mesh inside this package  

## Related

- Product CLI and packages: [README.md](../../README.md), `orvalho serve`
- Examples: [cat-ssr](../../examples/cat-ssr), [pesquisarr](../../examples/pesquisarr)
- goja language downlevel: [docs/goja-compat.md](../../docs/goja-compat.md)
