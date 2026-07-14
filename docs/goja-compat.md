# goja guest JS compatibility (stub)

Orvalho runs actor code in [goja](https://github.com/dop251/goja) (pure Go). Guest packages
may be authored in modern JS (and later Astro / Workers-shaped toolchains), but the **deployed
bundle** must parse and run on goja.

## Downlevel target

| Setting | Value | Notes |
|---------|--------|--------|
| Tool | **esbuild** (pinned in `mise.toml` as `http:esbuild`) | Syntax transform + bundle only |
| `--target` | **`es2015`** | ES6 language level |
| `--format` | **`iife`** | Single script the host can `vm.RunString`; no native ESM loader in v1 |
| `--platform` | **`neutral`** | No Node/browser built-ins assumed |

### Why ES2015

- goja implements **ECMAScript 5.1** fully and **most of ES6**, with remaining gaps tracked upstream.
- Syntax **newer than ES2015** (optional chaining `?.`, nullish coalescing `??`, class fields, top-level await, etc.) is **not** a reliable goja surface. esbuild rewrites those forms when `--target=es2015`.
- ES2015 is a better product target than ES5: smaller output, keeps `const`/`let`/arrows/classes that goja already handles for typical worker graphs.
- **Downlevel is about language syntax only.** WinterTC / Workers APIs (`fetch`, `Request`, `Response`, `env` bindings, timers) are **host-provided**, not polyfilled by this pipeline.

## Fixture pipeline

```text
tools/js-downlevel/fixtures/modern.js   # modern source (uses ?. and ??)
        │
        ▼  mise run js:downlevel
tools/js-downlevel/out/bundle.js        # generated ES2015 IIFE
        │
        ▼  compared in CI
tools/js-downlevel/golden/bundle.js     # committed golden
```

| Task | Purpose |
|------|---------|
| `mise run js:downlevel` | Build `out/bundle.js` |
| `mise run js:downlevel:check` | Build and `diff` against golden |
| `mise run js:downlevel:golden` | Refresh golden after intentional fixture/tool changes |
| `mise run ci` | Golden check + `go test ./...` |

Go tests under `tools/js-downlevel` assert the fixture/golden exist and that the golden no longer contains the modern operators present in the fixture (smoke / invariant checks without requiring esbuild inside `go test`).

## Not covered here

- Full feature matrix of every ES2015+ construct vs goja (evolve this doc as isolates grow).
- Runtime polyfills for platform APIs.
- Astro / Workers adapter packaging (later milestones).
