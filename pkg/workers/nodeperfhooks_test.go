package workers

import "testing"

func runNodePerf(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodePerfHooksIdentity(t *testing.T) {
	runNodePerf(t, `
		var ph = require("perf_hooks");
		if (require("node:perf_hooks") !== ph) throw new Error("identity");
		if (ph.performance !== performance) throw new Error("performance identity");
		if (typeof ph.performance.now !== "function") throw new Error("now");
		var n = ph.performance.now();
		if (typeof n !== "number" || n !== n) throw new Error("now value " + n);
	`)
}

func TestNodePerfHooksHistogram(t *testing.T) {
	runNodePerf(t, `
		var h = require("perf_hooks").monitorEventLoopDelay();
		if (typeof h.enable !== "function") throw new Error("enable");
		if (h.percentile(50) !== 0) throw new Error("percentile");
	`)
}
