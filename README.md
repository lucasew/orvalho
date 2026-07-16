# orvalho

What if we could use phones as servers?

Product vision and constraints: [`SPEC.md`](./SPEC.md).

## Layout

| Path | Role |
|------|------|
| [`cmd/orvalho`](./cmd/orvalho) | **Single** product CLI ([Cobra](https://github.com/spf13/cobra) only) |
| [`pkg/cuex`](./pkg/cuex) | Embedded CUE preludes + `LoadHost` / `LoadPackage` |
| [`pkg/ovpkg`](./pkg/ovpkg) | Zip packages with root **`orvalho.cue`** |
| [`pkg/identity`](./pkg/identity) | Manager key material |
| [`pkg/actor`](./pkg/actor) | Actor host / goja isolate |
| [`pkg/ula`](./pkg/ula) | ULA IPv6 actor address allocator |
| [`pkg/manager`](./pkg/manager) / [`pkg/worker`](./pkg/worker) | Role library skeletons |
| [`attic/`](./attic) | Prior experimental code — **not** product; do not import |

### CLI

All parsing is Cobra. **`--data-dir` is always required** for host commands (no implicit path).

```bash
go build -o bin/orvalho ./cmd/orvalho

orvalho version
orvalho --data-dir /path/to/data config validate
orvalho --data-dir /path/to/data config show
orvalho --data-dir /path/to/data identity generate
orvalho --data-dir /path/to/data identity show

# Dev: serve one package (zip or directory) on loopback — no mesh/signing
orvalho serve ./pkg/ovpkg/testdata/minimal
orvalho serve ./examples/cat-ssr
orvalho serve ./my-actor.ovpkg --listen 127.0.0.1:8787
orvalho serve ./pkg --var SITE_TITLE=Cats --env-file .dev.vars
```

Packages declare **`agents`** (exactly one for `serve`), optional **`runtime.env`** projections, and typed **`bindings`** (e.g. `ASSETS` with `type: "assets"`). See [`SPEC.md`](./SPEC.md).

### Example: cat facts SSR

[`examples/cat-ssr`](./examples/cat-ssr) is the hand-written reference package (SPEC workload, no Astro yet).

```bash
orvalho serve ./examples/cat-ssr
curl http://127.0.0.1:8787/
curl http://127.0.0.1:8787/style.css   # via env.ASSETS.fetch when the worker forwards
```

With package `egress` wired into the isolate, `orvalho serve` performs allowlisted outbound `fetch`. The cat demo loads a live fact from `catfact.ninja` when the network is up; otherwise it shows an offline fallback page.


### Configuration (CUE)

- **No** JSON/YAML config as source of truth; **no** cue.mod.
- Embedded preludes: `prelude_common.cue`, `prelude_host.cue`, `prelude_package.cue` in `pkg/cuex`.
- Host and package instances are both named **`orvalho.cue`**.
- Secret **values** are outside CUE; everything else config-shaped goes through CUE.
- Live model is `cue.Value` (optional decode after validate).

## Guest JS downlevel (goja)

Modern actor JS is downleveled with **esbuild** to **ES2015** before it runs on goja.

- Compat note: [`docs/goja-compat.md`](./docs/goja-compat.md)
- `mise run js:downlevel` / `mise run ci`

## Development

```bash
go test ./...
mise run test
go build -o bin/orvalho ./cmd/orvalho
```
