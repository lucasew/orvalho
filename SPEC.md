# Orvalho Specification

This document constrains the WinterTC Isolate library, the bun-like toolchain command-line interface, and how an operator composes them. The phone-grid product is a consumer of those libraries.

Status: approved
Genre: library + cli

The key words MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY in this
document are to be interpreted as described in BCP 14 (RFC 2119,
RFC 8174) when, and only when, they appear in all capitals.

## Intention

Job: Ship a WinterTC Isolate and a bun-like command-line interface as importable libraries. The phone-grid product is the first consumer, not the only one.

Non-goals:

1. Multi-tenant product. Multi-installer product.
2. First-class WebAssembly actors. Guest JavaScript MAY instantiate WebAssembly later.
3. `tsnet` as the product network.
4. Kernel WireGuard.
5. Path-based HTTP as the primary way to address an Actor.
6. App store. Public package discovery.
7. Full Node.js host.
8. Ancient Android as a development gate.
9. Replacing goja.
10. CGO.
11. A god Runtime type that owns Isolate, Package, overlay, and Identity.

Inherited C (cite the file):

- Language: Go 1.25 — `go.mod`
- Guest virtual machine: goja — `go.mod`, Isolate implementation
- Configuration language: CUE, embedded preludes, no `cue.mod` — Configuration load package
- Command-line parse: Cobra, single binary, `--data-dir` explicit — `cmd/orvalho`
- Package encoding: directory authoring form and ZIP deploy form, root `orvalho.cue` — Package implementation
- `attic/` is not product — `attic/README.md`

## Technique

| ID | Input | Rule | Output |
|----|-------|------|--------|
| TEC-01 | Guest script + Options | One Isolate, one mailbox goroutine, no shared memory. Host sends Fetch, Tick, Close. Idle Isolate does not burn CPU. Force-close of one Isolate does not cancel siblings | Isolated guest |
| TEC-02 | Capability request | Not injected means not allowed. Outbound Fetch, Binding, string env, and primitive host ops are injected | Guest env |
| TEC-03 | Guest throw | Wrap in a tabled sentinel so `errors.Is` works. The JavaScript string is context, not identity | Go error |
| TEC-04 | Host CUE instance, Package CUE instance | Unify with embedded prelude. Validate. On failure never allocate an Isolate. Never listen | Configuration |
| TEC-05 | Package directory | esbuild Go API downlevels and bundles to one IIFE. No child process. Product binary is self-contained | Guest script string |
| TEC-06 | `package.json` + lockfile | Resolve a tree into a content store. Durable writes use atomic stage+rename | Store + lockfile |
| TEC-07 | Package tree | Write a ZIP with root `orvalho.cue`. Do not enroll a Device | Package ZIP |
| TEC-08 | WinterTC entrypoint | Host maps HTTP to `default.fetch(request, env, ctx)` | HTTP response |
| TEC-09 | Script entrypoint | Host evaluates the Script target (file path, `package.json` script name) as main. `default.fetch` is not required | Process exit |
| TEC-10 | Primitive host ops | Go exposes a small Binding table. A JavaScript dispatcher implements WinterTC and claimed Node.js modules | Guest API surface |
| TEC-11 | Overlay Dial/Listen | Userspace driver. Addresses are not host LAN addresses. This SPEC’s driver is loopback | Overlay connection |
| TEC-12 | Public identifier | Full word from the glossary. Short form only if that table lists it | Name |
| TEC-13 | New package interface | One primary type. Operations are constructors and methods on that type. Named interface at each listed seam. One complete adapter at introduction | Deep module |
| TEC-14 | Exported change | Implements a named technique, uses the adopted tool, and has a test that fails if the thought is missing | Reviewable change |

## Tooling

