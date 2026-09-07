# testdata

`node/` is Node.js `test/common` and `test/parallel` from `nodejs/node@v24.11.0`,
placed by `workspaced codebase apply` (gitignored).

`claims` lists paths under `node/` that `go test ./pkg/workers/ -run TestNodeClaims`
and `orvalho script run testdata/node/<path>` must pass. Add a line to claim a test.
