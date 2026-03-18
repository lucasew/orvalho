# Project Conventions

## General Rules
- Project conventions enforce a domain-driven structure, explicit naming, and single responsibility principles.
- Each package must have a single responsibility. Domain logic should be abstracted into pure functions for testability, separating it from external world interactions.
- Unexpected or background errors must be reported via `pkg/observability.ReportError` rather than direct logging or silent failures. All code paths that handle unexpected errors MUST funnel through this centralized error reporting function. Never call `console.error` or `Sentry.captureException` directly at the call site. No silent failures are allowed.
- The project 'orvalho' is a mesh actor runtime written in Go using `github.com/dop251/goja` for isolated actor execution.
- Strict constraint: The project must not use CGO dependencies. Use pure Go alternatives like `github.com/dop251/goja` and `github.com/ebitengine/purego`.

## Memory & Hardware
- Hardware capabilities (GPU, Camera) are implemented using `purego` to bind to native libraries dynamically, allowing for mocking when libraries are absent.
- The JavaScript implementation (`pkg/actor/js`) uses a step-based `Tick` loop. Timer logic (`setTimeout`, `setInterval`) is managed by a `TimerManager` in `pkg/actor/js/timer.go`, while the main runtime logic resides in `runtime.go`.
- The `pkg/actor/js` runtime handles script execution cancellation by running a goroutine that monitors `ctx.Done()` and calls `vm.Interrupt(ctx.Err())` to halt the Goja VM.
- The `pkg/actor/js` runtime limits timer processing (e.g., 1000 ops/tick) and checks context cancellation periodically during batch processing to prevent actor starvation and ensure responsiveness.
- Web API polyfills (timers, fetch, console) are injected into the Goja runtime and backed by Go bridge functions.
- Actors access hardware via the `env.DEVICES` JavaScript API, which provides `list(type)` and `get(id)` methods.

## Tooling & CI/CD
- Never commit tooling bootstrap artifacts, downloaded binaries, or temporary installer scripts (e.g., `install-mise.sh`, generated executables).
- Compiled binaries (e.g., `orvalho`) must be excluded from version control via `.gitignore`.
- CLI implementation uses the `cobra` library. Subcommands are placed in separate files within `cmd/orvalho` and registered via `init` functions.
- GitHub Actions CI/CD must be defined in exactly one workflow file (`.github/workflows/autorelease.yml`) with a single job executing a strict flow: Install, Codegen, PR on diff, CI, Release, and Artifact upload (skipping Release/Artifacts for Vercel/Cloudflare).
- The project uses `mise` for environment management and task execution, including `workspaced` for all linting and formatting. Tools must be explicitly pinned (no latest/lts), and dependencies must never be downgraded. `mise.toml` tasks like `install`, `test`, and `codegen` must ONLY depend on wildcards (e.g., `test:*`).
- When configuring tooling (e.g., as 'Arrumador'), do not modify source code to fix lint or test failures. Restrict changes to allowed configuration paths (e.g., `mise.toml`, `.github/workflows/**`, `scripts/**`, `.tool-versions`).

## Security & Identity
- Security vulnerability patterns, such as resource exhaustion, are documented in `.jules/sentinel.md`.
- `pkg/identity` derives deterministic SSH (Ed25519) and Age keys from BIP-39 mnemonics using SLIP-0010 paths `m/44'/59356'/0'/0'` and `m/44'/59356'/1'/0'`.
- The function `DeriveIdentities` in `pkg/identity` has been renamed to `Derive(mnemonic, passphrase)` and refactored to use helper functions for SSH and Age key derivation.
- Tests comparing OpenSSH PEM private keys must verify parsed key material or public keys, as `ssh.MarshalPrivateKey` produces non-deterministic output containing random checkints.
- The project uses `github.com/tyler-smith/go-bip39` for mnemonics, `github.com/stellar/go` (exp/crypto/derivation) for key derivation, and `github.com/btcsuite/btcd/btcutil/bech32` for encoding Age identity strings.

## Interfaces & PR Checkpoints
- The `Actor` interface in `pkg/actor` requires a `Tick(ctx context.Context) (bool, error)` method. It returns `true` if more work is pending (timers, jobs) and `false` if idle.
- Pull Requests must include mandatory checkpoint sections: `Assumptions`, `Alternatives Not Chosen`, `How To Pivot`, and `Next Knobs`. Use specific PR titles for agent roles (e.g., `🛠️ Refactor: [Description]`, `🛟 Arrumador: [Description]`, `📝 Docs: [Description]`).

## Operational Memory
- `pkg/actor` -> VM logic, step-based execution interfaces (`Actor`).
- `pkg/actor/js` -> JavaScript Actor implementation using Goja, handles step-based Tick loops and bridging between JS/Go.
- `pkg/identity` -> Cryptography, deterministic SSH & Age key derivation from BIP-39.
- `pkg/observability` -> Centralized error reporting (`ReportError`).
- `cmd/orvalho` -> CLI entrypoints using Cobra.
- `.github/workflows/autorelease.yml` -> Strict singular CI/CD pipeline definition.
- `mise.toml` -> Environment management, task runner configuration.
