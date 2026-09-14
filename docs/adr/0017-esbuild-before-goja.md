# ADR-0017: esbuild before goja

Status: accepted
Date: 2026-09-07

## Context

goja parses ES5.1 and most of ES2015. Guest files use later syntax:
`export`, `import.meta`, optional `catch`. A `NeedsBundle` scan misses
those forms. Teaching goja each new production is a parser fork.
ADR-0006: goja is not an interface.

esbuild `pkg/api` is already the adopted compiler (ADR-0003, TEC-05, INV-05).

## Decision

`script run` compiles guest source in memory with esbuild `pkg/api` to
CommonJS ES2015 before goja evaluates it. When the file exists on disk,
the compile is a bundle: resolve and downlevel the graph. When the
source is not on disk (tests, embed), the compile is a single-file
transform.

goja stays the evaluator. Syntax newer than the goja surface is
esbuild's job.

## Rejected

- Extending goja to parse newer syntax.
- A source heuristic (`NeedsBundle`) that skips compile when `import`
  or `from` are absent.
- Exec of `esbuild` on `PATH` (ADR-0003).
