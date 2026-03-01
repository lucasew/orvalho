# Orvalho Project Conventions

## Core Principles
1. **Domain-Driven Structure**: Group code by domain and responsibility, not file type. Things that change together should live together.
2. **Explicit Naming**: Rename cryptic or misleading names to be self-documenting.
3. **Single Responsibility**: Methods and structures should have a single, clear purpose. Extract abstractions when duplication occurs at least three times (Rule of Three).

## Directory Layout
* `pkg/actor` -> VM logic and interfaces for the actor model.
* `pkg/actor/js` -> JavaScript runtime implementation using goja.
* `pkg/identity` -> Deterministic key derivation (SSH/Age) from BIP-39 mnemonics.
* `pkg/observability` -> Centralized error reporting.

## Observability & Error Handling
* **No Silent Failures**: Every unexpected or background error must be reported.
* **Centralized Reporting**: Use `pkg/observability.ReportError` for all unexpected errors rather than direct logging or silent failures. Never ignore errors.

## Technical Constraints
* **No CGO**: The project must not use CGO dependencies. Use pure Go alternatives (e.g., `github.com/dop251/goja`, `github.com/ebitengine/purego`).
* **Environment Management**: The project uses `mise` for environment management. Required Go versions and task definitions (e.g., `test`, `ci`) are in `mise.toml`. Use `mise run test` or `mise run ci` for verification.

## Testing Guidelines
* **Test Isolation**: Tests should verify the actual implementation, not just exercise external libraries.
* **Key Testing**: Tests comparing OpenSSH PEM private keys must verify parsed key material or public keys, as `ssh.MarshalPrivateKey` produces non-deterministic output containing random checkints.

## Security
* **Resource Exhaustion**: Ensure safeguards against resource exhaustion (e.g., bounded timer processing in JS runtime) as documented in security patterns.

## Build Artifacts
* Compiled binaries (e.g., `orvalho`) and environment directories (e.g., `.mise`) must be excluded from version control via `.gitignore`.
