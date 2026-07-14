# orvalho

What if we could use phones as servers?

Product vision and constraints: [`SPEC.md`](./SPEC.md).

## Layout

| Path | Role |
|------|------|
| [`cmd/orvalho`](./cmd/orvalho) | **Single** product CLI entrypoint ([Cobra](https://github.com/spf13/cobra)) |
| [`pkg/manager`](./pkg/manager) | Manager library skeleton |
| [`pkg/worker`](./pkg/worker) | Worker library skeleton |
| [`pkg/identity`](./pkg/identity) | Manager key material |
| [`pkg/actor`](./pkg/actor) | Actor host / goja isolate |
| [`pkg/manifest`](./pkg/manifest) | Package manifest parse/validate (migrating to CUE — see SPEC) |
| [`pkg/ovpkg`](./pkg/ovpkg) | Zip package library |
| [`pkg/ula`](./pkg/ula) | ULA IPv6 actor address allocator |
| [`attic/`](./attic) | Prior experimental code — **not** product; do not import |

### CLI

One binary, role subcommands:

```bash
go build -o bin/orvalho ./cmd/orvalho

orvalho --help
orvalho identity generate|show
orvalho manager version   # skeleton
orvalho worker version    # skeleton
```

Config is **CUE** for host and package manifests (SPEC). Do not add parallel non-Cobra entrypoints.

Manager runs on the owner's primary machine (Linux first). Worker runs on spare phones (Android) or Linux for development.

## Guest JS downlevel (goja)

Modern actor JS is downleveled with **esbuild** to **ES2015** before it runs on goja.

- Compat note / target rationale: [`docs/goja-compat.md`](./docs/goja-compat.md)
- Fixture pipeline: `tools/js-downlevel/`
- Build: `mise run js:downlevel`
- CI golden check: `mise run js:downlevel:check` (also part of `mise run ci`)

## Development

```bash
go test ./...
# or
mise run test

go build -o bin/orvalho ./cmd/orvalho
```
