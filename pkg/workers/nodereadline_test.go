package workers

import "testing"

func runNodeReadline(t *testing.T, src string) {
	t.Helper()
	iso := New("", Options{Imports: NodeScriptImports()})
	if err := iso.ScriptMain(t.Context(), src, "t.js"); err != nil {
		t.Fatal(err)
	}
}

func TestNodeReadlineIdentity(t *testing.T) {
	runNodeReadline(t, `
		var rl = require("readline");
		if (require("node:readline") !== rl) throw new Error("identity");
		if (typeof rl.createInterface !== "function") throw new Error("createInterface");
		if (typeof rl.Interface !== "function") throw new Error("Interface");
		if (typeof rl.emitKeypressEvents !== "function") throw new Error("emitKeypressEvents");
		if (typeof rl.clearLine !== "function") throw new Error("clearLine");
		if (typeof rl.clearScreenDown !== "function") throw new Error("clearScreenDown");
		if (typeof rl.cursorTo !== "function") throw new Error("cursorTo");
		if (typeof rl.moveCursor !== "function") throw new Error("moveCursor");
	`)
}

const readlineFakeStream = `
function fakeStream() {
	var ev = Object.create(null);
	return {
		on: function (name, fn) {
			(ev[name] = ev[name] || []).push(fn);
			return this;
		},
		removeListener: function (name, fn) {
			var list = ev[name] || [];
			ev[name] = list.filter(function (f) { return f !== fn; });
			return this;
		},
		emit: function (name) {
			var args = [].slice.call(arguments, 1);
			var list = (ev[name] || []).slice();
			for (var i = 0; i < list.length; i++) list[i].apply(this, args);
			return this;
		},
		write: function (chunk) {
			this.written = (this.written || "") + String(chunk);
			return true;
		}
	};
}
`

func TestNodeReadlineAsyncIterator(t *testing.T) {
	runNodeReadline(t, readlineFakeStream+`
		var input = fakeStream();
		var rl = require("readline").createInterface({ input: input });
		var it = rl[Symbol.asyncIterator]();
		if (!it || typeof it.next !== "function") throw new Error("iterator");
		var got = [];
		var p = it.next().then(function (a) {
			got.push(a.value);
			return it.next();
		}).then(function (b) {
			got.push(b.value);
			return it.next();
		}).then(function (c) {
			if (!c.done) throw new Error("not done");
			if (got.join(",") !== "a,b") throw new Error("got " + got);
		});
		input.emit("data", "a\nb\n");
		input.emit("end");
		var finished = false;
		p.then(function () { finished = true; });
		setTimeout(function () {
			if (!finished) throw new Error("iterator hung");
		}, 0);
	`)
}

func TestNodeReadlineLines(t *testing.T) {
	runNodeReadline(t, readlineFakeStream+`
		var input = fakeStream();
		var lines = [];
		var closed = false;
		var rl = require("readline").createInterface({ input: input });
		rl.on("line", function (line) { lines.push(line); });
		rl.on("close", function () { closed = true; });
		input.emit("data", "a\nb\n");
		input.emit("end");
		if (lines.length !== 2 || lines[0] !== "a" || lines[1] !== "b") {
			throw new Error("lines " + JSON.stringify(lines));
		}
		if (!closed) throw new Error("no close");
	`)
}

func TestNodeReadlineConstructorAndSeparators(t *testing.T) {
	runNodeReadline(t, readlineFakeStream+`
		var input = fakeStream();
		var lines = [];
		var rl = new (require("readline").Interface)({ input: input });
		rl.on("line", function (line) { lines.push(line); });
		input.emit("data", "012\n345\r67\r\n89\u2028ABC\u2029DEF");
		input.emit("end");
		var want = ["012", "345", "67", "89", "ABC", "DEF"];
		if (JSON.stringify(lines) !== JSON.stringify(want)) {
			throw new Error("lines " + JSON.stringify(lines));
		}
	`)
}

func TestNodeReadlineCRLFAcrossChunks(t *testing.T) {
	runNodeReadline(t, readlineFakeStream+`
		var input = fakeStream();
		var lines = [];
		var rl = require("readline").createInterface({ input: input, crlfDelay: Infinity });
		rl.on("line", function (line) { lines.push(line); });
		input.emit("data", "a\nb");
		input.emit("data", "\r\n");
		input.emit("end");
		if (lines.length !== 2 || lines[0] !== "a" || lines[1] !== "b") {
			throw new Error("lines " + JSON.stringify(lines));
		}
		if (String(lines[1]).indexOf("\r") >= 0) throw new Error("kept CR");
	`)
}

func TestNodeReadlineNoTrailingNewline(t *testing.T) {
	runNodeReadline(t, readlineFakeStream+`
		var input = fakeStream();
		var lines = [];
		var rl = require("readline").createInterface({ input: input });
		rl.on("line", function (line) { lines.push(line); });
		input.emit("data", "hello");
		input.emit("end");
		if (lines.length !== 1 || lines[0] !== "hello") {
			throw new Error("lines " + JSON.stringify(lines));
		}
	`)
}

func TestNodeReadlineQuestionAndWrite(t *testing.T) {
	runNodeReadline(t, readlineFakeStream+`
		var input = fakeStream();
		var output = fakeStream();
		var rl = require("readline").createInterface({ input: input, output: output });
		var got;
		rl.question("name? ", function (ans) { got = ans; });
		if (output.written !== "name? ") throw new Error("prompt " + JSON.stringify(output.written));
		rl.write("ada\n");
		if (got !== "ada") throw new Error("answer " + got);
		rl.close();
	`)
}

func TestNodeReadlineCursorHelpers(t *testing.T) {
	runNodeReadline(t, readlineFakeStream+`
		var rl = require("readline");
		var out = fakeStream();
		rl.cursorTo(out, 0, 0);
		rl.clearScreenDown(out);
		rl.clearLine(out, 1);
		rl.moveCursor(out, 1, -1);
		if (out.written.indexOf("\x1b[") < 0) throw new Error("no csi " + JSON.stringify(out.written));
		var called = false;
		rl.cursorTo(null, 0, 0, function () { called = true; });
		if (!called) throw new Error("callback");
		var stream = fakeStream();
		var keys = [];
		stream.on("keypress", function (ch, key) { keys.push(key.name); });
		rl.emitKeypressEvents(stream);
		stream.emit("data", "\r");
		if (keys[0] !== "return") throw new Error("key " + keys[0]);
	`)
}
