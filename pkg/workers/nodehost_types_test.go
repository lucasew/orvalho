package workers

import "testing"

func TestNodeHostTypesPredicates(t *testing.T) {
	iso := New("", Options{Imports: NodeScriptImports()})
	err := iso.ScriptMain(t.Context(), `
		var assert = require("assert");
		var types = require("util/types");

		assert.strictEqual(types, require("util").types);

		assert.ok(types.isDate(new Date()));
		assert.ok(!types.isDate({}));
		assert.ok(!types.isDate(Date.now()));

		assert.ok(types.isRegExp(/x/));
		assert.ok(types.isRegExp(new RegExp("x")));
		assert.ok(!types.isRegExp("x"));

		assert.ok(types.isNativeError(new Error()));
		assert.ok(types.isNativeError(new TypeError("t")));
		class MyError extends Error {}
		assert.ok(types.isNativeError(new MyError()));
		assert.ok(!types.isNativeError({ __proto__: Error.prototype }));
		assert.ok(!types.isNativeError({ message: "x" }));

		var ab = new ArrayBuffer(8);
		assert.ok(types.isArrayBuffer(ab));
		assert.ok(types.isAnyArrayBuffer(ab));
		assert.ok(!types.isSharedArrayBuffer(ab));
		assert.ok(!types.isArrayBuffer(new Uint8Array(ab)));

		var u8 = new Uint8Array(ab);
		assert.ok(types.isTypedArray(u8));
		assert.ok(types.isUint8Array(u8));
		assert.ok(types.isArrayBufferView(u8));
		assert.ok(!types.isTypedArray(ab));
		assert.ok(!types.isUint8Array({ [Symbol.toStringTag]: "Uint8Array" }));

		assert.ok(types.isDataView(new DataView(ab)));
		assert.ok(!types.isDataView(u8));

		assert.ok(types.isMap(new Map()));
		assert.ok(types.isSet(new Set()));
		assert.ok(types.isWeakMap(new WeakMap()));
		assert.ok(types.isWeakSet(new WeakSet()));
		assert.ok(!types.isMap(new Set()));
		assert.ok(!types.isSet(new Map()));

		assert.ok(types.isPromise(Promise.resolve(1)));
		assert.ok(!types.isPromise({ then: function () {} }));

		assert.ok(types.isAsyncFunction(async function () {}));
		assert.ok(!types.isAsyncFunction(function () {}));
		assert.ok(types.isGeneratorFunction(function* () {}));
		assert.ok(types.isGeneratorObject((function* () {})()));

		assert.ok(types.isMapIterator((new Map())[Symbol.iterator]()));
		assert.ok(types.isSetIterator((new Set())[Symbol.iterator]()));

		assert.ok(types.isArgumentsObject((function () { return arguments; })()));
		assert.ok(!types.isArgumentsObject([]));

		assert.ok(types.isBooleanObject(new Boolean(false)));
		assert.ok(!types.isBooleanObject(false));
		assert.ok(types.isNumberObject(new Number(1)));
		assert.ok(!types.isNumberObject(1));
		assert.ok(types.isStringObject(new String("x")));
		assert.ok(!types.isStringObject("x"));
		assert.ok(types.isSymbolObject(Object(Symbol("x"))));
		assert.ok(!types.isSymbolObject(Symbol("x")));
		assert.ok(types.isBoxedPrimitive(new Boolean(true)));
		assert.ok(!types.isBoxedPrimitive(true));

		assert.ok(!types.isProxy({}));
		assert.ok(!types.isProxy(1));
		var revoked = Proxy.revocable({}, {});
		revoked.revoke();
		assert.ok(types.isProxy(revoked.proxy));
	`, "types_pred.js")
	if err != nil {
		t.Fatal(err)
	}
}
