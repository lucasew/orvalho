## IGNORE: LLM Monologue Leaks

**- Pattern:** Leaving internal thought processes, AI conversational filler, or prompt references in source code comments (e.g., `// The prompt says...`, `// User didn't ask for...`, `// Let's respect user input for now.`).
**- Justification:** Source code comments must document the "why" for human developers, not the AI's internal reasoning or conversational filler.
**- Files Affected:** `pkg/actor/js/runtime.go`, `pkg/identity/identity.go`

## IGNORE: Broad Git Staging in CI Workflows

**- Pattern:** Using `git add -A` or `git add .` inside automated CI/CD scripts or workflows (e.g., in `.github/workflows/autorelease.yml` for codegen).
**- Justification:** Violates the strict scope boundaries constraint: "Never use `git add -A` or `git add .`. Stage files explicitly with `git add <path>`".
**- Files Affected:** `.github/workflows/autorelease.yml`

## IGNORE: Downgrading CI Dependencies

**- Pattern:** Using major version tags like `@v4` for `actions/checkout` or `@v2` for `mise-action` instead of explicitly pinning exact versions, or downgrading them explicitly.
**- Justification:** Global instructions strictly require pinning exact versions of tools and dependencies, and explicitly forbid downgrading dependencies like `actions/checkout` to `v4` or `mise-action` to `v2`.
**- Files Affected:** `.github/workflows/autorelease.yml`

## IGNORE: Ineffectual Assignments in Tests

**- Pattern:** Assigning returned errors to existing variables using `=` without proper declaration (`:=`), leading to compilation errors (e.g., `_, err = r.Tick(ctx)` when `err` is undeclared).
**- Justification:** Causes basic Go compilation errors (`undefined: err`), breaking the build and test pipeline.
**- Files Affected:** `pkg/actor/js/runtime_test.go`

## IGNORE: Exposing Go Internals in JS API Docs

**- Pattern:** Documenting internal Go implementation details (e.g., "min-heap", "Go heap", "TimerManager", "Tick loop") inside the TSDoc for the JavaScript boundary API (`actor.d.ts`).
**- Justification:** `actor.d.ts` defines the ambient JS environment. Leaking Go-specific architectural details into the JS abstraction boundary violates encapsulation and adds unnecessary noise.
**- Files Affected:** `pkg/actor/js/actor.d.ts`

## IGNORE: Fixing Starvation via Timer Clamping

**- Pattern:** Attempting to prevent infinite tight loops or actor starvation by clamping timer intervals to `1ms` (e.g., `if t.interval < time.Millisecond { t.interval = time.Millisecond }`).
**- Justification:** Starvation should be handled via batch processing limits (e.g., `maxOpsPerTick = 1000`) and checking context cancellation, not by artificially altering the developer's requested timer interval.
**- Files Affected:** `pkg/actor/js/runtime.go`

## IGNORE: Inadequate Context in Centralized Error Reporting

**- Pattern:** Implementing the centralized error reporting function (e.g., `ReportError`) with basic logging (`log.Printf`, `fmt.Fprintf`) that omits stack traces and relevant metadata.
**- Justification:** Global instructions mandate that if Sentry is not set up, the error must be logged with enough context, specifically requiring message, stack, and metadata. Basic print statements do not satisfy this requirement.
**- Files Affected:** `pkg/observability/observability.go`, `pkg/observability/errors.go`

## IGNORE: Bundling Independent Changes

**- Pattern:** Bundling multiple independent changes (e.g., simultaneous refactoring in disparate domains or mixing logic fixes with tooling updates) into a single Pull Request.
**- Justification:** Pull Requests must be atomic and strictly scoped to avoid unrelated changes that complicate code review.
**- Files Affected:** Broad

## IGNORE: Unchecked Errors

**- Pattern:** Ignoring error returns (e.g., `r.Tick(ctx)`) or assigning them to `_` instead of explicitly checking and handling them.
**- Justification:** Error returns must never be ignored or assigned to the blank identifier. All errors must be explicitly checked and handled, including in tests.
**- Files Affected:** `pkg/actor/js/runtime_test.go`, `pkg/actor/js/runtime.go`
