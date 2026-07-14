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
- Prefer [WinterTC](https://wintertc.org/) / Workers-shaped web APIs over proprietary host surfaces.

## Non-goals (current vision)

- Multi-tenant / multi-installer product (many people installing on one device).
- App store / public package discovery (useful for popularization later; not the trust model now).
- First-class **Wasm** actors or a second guest runtime (Wasm may appear later only as something JS instantiates; it is not part of packaging or the actor model now).
- Actor-to-actor communication in v1 (local or remote). Later: explicit exported APIs (e.g. contract/RPC style), not shared memory.
- Node.js-compatible server surface on device.
- Tailscale/`tsnet` as product or admin networking in v1.
- Path-based HTTP gateway as the primary way to address actors.
- Supporting ancient Android as a development gate (backports later; do not freeze progress on min-SDK archaeology).
- SBCs, iOS, or kernel WireGuard as requirements.

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
- **Daemon** plus **CLI** for automation.
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
- Actors are **fully ephemeral**: process or actor restart drops heap. Durable data only through **bindings** the actor explicitly uses.
- Programming model: **WinterTC / Cloudflare Workers-esque** edge runtime — especially `export default { fetch(request, env, ctx) }` (or the compiled equivalent the host invokes), `Request` / `Response` / related web types, and capability-gated `env`.
- Not a generic Node process: no ambient `fs`, raw sockets, or process environment.

### Engine and build

- **Runtime VM:** [goja](https://github.com/dop251/goja) (pure Go; CGO avoided).
- **Guest build:** toolchain emits Workers-shaped output, then **esbuild (or equivalent) downlevels** to a JS level documented in a goja compat matrix. Downlevel fixes language/syntax; **platform APIs are host-provided**, not assumed from the VM.
- Prefer one engine on the main line; experimental alternate VMs are not product surface.

## Packages

Deploy unit is a **zip package** (Orvalho package):

- Archive format: zip.
- Manifest: **`orvalho.json`** at a fixed path in the archive.
- Payload: JS worker build graph, static assets, and other files the manifest references.
- **Signed by the manager key**; worker verifies before install/update.

Manifest (conceptual — exact schema evolves in code) declares at least:

- Actor identity / name
- Entry / runtime kind (`js`)
- Requested **bindings** and **permissions**
- **Egress allowlist** (hosts or patterns the actor may `fetch`)
- Port (and related publish) hints; **IPv6 assignment remains manager authority**

## Bindings (`env`)

One **bindings API** for everything host-injected. v1 and later differ in *which* bindings exist, not in shape.

| Binding family | v1 | Later |
|----------------|----|--------|
| **Assets** | Yes — read-only files from the package | |
| **Secrets** | Yes — values injected at install by manager | |
| **Configuration** | Yes — non-secret config bindings | |
| **Storage** (KV/SQL/etc.) | Not required for first demo | When actors need durability |
| **Devices (HAL)** | Same API shape reserved | Camera, GPU, sensors, … via host drivers |
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

This proves: identity, pair, sign/install, sandbox, bindings (assets/secrets/config), egress policy, userspace overlay, IPv6-per-actor publish.

## Implementation notes (base)

- **Language:** Go, pure Go preferred (**no CGO**).
- **JS host:** goja + host polyfills/bindings for WinterTC subset.
- **Identity:** deterministic key material where already started (e.g. manager/device keys; mesh keys derived or minted from the same trust root). Existing `pkg/identity` is salvageable if useful; not sacred.
- **Codebase attitude:** selective reuse of current tree and branches only when it clearly fits; **do not overfit architecture to existing checkpoint code**. One runtime story (goja), not dual goja/QuickJS product paths.
- **Android lifecycle:** best-effort background stickiness; correctness must not require promising 24/7 uptime on stock Android in v1 docs. Mesh and actors reconnect when the worker runs.
- **Scale:** small — a handful of personal devices per owner; not a cluster product.

## Milestone sketch

1. **Runtime contract** — goja isolate, WinterTC subset, `fetch` handler, timers, assets/secrets/config bindings, esbuild pipeline.
2. **Package + manager** — zip + `orvalho.json`, sign/verify, localhost daemon/CLI/UI, permission consent.
3. **Mesh + publish** — wireguard-go, pair, IPv6-per-actor, HTTP serve on actor address.
4. **Reference** — Astro SSR cat API on a worker (Linux, then Android).
5. **Later** — device bindings (HAL), durable storage bindings, actor-exported APIs, older Android backports, optional discovery UX / app store experiments.

## Open details (deliberately not frozen here)

Exact `orvalho.json` schema, ULA prefix math, HTTP vs HTTPS on the mesh, per-actor resource limit numbers, relay deployment topology, and Android packaging specifics — decide in implementation as the base lands.
