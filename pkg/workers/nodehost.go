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
		imports.Alias[any]{From: "node:util", To: "util"},
		imports.Alias[any]{From: "node:assert", To: "assert"},
		imports.Alias[any]{From: "node:path", To: "path"},
		imports.Alias[any]{From: "path/posix", To: "path"},
		imports.Alias[any]{From: "node:path/posix", To: "path"},
		imports.Alias[any]{From: "node:path/win32", To: "path/win32"},
		imports.Alias[any]{From: "node:fs", To: "fs"},
		imports.Alias[any]{From: "node:module", To: "module"},
		imports.Alias[any]{From: "node:url", To: "url"},
		imports.Alias[any]{From: "node:crypto", To: "crypto"},
		imports.Alias[any]{From: "node:os", To: "os"},
		imports.Alias[any]{From: "node:process", To: "process"},
		imports.Alias[any]{From: "node:child_process", To: "child_process"},
		imports.Map[any]{
			"util":          nodehostScript("nodehost/util.js"),
			"util/types":    nodehostScript("nodehost/types.js"),
			"assert":        nodehostScript("nodehost/assert.js"),
			"path":          nodehostScript("nodehost/path.js"),
			"path/win32":    nodehostScript("nodehost/path_win32.js"),
			"fs":            nodeFSBinding{},
			"module":        nodeModuleBinding{},
			"url":           nodeURLBinding{},
			"crypto":        nodeCryptoBinding{},
			"os":            nodeOSBinding{},
			"process":       nodeProcessBinding{},
			"child_process": nodeChildBinding{},
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
