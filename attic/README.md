# Attic

This directory holds **previous experimental product code** kept for reference only.

- **Not product code.** Nothing under `attic/` is part of the live Orvalho API or delivery surface.
- **Not on the product import path.** Live packages must not import `orvalho/attic/...`. Prefer conscious cherry-picks (or rewriting against SPEC.md) over wiring attic in by default.
- **Selective reuse.** Steal ideas or small chunks when useful; do not treat this tree as the foundation for new features.

The locked product vision lives in [`SPEC.md`](../SPEC.md) at the repo root. New work belongs outside `attic/`.
