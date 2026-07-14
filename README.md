# orvalho

What if we could use phones as servers?

Product vision: [`SPEC.md`](./SPEC.md).

## Guest JS downlevel (goja)

Modern actor JS is downleveled with **esbuild** to **ES2015** before it runs on goja.

- Compat note / target rationale: [`docs/goja-compat.md`](./docs/goja-compat.md)
- Fixture pipeline: `tools/js-downlevel/`
- Build: `mise run js:downlevel`
- CI golden check: `mise run js:downlevel:check` (also part of `mise run ci`)
