package workers

import "testing"

func runNodeWorkerThreads(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeWorkerThreadsIdentity(t *testing.T) {
	runNodeWorkerThreads(t, `
		var wt = require("worker_threads");
		if (require("node:worker_threads") !== wt) throw new Error("identity");
		if (wt.isMainThread !== true) throw new Error("isMainThread");
		if (wt.parentPort !== null) throw new Error("parentPort");
		if (wt.threadId !== 0) throw new Error("threadId");
		if (typeof wt.Worker !== "function") throw new Error("Worker");
		if (typeof wt.MessageChannel !== "function") throw new Error("MessageChannel");
		if (typeof wt.receiveMessageOnPort !== "function") throw new Error("receiveMessageOnPort");
		if (wt.default !== wt) throw new Error("default");
		var ch = new wt.MessageChannel();
		if (typeof ch.port1.postMessage !== "function") throw new Error("port1");
		if (typeof ch.port2.on !== "function") throw new Error("port2");
		var w = new wt.Worker("x.js");
		if (typeof w.postMessage !== "function") throw new Error("worker postMessage");
		if (typeof w.terminate !== "function") throw new Error("terminate");
		wt.setEnvironmentData("k", 1);
		if (wt.getEnvironmentData("k") !== 1) throw new Error("env");
	`)
}
