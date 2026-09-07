# testdata

`node/` is Node.js `test/common` and `test/parallel` from `nodejs/node@v24.11.0`,
placed by `workspaced codebase apply` (gitignored).

`claims` and `claims.d/*` list paths under `node/` that
`go test ./pkg/workers/ -run TestNodeClaims` and
`orvalho script run testdata/node/<path>` must pass.
Add a new file under `claims.d/` to claim a test.
