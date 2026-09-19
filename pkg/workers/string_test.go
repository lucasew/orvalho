package workers

import "testing"

func TestStringWellFormed(t *testing.T) {
	iso := New("", Options{})
	err := iso.ScriptMain(t.Context(), `
		if (typeof String.prototype.toWellFormed !== "function") throw new Error("toWellFormed");
		if (typeof String.prototype.isWellFormed !== "function") throw new Error("isWellFormed");
		if ("ok".toWellFormed() !== "ok") throw new Error("ascii");
		if (!"ok".isWellFormed()) throw new Error("ascii well");
		var lone = String.fromCharCode(0xD800);
		if (lone.isWellFormed()) throw new Error("lone high");
		if (lone.toWellFormed() !== "\uFFFD") throw new Error("replace");
		var pair = String.fromCharCode(0xD83D, 0xDE00);
		if (!pair.isWellFormed()) throw new Error("pair");
		if (pair.toWellFormed() !== pair) throw new Error("pair keep");
	`, "t.js")
	if err != nil {
		t.Fatal(err)
	}
}
