# Pesquisarr on `orvalho serve`

Bridge package for the [pesquisarr](https://github.com/lucasew/pesquisarr) Astro → Cloudflare Workers build.

## Assemble from a local pesquisarr checkout

```bash
# from orvalho repo root; adjust PESQUISARR path
export PESQUISARR=../pesquisarr
./examples/pesquisarr/assemble.sh
orvalho serve ./examples/pesquisarr --listen 127.0.0.1:8788
```

Requires `esbuild` on `PATH` (mise tools) for multi-file ESM bundle-on-load.

## Notes

- `agents.main.entrypoint` is `entry.mjs` (Astro CF adapter server entry).
- `ASSETS` binding roots at `assets/` (copied from `dist/client`).
- Full feature parity needs more host drivers (KV `SESSION`, etc.) and richer `nodejs_compat` stubs.
- Generated payload files under this directory are gitignored; only `orvalho.cue` + scripts are committed.