| TEC | Tool | Relation | We do not | Cite |
|-----|------|----------|-----------|------|
| TEC-01 | goja | wrap | a second guest virtual machine | `go.mod` |
| TEC-02 | Isolate Options | implement | ambient guest network | path:pkg/workers |
| TEC-03 | `errors` sentinels | implement | string-match guest text | path:pkg/workers/errors.go, stdlib:errors |
| TEC-04 | CUE | adopt | a second schema, JSON/YAML config | path:pkg/cuex, org:CUE |
| TEC-05 | esbuild `pkg/api` | wrap | `esbuild` on `PATH`, `os/exec` of esbuild | github.com/evanw/esbuild/pkg/api |
| TEC-06 | content store | implement | exec pnpm. Exec Aube | none |
| TEC-06 write | lewkit `x/io/atomic` | wrap | in-place write of finals | lewtec/lewkit |
| TEC-07 | `archive/zip` | wrap | a second archive format | path:pkg/ovpkg, stdlib:archive/zip |
| TEC-08 | `net/http` | adopt | a second HTTP stack | path:pkg/workers, stdlib:net/http |
| TEC-09 | Isolate | implement | require `default.fetch` on Script | path:pkg/workers |
| TEC-10 | JS dispatcher + Go primitives | implement | WinterTC as a Go function bag | lewtec/tailgopher (shape) |
| TEC-11 | loopback | implement | host LAN membership | stdlib:net |
| TEC-12 | this glossary | implement | unlisted shortenings | this document |
| TEC-13 | listed interfaces | implement | engine interface over goja | this document |
| TEC-14 | first-party tests, WPT, Node.js module tests | implement | “it runs” as the bar | this document |
| CLI parse | Cobra | adopt | a second argv parser | path:cmd/orvalho, `go.mod` |
| Identity key | Ed25519 PEM | adopt | attic bip39/age as product | path:pkg/identity, stdlib:crypto/ed25519 |
| ULA plan | Allocation math | implement | kernel addressing | path:pkg/ula, stdlib:net/netip |
| WASM engine | wazero | wrap, later work | first-class WASM actors | lewtec/tailgopher |

| Cell | Pick | C or D | Implements | Cite if C |
|------|------|--------|------------|-----------|
| Language | Go | C | TEC-01–TEC-14 | `go.mod` |
| Runtime | goja | C | TEC-01 | `go.mod` |
| Persistence | files: `--data-dir` Identity, project lockfile + store | D | TEC-06, Identity | |
| UI | none | D | | |
| Packaging | ZIP Package, self-contained binary | C | TEC-05, TEC-07 | path:pkg/ovpkg |
| Identity | Ed25519 PEM | C | Identity generate | path:pkg/identity |
| Host OS | Linux | C | CLI | this tree |

## Terminology

| Concept | Approved | Banned |
|---------|----------|--------|
| Guest virtual machine instance | Isolate | VM, runtime (as the instance), worker (as the instance) |
| WinterTC program start | `default.fetch` | handler, listener (as the entrypoint) |
| Script program start | Script | bun main, node main |
| Deploy unit | Package | ovpkg (as a type), bundle (as the unit), app |
| Registry module in the store | Dependency | pkg, dep, npm package (as our type) |
| Host CUE instance | Configuration | config (as a type), settings |
| Manager keypair | Identity | manager (as the key type), keypair (as the type name) |
| Recorded ULA /128 | Allocation | address (alone), IP (as the type) |
| Host-driven guest | Actor | process, thread (as the guest) |
| Installed grid unit | Actor (mesh, later) | worker (as the CLI noun) |
| Long-running phone/Linux process | Device | worker (as the CLI noun) |
| Person who runs `orvalho` | Operator | user, developer (in law) |
| Go program that imports Isolate | Embedder | consumer, client (in law) |
| Named injected capability | Binding | driver (as the guest-facing type) |
| Userspace network implementation | overlay driver | VPN (as the type), mesh (as the Dial type) |
| Well-thought landing | TEC-14 | rushed code, it works |

### Approved short forms

Only these short forms MAY appear as tokens in law and in new exported names. Any other shortening MUST be written in full.

| Form | Means |
|------|--------|
| HTTP | Hypertext Transfer Protocol |
| HTTPS | HTTP over TLS |
| TLS | Transport Layer Security |
| TCP | Transmission Control Protocol |
| IP | Internet Protocol |
| IPv6 | Internet Protocol version 6 |
| DNS | Domain Name System |
| URL | Uniform Resource Locator |
| VPN | virtual private network |
| CUE | CUE configuration language |
| ULA | Unique Local Address (RFC 4193) |
| JS | JavaScript (prose only) |
| WASM | WebAssembly |
| WPT | Web Platform Tests |
| CLI | command-line interface |
| API | application programming interface |
| ZIP | zip archive format |
| PEM | Privacy-Enhanced Mail key encoding |
| Ed25519 | Edwards-curve digital signature algorithm |
| CGO | Go cgo |
| ESM | ECMAScript module |
| IIFE | immediately-invoked function expression |
| npm | npm package registry |
| RFC | Request for Comments |
| BCP | Best Current Practice |
| ID | identifier in prose. Exported name is `Identity`. Public identifier field is `PublicID` |
| FS | Go `fs.FS` only |
| JSON | JavaScript Object Notation |

