package workers

import (
	"testing"

	"github.com/lucasew/orvalho/pkg/imports"
)

func TestRollupParseAstUsesAcorn(t *testing.T) {
	iso := New("", Options{
		Imports: append(NodeScriptImports(), importMap(map[string]any{
			"acorn": imports.Script{Source: `
				exports.parse = function (code, opts) {
					return {
						type: "Program",
						sourceType: "module",
						body: [{
							type: "ImportDeclaration",
							start: 0,
							end: 24,
							specifiers: [{ type: "ImportDefaultSpecifier", local: { type: "Identifier", name: "x" }, imported: null }],
							source: { type: "Literal", value: "y", start: 14, end: 17 }
						}]
					};
				};
			`},
		})...),
	})
	err := iso.ScriptMain(t.Context(), `
		var m = require("rollup/parseAst");
		var ast = m.parseAst("import x from \"y\";\nexport default x;\n");
		if (ast.type !== "Program") throw new Error("type " + ast.type);
		if (ast.body[0].type !== "ImportDeclaration") throw new Error("body " + ast.body[0].type);
		if (ast.body[0].source.value !== "y") throw new Error("src " + ast.body[0].source.value);
		var n = 0;
		m.parseAstAsync("export default 1;").then(function (a) {
			if (a.body.length !== 1) throw new Error("async");
			n = 1;
		});
		setTimeout(function () { if (n !== 1) throw new Error("no async"); }, 0);
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
