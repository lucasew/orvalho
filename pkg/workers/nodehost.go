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
		imports.Map[any]{
			"util":   nodehostScript("nodehost/util.js"),
			"assert": nodehostScript("nodehost/assert.js"),
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