Proper names (not shortenings): Orvalho, WinterTC, Cloudflare, Node.js, goja, wazero, Cobra, esbuild, lewkit.

Banned in new exported identifiers and new import paths: `cfg`, `pkg`, `util`, `helpers`, `misc`, `mgr`, `iso`, `dep`, `impl`, `svc`, `dev`, `vm`, `opts` as a type, `ovpkg` as a type, `cuex` as a type, `HAL`, `RPC`, `CAS`, `XDG`, `SSR`, `CF`, `KV`, `SQL`, `APK`, `SDK`, `LAN`, `WAN`, `UI`, `NAT`, `DI`.

Legacy import paths (MAY remain on disk; new types use the approved word): Configuration load is `pkg/cuex`, Package is `pkg/ovpkg`, Allocation is `pkg/ula`, Isolate host is `pkg/workers`.

## Types

### Library

| Type | Exported | Identity or value | Mutable | Nil/error | Callers MUST NOT |
|------|----------|-------------------|---------|-----------|------------------|
| Isolate | yes | identity (one virtual machine) | yes (heap, timers) | `New` returns non-nil; methods return error | Share Isolate memory; touch the goja value; drive it from two goroutines except via its mailbox |
| Options | yes | value | no after `New` | zero value is valid (defaults) | Mutate after `New`; inject a capability they do not intend to grant |
| Binding | yes | identity (host object) | host-owned | unknown type → never-allocate | Register a Binding from guest code |
| Package | yes | identity (directory authoring form, ZIP deploy form) | no (projection returns a new value) | missing `orvalho.cue` → error | Treat it as a Dependency. Serve after CUE failure. Serve after Binding failure |
| Configuration | yes | identity (`cue.Value`) | no after validate | unify fail → error | Maintain a parallel Go schema |
| Identity | yes | identity (Ed25519) | no (new file is a new Identity) | bad PEM → error | Log the private key. Export the private key |
| Allocation | yes | identity (recorded ULA) | store records it | bad plan → error | Invent a /128 outside the allocator |
| Actor | yes (interface) | identity | host-driven | `Tick` error on cancel. `Tick` error on fail | Start a free-running guest loop |
| Dependency | yes | identity (name + version in the store) | store mutates | resolve fail → error | Treat it as a Package |

### Command-line interface

Grammar: `orvalho <noun> <action>`. Depth is two. `orvalho version` is the only root exception.

| Command | Type it mutates | Transition | Bad input |
|---------|-----------------|------------|-----------|
| `version` | none | print | extra args → exit 1 |
| `package serve` | Isolate (process-local) | load Package → listen → `default.fetch` | bad Package, not exactly one agent → exit 1, no listen |
| `script run` | Isolate (process-local) | evaluate main | missing file, guest throw → exit 1 |
| `dependency install` | Dependency store + lockfile | resolve declared tree | resolve fail → exit 1, no half store |
| `dependency add` | Dependency store + lockfile | add name, resolve | resolve fail → exit 1 |
| `dependency remove` | Dependency store + lockfile | drop name, resolve | unknown name → exit 1 |
| `package build` | build artifact | TEC-05 | bundle fail → exit 1, no partial artifact |
| `package create` | Package ZIP | TEC-07 | validate fail → exit 1, no ZIP |
| `identity generate` | Identity file | write `manager.key` under `--data-dir` | exists without force → exit 1 |
| `identity show` | none | print PublicID | missing key → exit 1 |
| `configuration validate` | none | unify host CUE | invalid CUE → exit 1 |
| `configuration show` | none | print concrete CUE | invalid CUE → exit 1 |
| `device start` | `--data-dir` | later | fail closed, tabled error |
| `device pair` | `--data-dir` | later | fail closed, tabled error |
| `manager start` | `--data-dir` | later | fail closed, tabled error |
| `actor install` | `--data-dir` | later | fail closed, tabled error |
| `actor list` | `--data-dir` | later | fail closed, tabled error |

`--data-dir` is required for Identity, Configuration, Device, Manager, and Actor commands. `package`, `script`, and `dependency` commands do not use `--data-dir`. Dependency store is `--store-dir` when that flag is set. Otherwise the store is project-local.

### Stored entities (CLI)

