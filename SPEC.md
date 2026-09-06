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
12. Host Node.js as a guest runtime or as a shebang target.
13. Lifecycle scripts (`preinstall`, `install`, `postinstall`).
14. Native Node.js addons (`.node`). Guest native code is WASM.
15. Workspaces in this version.
16. Hoisted `node_modules` as the default linker.

Inherited C (cite the file):

- Language: Go — `go.mod`
- Guest virtual machine: goja — `go.mod`, Isolate implementation
- Configuration language: CUE, embedded preludes, no `cue.mod` — Configuration load package
- Command-line parse: Cobra, single binary, `--data-dir` explicit — `cmd/orvalho`
- Package encoding: directory authoring form and ZIP deploy form, root `orvalho.cue` — Package implementation
- `attic/` is not product — `attic/README.md`
- Tarball fetch: fetchurl `Fetcher` — TEC-06

## Technique

| ID | Input | Rule | Output |
|----|-------|------|--------|
| TEC-01 | Guest script + Options | One Isolate, one mailbox goroutine, no shared memory. Host sends Fetch, Tick, Close. Idle Isolate does not burn CPU. Force-close of one Isolate does not cancel siblings | Isolated guest |
| TEC-02 | Capability request | Not injected means not allowed. Outbound Fetch, Binding, string env, and primitive host ops are injected | Guest env |
| TEC-03 | Guest throw | Wrap in a tabled sentinel so `errors.Is` works. The JavaScript string is context, not identity | Go error |
| TEC-04 | Host CUE instance, Package CUE instance | Unify with embedded prelude. Validate. On failure never allocate an Isolate. Never listen | Configuration |
| TEC-05 | Package directory | esbuild Go API downlevels and bundles to one IIFE. No child process. Product binary is self-contained | Guest script string |
| TEC-06 | `package.json` + Lockfile + registry tarball URL + integrity | Fetch through fetchurl (`FETCHURL_SERVER` when set, then the `resolved` URL). Verify the hash. Store the tarball at `{store}/{algo}/{shard}/{hash}` (shard is the first two hex characters). Durable writes use atomic stage+rename | Content store |
| TEC-15 | Lockfile + content store | Unpack each Dependency into `node_modules/.orvalho/<name>@<version>/node_modules/<name>`. Symlink each root Dependency at `node_modules/<name>`. Inside a slot, symlink that package's declared Dependencies and each peer Dependency that already exists in the graph | Isolated tree |
| TEC-16 | `package.json` + existing Lockfile bytes | Parse through the Lockfile seam. The first adapter is `package-lock.json` lockfileVersion 2 and 3. The writer emits lockfileVersion 3. `packages` keys are npm paths (`node_modules/<name>`). An unrecognized Lockfile is an error. A new project receives `package-lock.json` | Lockfile |
| TEC-17 | specifier + requiring file | Node.js CommonJS walk: parent `node_modules` climb, isolated nested slots, `exports` with conditions `node` and `require`, then `main`, then `index.js` / `.js` / `.json`. No `.node` | File path inside the tree |
| TEC-18 | package `bin` field | Symlink `node_modules/.bin/<name>` to the package file. When Script run executes a bin or a `package.json` script, prepend a directory whose `node` entry is this CLI | Process with Orvalho as `node` |
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
| TEC-06 | fetchurl `Fetcher` | adopt | a second download-and-hash client | github.com/fetchurl/fetchurl |
| TEC-06 store | content store | implement | exec pnpm. Exec Aube. Import fetchurl `internal/repository` | fetchurl/spec store layout (`/:algo/:shard/:hash`) |
| TEC-06 write | lewkit `x/io/atomic` | wrap | in-place write of finals | lewtec/lewkit |
| TEC-15 | isolated linker | implement | hoisted default. Virtual store named `.pnpm` or `.aube` | none |
| TEC-16 | `package-lock.json` | implement | default `aube-lock.yaml`. Other lockfile writers in this version | none |
| TEC-17 | Node.js CommonJS walk | implement | host Node.js as the resolver | path:pkg/imports |
| TEC-18 | scoped `PATH` `node` trampoline | implement | rewrite store shebangs. Install a global `node` | none |
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
| Persistence | files: `--data-dir` Identity; project Lockfile; content store at `--store-dir` when set, else `<project>/.orvalho/store` | C | TEC-06, TEC-16, Identity | this tree, fetchurl/spec |
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
| Isolated `node_modules` projection | isolated tree | hoisted tree (as the default), flat node_modules (as the type) |
| Hidden isolated slots | virtual store | `.pnpm`, `.aube` (as our directory name) |
| Hash-addressed tarball directory | content store | cache (as the type), CAS |
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

Proper names (not shortenings): Orvalho, WinterTC, Cloudflare, Node.js, goja, wazero, Cobra, esbuild, lewkit, fetchurl.

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
| Store | yes | identity (directory of hashed tarballs) | yes (atomic writes) | hash mismatch → error; fetch fail → error | Share it with `--data-dir`. Write finals in place |

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

`--data-dir` is required for Identity, Configuration, Device, Manager, and Actor commands. `package`, `script`, and `dependency` commands do not use `--data-dir`. Dependency store is `--store-dir` when that flag is set. Otherwise the store is `<project>/.orvalho/store`.

### Stored entities (CLI)

