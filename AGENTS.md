# Agent and Developer Guidelines

This document outlines the coding conventions, architecture, and operational guidelines for the `orvalho` project. All contributors (human and AI) must adhere to these rules.

## Project Structure

The project follows a domain-driven structure within the `pkg/` directory.

- **`pkg/`**: Contains the core library code.
  - **`actor`**: Domain logic for actors.
    - **`js`**: JavaScript runtime implementation.
  - **`identity`**: Cryptographic identity management.
  - **`observability`**: Centralized logging and error reporting.

### Rules:
- **Group by Domain:** Things that change together should live together. Avoid grouping by file type (e.g., `controllers`, `models`).
- **Single Responsibility:** Each package and file should have a clear, single purpose.
- **Colocation:** Tests should be next to the code they test (e.g., `runtime.go` and `runtime_test.go`).

## Naming Conventions

- **Explicit Names:** Avoid cryptic abbreviations. Use `javascript` instead of `js` (unless standard like `http`, `json`).
- **Function Names:** Should be verbs or verb phrases (e.g., `DeriveIdentities`, `ReportError`).
- **Variable Names:** Should be descriptive enough to understand their purpose without context.

## Code Quality

- **Small Functions:** Functions should be small and focused. If a function does more than one thing, extract it.
- **Single Responsibility Principle (SRP):** Classes and modules should have one reason to change.
- **Error Handling:**
  - Never ignore errors.
  - Use the centralized `pkg/observability` package for reporting unexpected errors.
  - Libraries should return errors to the caller.
  - Background processes must log/report errors using `observability.ReportError`.

## Testing

- **Unit Tests:** Write unit tests for all logic.
- **Table-Driven Tests:** Prefer table-driven tests for multiple scenarios.
- **Mocks:** Use interfaces to allow mocking of dependencies (e.g., hardware access).

## Tools

- **Mise:** Use `mise` for environment management.
- **Linting:** Ensure code passes `go vet` and `staticcheck`.