| Entity | Kind | Identity authority | A/B rels `(min,max)` | Root | Invariant IDs |
|--------|------|--------------------|----------------------|------|---------------|
| Identity | entity | this data-dir, Ed25519 PublicID | Operator (0,\*) — Identity (1,1) per file | yes | INV-06 |
| Lockfile | entity | project directory | Project (1,1) — Lockfile (0,1) | yes | INV-07 |
| Dependency | weak entity | (Lockfile, name, version) | Lockfile (1,\*) — Dependency (1,1) | no | INV-07 |

## Seams

Each row is an interface. One complete adapter at introduction. A second adapter is added when it exists. goja is not a seam.

| Interface | Injected where | First adapter | MUST NOT |
|-----------|----------------|---------------|----------|
| Binding | Options | assets over `fs.FS` | guest-registered drivers |
| outbound Fetch | Options | allowlisted `net/http` | ambient Fetch |
| Actor | host loop | Isolate mailbox | free-running guest thread |
| overlay driver | mesh compose | loopback | assume host LAN |
| atomic write | Package, Identity, lockfile | lewkit `x/io/atomic` | write finals in place |
| primitive host ops | Isolate | Go funcs the JavaScript dispatcher calls | implement WinterTC as a Go function bag |

Not interfaces: goja, esbuild, CUE, Cobra.

## Invariants

| ID | Predicate | On | Forbidden bypass |
|----|-----------|----|------------------|
| INV-01 | Isolates share no memory. They exchange messages only through the host | Isolate | shared map, shared goja runtime |
| INV-02 | A capability the host did not inject is absent | Options | global Fetch, ambient FS |
| INV-03 | CUE failure never allocates an Isolate and never listens | Package, Configuration | listen then error |
| INV-04 | Guest throw unwraps to a tabled sentinel | Isolate | `err.Error()` string match as identity |
| INV-05 | Product binary does not exec `esbuild` | Package build/serve, Script run | `PATH` esbuild |
| INV-06 | Identity private key is not printed | Identity | `identity show` of PEM private |
| INV-07 | Dependency store writes are atomic | Dependency | truncate lockfile in place |
| INV-08 | Force-close of one Isolate does not lock another Isolate. Force-close does not cancel another Isolate | Isolate | process-wide interrupt |
| INV-09 | New exported identifier is a glossary word, including an approved short form | all types | `cfg`, `util`, `pkg` as a type |
| INV-10 | Package interface is one primary type. No exported `util` surface | all packages | `helpers.go` public API |
| INV-11 | Exported operation has a test that can fail TEC-14 | public ops | demo-only path |
| INV-12 | CGO is absent | module | cgo import |
| INV-13 | `serve` requires exactly one agent in the Package | `package serve` | silent first-agent pick |
| INV-14 | Overlay addresses are not host LAN addresses | overlay | bind product traffic on the host interface as the model |

## Errors

| Public operation | Bad input | One reaction |
|------------------|-----------|--------------|
| Isolate.New / Fetch / Tick | missing `default.fetch` on WinterTC path, thrown guest, cancelled context | return tabled error; no panic |
| Isolate outbound Fetch | host missing, allowlist miss | tabled deny; no request |
| Binding.Materialize | unknown type, missing FS | never-allocate; tabled error |
| Package open | missing `orvalho.cue`, unsafe path | tabled error; no Isolate |
| Package.WithRuntimeEnv | CUE unify fail | tabled error; no half Package |
| Configuration load | invalid CUE | tabled error |
| Identity.Generate/Save | exists, empty path, bad PEM | tabled error; no overwrite unless force |
| Allocation | bad prefix | tabled error; no address |
| `orvalho` any command | any error | exit 1, message on stderr |
| `orvalho version` | extra args | exit 1 |
| `package serve` | bad Package, agent count ≠ 1 | exit 1, no listen |
| `script run` | missing file, guest throw | exit 1 |
| `dependency *` | resolve fail, unknown name | exit 1, store unchanged |
| `package build` | bundle fail | exit 1, no partial artifact |
| `package create` | validate fail | exit 1, no ZIP |
| `device` / `manager` / `actor` | invoked before later work | exit 1, tabled skeleton error |

## Actors

Library surface: no people. CLI surface uses the rows below.

| Actor | Obligations |
|-------|-------------|
| Operator | Runs `orvalho`. Passes `--data-dir` for host commands. |
| Embedder | Constructs Isolate, injects Options, owns HTTP listen when not using `package serve`. |

## Capabilities

Library surface: Embedder only. CLI surface: Operator rows.

