package workers

import "testing"

func runNodeEvents(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeEventsIdentity(t *testing.T) {
	runNodeEvents(t, `
		var EE = require("events");
		if (require("node:events") !== EE) throw new Error("identity");
		if (EE.EventEmitter !== EE) throw new Error("EventEmitter");
		var ee = new EE();
		var n = 0;
		ee.on("x", function (v) { n = v; });
		ee.emit("x", 7);
		if (n !== 7) throw new Error("emit " + n);
		if (ee.eventNames()[0] !== "x") throw new Error("names");
	`)
}
