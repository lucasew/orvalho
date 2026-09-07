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
		imports.Alias[any]{From: "path/posix", To: "path"},
		imports.Alias[any]{From: "node:path/posix", To: "path"},
		nodePrefixed(imports.Map[any]{
			"util":          nodehostScript("nodehost/util.js"),
			"util/types":    nodehostScript("nodehost/types.js"),
			"assert":        nodehostScript("nodehost/assert.js"),
			"path":          nodehostScript("nodehost/path.js"),
			"path/win32":    nodehostScript("nodehost/path_win32.js"),
			"fs":            nodeFSBinding{},
			"fs/promises":   nodeFSPromisesBinding{},
			"module":        nodeModuleBinding{},
			"url":           nodeURLBinding{},
			"crypto":        nodeCryptoBinding{},
			"os":            nodeOSBinding{},
			"process":       nodeProcessBinding{},
			"child_process": nodeChildBinding{},
			"readline":      nodeReadlineBinding{},
			"perf_hooks":    nodePerfHooksBinding{},
		}),
		imports.Func[any](func(spec string, next imports.Resolver[any]) (any, error) {
			if spec == "common" || strings.HasSuffix(spec, "/common") || strings.HasSuffix(spec, "/common/index.js") {
				return nodehostScript("nodehost/common.js"), nil
			}
			return next(spec)
		}),
	}
}

// nodePrefixed claims both name and node:name for each Map entry.
func nodePrefixed(m imports.Map[any]) imports.Map[any] {
	out := make(imports.Map[any], len(m)*2)
	for k, v := range m {
		out[k] = v
		out["node:"+k] = v
	}
	return out
}

func nodehostScript(name string) imports.Script {
	data, err := nodehostFS.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return imports.Script{Source: string(data), File: name}
}