| Entity | Kind | Identity authority | A/B rels `(min,max)` | Root | Invariant IDs |
|--------|------|--------------------|----------------------|------|---------------|
| Identity | entity | this data-dir, Ed25519 PublicID | Operator (0,\*) — Identity (1,1) per file | yes | INV-06 |
| Lockfile | entity | project directory | Project (1,1) — Lockfile (0,1) | yes | INV-07, INV-19 |
| Dependency | weak entity | (Lockfile, name, version) | Lockfile (1,\*) — Dependency (1,1) | no | INV-07, INV-18, INV-20 |
| Store | entity | `--store-dir` when set, else `<project>/.orvalho/store` | Project (0,\*) — Store (0,1) | yes | INV-07 |

## Seams

Each row is an interface. One complete adapter at introduction. A second adapter is added when it exists. goja is not a seam.

| Interface | Injected where | First adapter | MUST NOT |
|-----------|----------------|---------------|----------|
| Binding | Options | assets over `fs.FS` | guest-registered drivers |
| outbound Fetch | Options | allowlisted `net/http` | ambient Fetch |
| Actor | host loop | Isolate mailbox | free-running guest thread |
| overlay driver | mesh compose | loopback | assume host LAN |
| atomic write | Package, Identity, Lockfile, Store | lewkit `x/io/atomic` | write finals in place |
| Lockfile | `dependency install` | `package-lock.json` v2 read, v3 read and write | a second format without a new adapter |
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
| INV-15 | Only root Dependencies appear as `node_modules/<name>`. Transitive Dependencies live under the virtual store | isolated tree | hoist every transitive name to the project top level |
| INV-16 | Script run and bin execution do not invoke host Node.js | Script, `.bin` | `#!/usr/bin/env node` reaching a host `node` |
| INV-17 | Lifecycle scripts do not run. `.node` files are not loaded | Dependency | `postinstall`, `node-gyp`, `process.dlopen` |
| INV-18 | An optional Dependency is installed only when its packument `cpu` list contains `wasm32` | Dependency | fetch `fsevents` or a platform `os`/`cpu` optional |
| INV-19 | An unrecognized Lockfile is refused. A second Lockfile is not written beside it | Lockfile | write `package-lock.json` next to `yarn.lock` |
| INV-20 | A peer Dependency is linked when a node with that name is already in the graph. A missing peer is not fetched | isolated tree | auto-install a missing peer from the registry |

## Errors

| Public operation | Bad input | One reaction |
|------------------|-----------|--------------|
| Store fetch | integrity mismatch, all sources fail | tabled error; no store object |
| Lockfile parse | unrecognized format, invalid JSON | tabled error; no tree |
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
| `dependency *` | resolve fail, unknown name, unrecognized Lockfile, integrity mismatch | exit 1, store unchanged |
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
- [ ] `dependency install` on a `package.json` with one registry Dependency writes `package-lock.json`, a content-store object, and an isolated tree whose top level contains only that name.
- [ ] `script run` of a file that `require`s that Dependency loads it through `exports` / `main` without host Node.js.
- [ ] `package serve` of a Package that has `node_modules` still evaluates one IIFE and does not walk the isolated tree.
- [ ] An optional Dependency whose packument `cpu` lacks `wasm32` is absent from the isolated tree and does not fail install.
- [ ] A `yarn.lock` in the project directory makes `dependency install` exit 1 with the store and `node_modules` unchanged.

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
11. Workspaces and `workspace:` specifiers.
12. Lockfile adapters after `package-lock.json` (pnpm, Yarn, bun, aube).
13. Registries other than `registry.npmjs.org`. `git:`, `file:`, `link:` specifiers.
14. `package.json` `#imports`.
15. Live `require` inside `package serve`.
16. `--prod` (omit `devDependencies`).
17. npm optional try-and-skip. Peer auto-fetch.
18. Hoisted linker.
19. User-level XDG content store. Sharing is `FETCHURL_SERVER`.

## Assumptions

| ID | Fact | If false |
|----|------|----------|
| AS-01 | The npm registry answers Dependency resolve | `dependency *` cannot complete; Package serve of a pre-resolved tree still can |
| AS-02 | esbuild `pkg/api` stays pure Go | TEC-05 and INV-05 need a new adopted tool |
| AS-03 | lewkit `x/io/atomic` remains the write primitive | wrap a replacement with the same stage+rename contract |
| AS-04 | fetchurl `Fetcher` honors `FETCHURL_SERVER` and verifies lowercase hex hashes | TEC-06 needs a new adopted fetch client |

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
- ADR-0009: Isolated linker under `node_modules/.orvalho/`. Rejected: npm hoisted default. Rejected: virtual store named `.pnpm` or `.aube`.
- ADR-0010: Lockfile is `package-lock.json` with a named seam. Rejected: `aube-lock.yaml` as the default. Rejected: polyglot writers in this version. Rejected: exec aube.
- ADR-0011: Orvalho is the Node.js-compatible runtime. Rejected: host `node` for `require` or `.bin`. Rejected: rewrite shebangs in the store. Script run prepends a scoped `PATH` `node` (bun `--bun`, always on).
- ADR-0012: `package serve` stays one IIFE. Embedders MAY inject `NodeModules`. Rejected: Isolate walks `node_modules` on serve.
- ADR-0013: Optional Dependencies install only when packument `cpu` contains `wasm32`. Rejected: npm try-and-skip for platform addons.
- ADR-0014: Peer Dependencies are satisfied from the existing graph. Rejected: npm 7 auto-fetch of a missing peer.
- ADR-0015: Durable writes wrap lewkit `x/io/atomic`. Language is Go 1.27. Rejected: a second stage+rename helper beside that primitive.
