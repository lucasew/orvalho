package workspaced

// Upstream Node.js tests, placed JIT (gitignored). Not a submodule.
// Pin matches process.versions.node major used by script run.
#node_tests: {
	from:    "github:nodejs/node"
	version: "v24.11.0"
}

workspaced: {
	inputs: {
		node: {
			from:    #node_tests.from
			version: #node_tests.version
		}
	}
	modules: {
		node_tests: {
			from: "core:place"
			config: {
				items: {
					"testdata/node/common":   "node:test/common"
					"testdata/node/parallel": "node:test/parallel"
				}
			}
		}
	}
}
