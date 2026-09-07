package workers

import (
	"embed"
	"strings"

	"github.com/lucasew/orvalho/pkg/imports"
)

//go:embed nodehost/*.js
var nodehostFS embed.FS

// NodeScriptImports is the require chain for Script run: claimed Node
// builtins plus a stub for Node's test/common helper.
func NodeScriptImports() []imports.Handler[any] {
	return []imports.Handler[any]{
		imports.Alias[any]{From: "assert", To: "node:assert"},
		imports.Alias[any]{From: "child_process", To: "node:child_process"},
		imports.Alias[any]{From: "crypto", To: "node:crypto"},
		imports.Alias[any]{From: "fs", To: "node:fs"},
		imports.Alias[any]{From: "fs/promises", To: "node:fs/promises"},
		imports.Alias[any]{From: "module", To: "node:module"},
		imports.Alias[any]{From: "os", To: "node:os"},
		imports.Alias[any]{From: "path", To: "node:path"},
		imports.Alias[any]{From: "path/posix", To: "node:path"},
		imports.Alias[any]{From: "node:path/posix", To: "node:path"},
		imports.Alias[any]{From: "path/win32", To: "node:path/win32"},
		imports.Alias[any]{From: "perf_hooks", To: "node:perf_hooks"},
		imports.Alias[any]{From: "process", To: "node:process"},
		imports.Alias[any]{From: "readline", To: "node:readline"},
		imports.Alias[any]{From: "tty", To: "node:tty"},
		imports.Alias[any]{From: "url", To: "node:url"},
		imports.Alias[any]{From: "util", To: "node:util"},
		imports.Alias[any]{From: "util/types", To: "node:util/types"},
		imports.Map[any]{
			"node:assert":        nodehostScript("nodehost/assert.js"),
			"node:child_process": nodeChildBinding{},
			"node:crypto":        nodeCryptoBinding{},
			"node:fs":            nodeFSBinding{},
			"node:fs/promises":   nodeFSPromisesBinding{},
			"node:module":        nodeModuleBinding{},
			"node:os":            nodeOSBinding{},
			"node:path":          nodehostScript("nodehost/path.js"),
			"node:path/win32":    nodehostScript("nodehost/path_win32.js"),
			"node:perf_hooks":    nodePerfHooksBinding{},
			"node:process":       nodeProcessBinding{},
			"node:readline":      nodeReadlineBinding{},
			"node:tty":           nodeTTYBinding{},
			"node:url":           nodeURLBinding{},
			"node:util":          nodehostScript("nodehost/util.js"),
			"node:util/types":    nodehostScript("nodehost/types.js"),
		},
		imports.Func[any](func(spec string, next imports.Resolver[any]) (any, error) {
			if spec == "common" || strings.HasSuffix(spec, "/common") || strings.HasSuffix(spec, "/common/index.js") {
				return nodehostScript("nodehost/common.js"), nil
			}
			return next(spec)
		}),
	}
}

func nodehostScript(name string) imports.Script {
	data, err := nodehostFS.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return imports.Script{Source: string(data), File: name}
}
