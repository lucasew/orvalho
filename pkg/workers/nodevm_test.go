package workers

import "testing"

func runNodeVM(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeVMIdentity(t *testing.T) {
	runNodeVM(t, `
		var vm = require("vm");
		if (require("node:vm") !== vm) throw new Error("identity");
		if (vm.default !== vm) throw new Error("default");
		if (typeof vm.runInThisContext !== "function") throw new Error("runInThisContext");
		if (typeof vm.Script !== "function") throw new Error("Script");
		if (typeof vm.createContext !== "function") throw new Error("createContext");
	`)
}

func TestNodeVMRunInThisContextJitiShape(t *testing.T) {
	runNodeVM(t, `
		var vm = require("node:vm");
		var fn = vm.runInThisContext(
			"(function (exports, require, module, __filename, __dirname) { exports.n = 1 + 2; })",
			{filename: "/tmp/jiti-mod.js", lineOffset: 0, displayErrors: false}
		);
		if (typeof fn !== "function") throw new Error("fn " + typeof fn);
		var mod = {exports: {}};
		fn(mod.exports, require, mod, "/tmp/jiti-mod.js", "/tmp");
		if (mod.exports.n !== 3) throw new Error("n " + mod.exports.n);
	`)
}

func TestNodeVMScript(t *testing.T) {
	runNodeVM(t, `
		var vm = require("node:vm");
		var s = new vm.Script("1 + 6", {filename: "s.js"});
		if (s.runInThisContext() !== 7) throw new Error("script");
	`)
}

func TestNodeVMCreateContext(t *testing.T) {
	runNodeVM(t, `
		var vm = require("node:vm");
		var ctx = vm.createContext({x: 2});
		if (!vm.isContext(ctx)) throw new Error("isContext");
		if (vm.isContext({})) throw new Error("plain");
		var n = vm.runInNewContext("x + 1", {x: 4});
		if (n !== 5) throw new Error("newctx " + n);
	`)
}

func TestNodeVMCompileFunction(t *testing.T) {
	runNodeVM(t, `
		var vm = require("node:vm");
		var fn = vm.compileFunction("return a + b;", ["a", "b"]);
		if (fn(2, 3) !== 5) throw new Error("compile " + fn(2, 3));
	`)
}
