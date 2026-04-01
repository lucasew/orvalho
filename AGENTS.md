# AGENTS.md

## Project Conventions
- **Domain-driven structure:** Packages must have a single responsibility. Domain logic should be abstracted into pure functions for testability, separating it from external world interactions.
- **Explicit naming:** Avoid vague names.
- **Error handling:** Unexpected or background errors must be reported via `pkg/observability.ReportError` rather than direct logging or silent failures. All error handling paths must funnel through this centralized function. Never ignore errors or assign to `_` (blank identifier). All errors must be explicitly checked and handled.
- **CGO:** Strict constraint: The project must not use CGO dependencies. Use pure Go alternatives like `github.com/dop251/goja` and `github.com/ebitengine/purego`.

## Operational Memory
- `pkg/actor` -> VM logic and interfaces
- `pkg/actor/js/actor.d.ts` -> JS ambient API declarations boundaries
- `pkg/identity` -> cryptography
- `pkg/observability` -> centralized error reporting