| ID | Actor | Sea-level goal |
|----|-------|----------------|
| CAP-01 | Operator | Serve a Package through `default.fetch` |
| CAP-02 | Operator | Run a Script as main |
| CAP-03 | Operator | Mutate the Dependency store |
| CAP-04 | Operator | Create a Package ZIP |
| CAP-05 | Operator | Generate Identity |
| CAP-06 | Operator | Show Identity |
| CAP-07 | Operator | Validate Configuration |
| CAP-08 | Operator | Show Configuration |
| CAP-09 | Embedder | Host an Isolate with injected capabilities |

## Quality

| Concern | Measure, or why it cannot happen |
|---------|----------------------------------|
| Compatibility | Pinned WPT directories for the WinterTC surface. For each exported Node.js module, the hosted Node.js cases we claim, including workerd-catalog cases. A regression is a newly failing listed test. Test262 is not a gate. |
| Error model | Every public failure is a tabled sentinel. Guest throws unwrap so `errors.Is` works. |
| Exit contract | Exit 0 on success. Exit 1 on any command error. Errors on stderr. Success data on stdout. Long-running commands die on signal. |
| Untrusted input | Hostile Package, guest JavaScript, Dependency name, CUE. Never-allocate on validate fail. Never-allocate on Binding fail. INV-02, INV-03, INV-07, INV-08. |

## Security

In scope: hostile guest JavaScript, untrusted Package, untrusted Dependency name, Identity private key at rest, Isolate isolation.

Why multi-tenant policy cannot happen: non-goal 1. Code isolation is the bar. Many installers, hard data namespaces, and fair-share product are out of scope.

Residual risk: goja defects, side channels, Operator compromise of `--data-dir`. Side channels are not in the v1 threat model.

## Success

- [ ] `orvalho package serve` on the minimal Package listens and answers HTTP through `default.fetch`.
- [ ] `orvalho script run` on a file without `default.fetch` exits 0 when the Script succeeds.
- [ ] Missing `orvalho.cue` makes `package serve` exit 1 with no listen.
- [ ] A guest throw is `errors.Is` a tabled sentinel.
- [ ] Force-close of one Isolate leaves another Isolate able to Fetch.
- [ ] `package build` and `package serve` succeed with no `esbuild` on `PATH`.
- [ ] `identity generate` refuses to overwrite without force.
- [ ] `go.mod` has no CGO requirement.
- [ ] A listed WPT directory is executed in continuous integration. Failures are counted, not ignored.

## Later work

1. wazero integration. Guest JavaScript instantiates WASM. Engine is already chosen.
2. Common JavaScript-environment spine extract (tailgopher and WinterTC).
3. Overlay drivers (`tsnet`, wireguard-go) and userspace ingress.
4. CLI: `device start`, `device pair`, `manager start`, `actor install`, `actor list`.
5. Multi-agent supervisor.
6. Manager localhost UI.
7. Android packaging.
8. Rename legacy import paths to glossary words.
9. Switch the WPT pin to the official WinterTC cut when that cut exists.
10. Durable storage Bindings, device Bindings, Actor-to-Actor messages through the host.

## Assumptions

| ID | Fact | If false |
|----|------|----------|
| AS-01 | The npm registry answers Dependency resolve | `dependency *` cannot complete; Package serve of a pre-resolved tree still can |
| AS-02 | esbuild `pkg/api` stays pure Go | TEC-05 and INV-05 need a new adopted tool |
| AS-03 | lewkit `x/io/atomic` remains the write primitive | wrap a replacement with the same stage+rename contract |

## Decision history

- ADR-0000: Two guest entrypoints, `package serve` and `script run`. Rejected: one verb for both.
- ADR-0001: Noun then action, depth two. Rejected: facet prefixes (`wintertc`, `toolchain`, `mesh`) and bun-flat root verbs.
- ADR-0002: Device is the CLI noun for the long-running host. Rejected: CLI noun `worker`.
- ADR-0003: esbuild as a Go library. Rejected: `esbuild` on `PATH`.
- ADR-0004: Overlay driver is loopback in this SPEC. Rejected: wireguard-go as the first driver. Rejected: `tsnet` as the first driver.
- ADR-0005: wazero is the WASM engine. Integration is later. Rejected: first-class WASM actors.
- ADR-0006: goja is not an interface. Rejected: swappable guest engines.
- ADR-0007: WPT directories plus per-module Node.js tests are the compatibility bar. Rejected: full Node.js test tree as a gate. Rejected: first-party tests only.
- ADR-0008: Dependency store is not `--data-dir`. Rejected: one directory for mesh state and the content store.
