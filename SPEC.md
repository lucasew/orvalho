# Orvalho

(dew — like rain, but not from clouds)

## Vision

Give personal utility back to old phones and other low-power devices that are often thrown out because they cannot run the latest apps or a full GNU/Linux distro, yet still have radios, storage, and enough CPU to run useful services.

**Product shape:** phones (and similar devices) as **personal servers** for their owner — self-hosting, not multi-tenant cloud.

Stock ROM is the target environment: drivers already exist; OS process isolation on legacy devices is not trustworthy enough to be the security boundary.

## Goals

- Run owner-deployed **server workloads** on spare Android devices without root or a custom OS.
- Isolate workloads from each other and from ambient host power using **in-process JavaScript VMs**, not OS multi-tenancy.
- Expose platform capabilities through a **HAL-style bindings API** (web-shaped, capability-gated).
- Distribute and update workloads as **signed packages**.
- Connect manager, workers, and clients over a **userspace overlay network** so the owner does not depend on public IPs or trusting the LAN.
- Prefer [WinterTC](https://wintertc.org/) / [Cloudflare Workers](https://developers.cloudflare.com/workers/)-shaped web APIs over proprietary host surfaces. **WinterTC when it specifies; Cloudflare Workers when it does not.** Long-term bar: real Workers/Astro adapter bundles run **without rewriting guest JS** (only package metadata and host-provided `env`).

## Non-goals (current vision)

- Multi-tenant / multi-installer product (many people installing on one device).
- App store / public package discovery (useful for popularization later; not the trust model now).
- First-class **Wasm** actors or a second guest runtime (Wasm may appear later only as something JS instantiates; it is not part of packaging or the actor model now).
- Actor-to-actor communication in v1 (local or remote). Later: explicit exported APIs (e.g. contract/RPC style), not shared memory.
- Full **Node.js-compatible** server surface on device (`nodejs_compat` / `cloudflare:workers` stubs may land later only as needed for real CF bundles — not a product Node host).
- Tailscale/`tsnet` as product or admin networking in v1.
- Path-based HTTP gateway as the primary way to address actors.
- Supporting ancient Android as a development gate (backports later; do not freeze progress on min-SDK archaeology).
- SBCs, iOS, or kernel WireGuard as requirements.
- Multi-agent **supervisor** in the first serve path (a package may declare multiple agents in CUE later; early `orvalho serve` requires exactly one).

## Trust model

| Principal | Trust |
|-----------|--------|
| Device owner | Trusted with the device; not treated as an attacker against their own hardware. |
| Manager | **Sole install and deploy authority** for a mesh. |
| Bundles / actor code | **Potentially hostile or compromised** (supply chain, bugs). Host + capabilities are the boundary. |
| LAN / carrier network | **Untrusted.** Membership is cryptographic (overlay peers), not “same Wi-Fi.” |

**Isolation bar:** hostile-bundle sandbox. Code isolation is intentionally close to multi-tenant *code* isolation; what stays out of scope is multi-tenant *policy* (many installers, hard data namespaces, fair-share product). Side channels are not part of the v1 threat model.

**Install consent:** at deploy time the manager shows requested permissions (capabilities, egress allowlist, later device bindings). The owner accepts or rejects. No ambient “full internet + full HAL” for a package by default.

## Roles

### Manager

- Runs on the owner’s primary machine (Linux first).
- **Daemon** plus **CLI** for automation (same product binary family as worker; see CLI below).
- **Router-style web UI** for setup and day-to-day admin (pair devices, deploy packages, review permissions, inspect mesh/actors).
- UI/API bound to **localhost only** in v1 (no LAN/WAN admin, no tsnet).
- Holds the manager key material; signs packages; pairs workers; allocates actor addresses.

### Worker

- Long-running process on the phone (Android product host) or on Linux for development.
- Runs the actor host, userspace overlay member, package store, and local enforcement of capabilities.
- Does not accept unsigned or non-manager-signed installs in v1.

## Host platforms

| Host | Role |
|------|------|
| **Android** | Product worker. Stock ROM, unprivileged userland. Ship as APK wrapping the runtime. |
| **Linux** | Development, CI, and manager host. Same Go codebase; mock or real drivers as available. |

**Support policy:** implement against a **modern/easy Android** first. Older API levels and extreme devices (e.g. very old handsets) are **backports**, not blockers.

## Actors

- **JavaScript only** as the guest language for now.
- **One VM per actor** (goja). No shared mutable state between actors. Actors may run **concurrently** with each other (separate isolates / host scheduling). Inside one actor: single-threaded, Workers-style event loop.
- Actors are **fully ephemeral**: process or actor restart drops heap. Durable data only through **bindings** and host-injected string env the actor explicitly receives.
- Programming model: **Cloudflare Module Worker** shape — `export default { fetch(request, env, ctx) }` (and later other exported handlers only if needed). `Request` / `Response` / related web types follow WinterTC where specified.
- **Packaging ≠ Worker definition:** `orvalho.cue` is deploy/package metadata (like wrangler config). The **JS bundle** is the Worker. Guest code must not require an Orvalho-specific SDK for the happy path.
- Not a generic Node process: no ambient `fs`, raw sockets, or process environment inside the isolate. Outside-world values reach the guest only as the concrete `env` the host builds after CUE evaluation.

### Engine and build

- **Runtime VM:** [goja](https://github.com/dop251/goja) (pure Go; CGO avoided).
- **Guest build / load:** Workers-shaped modules; **esbuild (or equivalent)** downlevels to a goja-safe language level (compat matrix). Downlevel fixes language/syntax; **platform APIs are host-provided**.
- **Multi-file bundles:** prefer **bundle-to-one on load** with esbuild (same idea as wrangler bundling) rather than a full multi-module workerd loader first. Real multi-chunk Astro/`no_bundle` trees are a compatibility bar, not a first gate.
- Prefer one engine on the main line; experimental alternate VMs are not product surface.

### Load and `env` materialize

1. Load package `orvalho.cue` with host-supplied **`runtime.env`** (see Packages).
2. CUE validate / project; on failure **do not allocate** the actor (no half-live process, no address burn) — Nomad-style placement failure.
3. Select the agent instance (see Packages / serve).
4. Build guest `env`:
   - **String properties** from concrete **`agents.<name>.env`** (`map[string]string`) — Cloudflare **vars/secrets** shape (guest cannot tell them apart).
   - **Object properties** from **`agents.<name>.bindings`** via the host **driver registry**.
5. Invoke **`default.fetch(request, env, ctx)`**. Do not dump raw `runtime.env` into the isolate unless CUE projected those keys onto `agent.env`.

## Packages

Deploy unit is a **zip package** (Orvalho package):

- Archive format: zip.
- Manifest: **`orvalho.cue` only** at the archive root. Validated by unifying with the embedded **package** prelude (plus common). **JSON package manifests are forbidden** — no dual read path, no migration shim.
- Payload: JS worker build graph, static assets, and other files the manifest references.
- **Signed by the manager key**; worker verifies before install/update.

### Package CUE shape (policy)

Conceptual model (prelude fields evolve in `pkg/cuex`; this freezes intent):

```cue
runtime: {
	// Outside world → package. Host unifies a concrete map at load/serve/install.
	env: [string]: string
}

agents: [string]: {
	entrypoint: string
	// String bag for this worker only (CF vars/secrets on guest env). Map, not a list of names.
	env: [string]: string
	// Typed host drivers (assets, later devices, storage, …). Not one binding per env var.
	bindings?: [string]: {
		type: string // driver id from the host registry
		// …driver-specific fields
	}
}

// Package-level concerns as today/later: id, egress, publish/port hints, etc.
```

- **`runtime.env`:** inputs from the outside world (`map[string]string`). Serve may fill from process environment, `.env` / `.dev.vars`, and `--var` (precedence is implementation detail). Manager/worker later use other channels; **same CUE field**.
- **CUE is the DTO:** the package validates `runtime.env` with CUE constraints and **routes** values into each agent’s `env` (copy, rename, derive). No parallel hand-maintained Go schema for this projection.
- **`agents.<name>.env`:** only this map becomes string properties on the guest `env` for that worker.
- **`bindings`:** named capabilities with a **`type`** (driver id). Host registry materializes objects onto guest `env` under the same names. String configuration is **not** modeled as one binding per variable.
- **Egress allowlist**, publish/port hints, package id: still package concerns; **IPv6 assignment remains manager authority**.

### Allocation / never-start

If CUE unify/validate fails, a required outside value is missing, a binding `type` is unknown to the host, or a driver cannot materialize → **the agent is never allocated** (serve does not listen; install/placement does not create a live actor). No silent stub objects on `env` for missing drivers.

### Dev serve

- **`orvalho serve`** loads one package (dir or zip) without mesh/signing.
- **Exactly one** entry in `agents` for now; zero or more than one is an error. A multi-agent **supervisor** is backlog.
- Same materialize rules as above so serve exercises the production env contract.

## Bindings (host drivers)

**Registry pattern:** the **host worker/serve process** registers drivers by id. Packages **request** bindings by name + `type`; guests **never** register drivers. v1 and later differ in *which* drivers exist, not in the ask/provide shape.

| Driver / surface | v1 | Later |
|------------------|----|--------|
| **String env** (`agent.env`) | Yes — CF-style strings on guest `env` (vars; secrets same at runtime, different provisioning) | Manager-sealed secret store |
| **`assets`** | Yes — CF-like `env.<NAME>.fetch(request\|url\|string) → Response` over package files | Richer static routing if needed |
| **Storage** (KV/SQL/etc.) | Not required for first demo | When actors need durability |
| **Devices (HAL)** | Same registry shape reserved | Camera, GPU, sensors, … |
| **Actor export / RPC** | No | Explicit exported APIs between actors |

Outbound **`fetch`**: only destinations allowed by the package allowlist after owner consent. A personal mesh must not become an open residential proxy by default.

## Networking

### Overlay

- **Userspace WireGuard** ([wireguard-go](https://github.com/WireGuard/wireguard-go)).
- Peer identity tied to Orvalho identity keys (manager/device); allowed peers form the mesh.
- If a peer is admitted and can reach a published endpoint, it may use that service — **no trust in LAN or public IP**. NAT traversal / relay as needed so “phone on LTE ↔ laptop” works without lucky port forwards.
- Owner should care that the device is somehow on the internet, not about managing underlay IPs.

### Addressing

- **IPv6 per actor** (ULA plan: mesh/owner prefix, device, then `/128` per actor).
- Manager allocates and records addresses at install/update.
- Clients address actors by **IPv6 (+ port)** — not path-based routing on a shared gateway as the primary model.
- MagicDNS-style names are optional sugar later.
- Host may short-circuit same-device delivery later; **v1 forbids actor↔actor use** regardless.

### Manager admin network

- Localhost only for the router-like UI and local API.

## Reference workload

First end-to-end proof:

1. Manager identity + worker pair.
2. Owner accepts permissions for a package (including egress allowlist).
3. Deploy an **Astro SSR** app built for a **Workers / WinterTC adapter path** (not static export as the proving target; not Node adapter).
4. Example: **SSR page that calls an allowlisted cat API** and returns HTML.
5. Client on the mesh opens `http://[actor-ipv6]:port/` (or equivalent) and gets a response.

This proves: identity, pair, sign/install, sandbox, CUE `runtime.env` → `agent.env`, assets (and other) drivers, egress policy, userspace overlay, IPv6-per-actor publish — ideally against an **unchanged Workers/Astro adapter bundle** plus Orvalho package metadata.

## CLI and configuration

### CLI (Cobra)

- [Cobra](https://github.com/spf13/cobra) for **all** command-line surfaces — **no custom argv / `flag` parsers**.
- Single product binary `orvalho` with role/subcommand trees (`identity`, `manager`, `worker`, `config`, …). Optional build tags may slim Android later without abandoning Cobra.
- Root **`orvalho version`** only — not per-role `manager version` / `worker version`.
- **Full Cobra** means every CLI surface goes through Cobra; it does **not** mean every mesh product feature is implemented.

### Configuration (CUE)

- **CUE is the only config language** for product configuration.
- **Outside values** for packages enter as **`runtime.env`: `map[string]string`** (host overlay). Secret *values* are not authored into the signed zip; they are supplied at serve/install and validated/projected by package CUE. Guest-visible strings are only what CUE emits on **`agents.<name>.env`**.
- **No parallel config system:** no JSON/YAML host or package config as source of truth; no “env as schema” outside CUE. Cobra flags are **paths or overlays into CUE**, not a second configuration model.
- **No hand-maintained config DTOs as schema.** The live model is **`cue.Value`**. Optional fill of a Go struct is allowed **only after** CUE validation — the struct is an output of validation, not a second schema you maintain beside CUE.
- Host and package instances are both named **`orvalho.cue`**. Which prelude applies depends on the load path (host vs package), not the filename.
- **Data dir** is **always** an explicit CLI argument (`--data-dir`). No implicit XDG/home discovery as the product model. Optional host config path via `--config` when needed (default: `<data-dir>/orvalho.cue`).

### CUE load style (contapila / workspaced)

- **Embed** preludes in the binary (`//go:embed`). **Ignore the CUE module system** — no `cue.mod`, no runtime `cue` CLI required for normal use.
- Three prelude layers (workspaced home/codebase analogy):
  - `prelude_common.cue` — shared constraints
  - `prelude_host.cue` — manager/worker host instance
  - `prelude_package.cue` — zip package instance
- Load recipe: `Compile` preludes → `Unify` instance (and optional flag/generated overlays) → `Validate` (including concrete checks for required fields). Empty host `orvalho.cue` may be valid when defaults suffice; missing package `orvalho.cue` is invalid.

### Repository layout (config / CLI)

| Path | Role |
|------|------|
| `cmd/orvalho` | Sole product CLI entrypoint (Cobra) |
| `pkg/cuex` | Embedded preludes; `LoadHost` / `LoadPackage`; `cue.Value` |
| `pkg/ovpkg` | Zip read/write; root `orvalho.cue`; validates via cuex |
| `pkg/identity` | Manager key material (values on disk, not in CUE) |
| `attic/` | Non-product; cherry-pick only — do not import from live code |

There is **no** product `pkg/manifest` JSON schema package. Package domain helpers live on CUE values via cuex/ovpkg.

## Implementation notes (base)

- **Language:** Go, pure Go preferred (**no CGO**).
- **JS host:** goja + host polyfills for WinterTC subset + CF-shaped `env` (strings + driver objects).
- **Binding drivers:** in-process host registry; built-ins first (`assets`, …); no guest- or zip-supplied drivers.
- **Identity:** manager/device key material as product code requires; mesh keys derived or minted from the same trust root. Attic code is cherry-pick only.
- **Codebase attitude:** selective reuse of current tree and branches only when it clearly fits; **do not overfit architecture to existing checkpoint code**. One runtime story (goja), not dual goja/QuickJS product paths.
- **Android lifecycle:** best-effort background stickiness; correctness must not require promising 24/7 uptime on stock Android in v1 docs. Mesh and actors reconnect when the worker runs.
- **Scale:** small — a handful of personal devices per owner; not a cluster product.

## Milestone sketch

1. **Runtime contract** — goja isolate, WinterTC/CF Module Worker `fetch`, timers, package CUE (`runtime.env` / `agents` / `agent.env` / bindings registry), assets driver, esbuild downlevel (+ on-load bundle for multi-file as needed), `orvalho serve` (exactly one agent).
2. **Package + manager** — zip + CUE manifest, sign/verify, localhost daemon/CLI (Cobra)/UI, permission consent, install-time value injection into `runtime.env`.
3. **Mesh + publish** — wireguard-go, pair, IPv6-per-actor, HTTP serve on actor address.
4. **Reference** — Astro SSR / real CF adapter bundle (e.g. cat API) on a worker (Linux, then Android) with **unchanged guest JS** where feasible.
5. **Later** — multi-agent supervisor, `nodejs_compat` / virtual module stubs as required, device bindings (HAL), durable storage drivers, actor-exported APIs, older Android backports, optional discovery UX / app store experiments.

## Open details (deliberately not frozen here)

ULA prefix math, HTTP vs HTTPS on the mesh, per-actor resource limit numbers, relay deployment topology, Android packaging specifics, exact serve value-source precedence, and further CUE field growth as features land — decide in implementation. Prelude field sets evolve in `pkg/cuex` with the code; this document freezes **policy**, not every CUE key.
