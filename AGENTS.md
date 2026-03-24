# Agents Guide & Conventions

This file documents the conventions, rules, and architecture boundaries for the Orvalho project. All agents must read and respect these guidelines before operating.

## Project Overview
Orvalho is a mesh actor runtime written in Go, utilizing `github.com/dop251/goja` for isolated actor execution. The objective is to use phones as servers, leveraging hardware capabilities dynamically.

## Core Directives & Constraints

1.  **CGO is Strictly Forbidden:**
    - The project MUST NOT use CGO dependencies.
    - Instead of CGO, use pure Go alternatives. For example, use `github.com/dop251/goja` for JavaScript execution and `github.com/ebitengine/purego` for hardware binding. This allows dynamic access and fallback/mocking when native libraries are absent.

2.  **Centralized Error Handling:**
    - Unexpected or background errors MUST be reported via `pkg/observability.ReportError` (or the centralized equivalent).
    - NEVER log errors directly (e.g., no raw `fmt.Println`, `log.Fatal`, or silent failures).
    - Every code path handling unexpected errors must funnel through the centralized reporting function.
    - Errors MUST NEVER be ignored or assigned to `_`. All errors must be explicitly checked, handled, and correctly propagated, including in tests. Empty catch blocks or swallowed errors are prohibited.

3.  **Domain-Driven Architecture:**
    - Packages under `pkg/` must enforce single responsibility principles and clear boundaries.
    - **Domain logic** should be abstracted into pure functions for testability, fully separating it from external world interactions.

4.  **Hardware & External Access Boundaries:**
    - Actors access hardware via the `env.DEVICES` JavaScript API, interacting using methods like `list(type)` and `get(id)`.
    - Web API polyfills (like timers, fetch) are injected into the Goja runtime and backed by Go bridge functions.
    - The ambient JS API is explicitly documented via TSDoc in `pkg/actor/js/actor.d.ts` to outline boundaries clearly without directly touching the execution Go code.

5.  **Runtime & Execution Limits:**
    - The JS actor runtime operates via a step-based `Tick` loop (`pkg/actor/js`).
    - The `Tick(ctx context.Context) (bool, error)` method returns `true` if work is pending (timers, jobs) and `false` if idle.
    - To prevent starvation, the runtime limits timer processing (e.g., max 1000 ops/tick) and periodically checks context cancellation during batch processing to ensure responsiveness.
    - Interruption is handled via a goroutine monitoring `ctx.Done()` and calling `vm.Interrupt(ctx.Err())` to halt the Goja VM gracefully.

6.  **Tooling & CI/CD Restrictions:**
    - `mise` is used for environment management.
    - Never commit tooling artifacts, downloaded binaries, or installer scripts (e.g., `install-mise.sh`, generated executables). Compiled binaries (`orvalho`) must be excluded via `.gitignore`.
    - Tools must be explicitly pinned (no `latest` or `lts`), and dependencies must never be downgraded. `mise.toml` tasks MUST depend ONLY on wildcards (e.g., `test:*`).
    - CI/CD must be exactly one workflow file (`.github/workflows/autorelease.yml`) with a single job executing a strict sequence: Install, Codegen, PR on diff, CI, Release, and Artifact upload.

7.  **Key Derivation Constraints:**
    - Tests comparing OpenSSH PEM private keys must verify parsed key material or public keys, as `ssh.MarshalPrivateKey` checkints generate non-deterministic outputs.
    - Key derivation uses SLIP-0010 paths via `github.com/stellar/go` (exp/crypto/derivation). SSH uses `m/44'/59356'/0'/0'` and Age uses `m/44'/59356'/1'/0'`. Age strings are encoded using `github.com/btcsuite/btcd/btcutil/bech32`.

8.  **Pull Requests & Review Checkpoints:**
    - PRs must have specific titles matching the active agent role (e.g., `🛠️ Refactor: [Desc]`, `🛟 Arrumador: [Desc]`, `📝 Docs: [Desc]`).
    - The PR description MUST include mandatory checkpoint sections: `Assumptions`, `Alternatives Not Chosen`, `How To Pivot`, and `Next Knobs`.

## Agent Roles & Responsibilities

-   **Docs 📝:**
    -   Creates high-quality, value-driven documentation without altering executable logic. Modifies only allowed paths (`**/*.md`, `**/*.d.ts`, etc).
    -   Detects drift, fills gaps, and enforces "Essentialism" (no obvious comments). Focuses on the "why" and non-obvious nuances.

-   **Janitor 🧹:**
    -   Fixes lint and formatting issues, implements small quality improvements (< 50 lines) without changing behavior.
    -   Maintains a journal of reusable insights in `.jules/janitor.md`.

-   **Arrumador 🛟:**
    -   Configures tooling (e.g., `mise.toml`, CI/CD workflows, scripts).
    -   Does NOT modify source code to fix lint/test failures; limits changes strictly to configuration paths.

-   **Sentinel 🛡️:**
    -   Handles security vulnerability patterns (like resource exhaustion documented in `.jules/sentinel.md`).

## Operational Memory

*   `pkg/actor/` -> Core VM interfaces and logic.
*   `pkg/actor/js/` -> JavaScript runtime implementation using goja, handling step-based `Tick` execution and timers.
*   `pkg/actor/js/actor.d.ts` -> Definition of ambient capabilities available to actors (boundary API).
*   `pkg/identity/` -> Cryptography and deterministic key derivation logic.
*   `pkg/observability/` -> Centralized error reporting and logging.
*   `cmd/orvalho/` -> CLI implementation using `cobra`, with subcommands registered via `init` functions.
