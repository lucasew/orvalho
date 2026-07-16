package js

import (
	"fmt"
	"os"
	"strings"

	"github.com/dop251/goja"
)

// installHostPolyfills adds minimal globals Astro/CF Workers bundles expect
// that are outside our WinterTC subset but required for real adapter output.
func (iso *Isolate) installHostPolyfills() {
	// console must exist before CF unenv patches Object.assign(console, …).
	// Log to host stderr so Astro/adapter errors are visible under serve.
	logFn := func(prefix string) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			parts := make([]string, 0, len(call.Arguments))
			for _, a := range call.Arguments {
				parts = append(parts, a.String())
			}
			fmt.Fprintf(os.Stderr, "[js %s] %s\n", prefix, strings.Join(parts, " "))
			return goja.Undefined()
		}
	}
	_ = iso.vm.Set("console", map[string]any{
		"log":   logFn("log"),
		"info":  logFn("info"),
		"warn":  logFn("warn"),
		"error": logFn("error"),
		"debug": logFn("debug"),
		"trace": logFn("trace"),
	})

	// atob / btoa (base64) — host-installed for CF/Astro bundles.
	_, _ = iso.vm.RunString(`
(function () {
  if (typeof globalThis.console === "undefined") {
    globalThis.console = { log: function(){}, info: function(){}, warn: function(){}, error: function(){}, debug: function(){}, trace: function(){} };
  }
  var B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=";
  if (typeof globalThis.btoa !== "function") {
    globalThis.btoa = function (input) {
      input = String(input);
      var str = "", i = 0;
      while (i < input.length) {
        var c1 = input.charCodeAt(i++);
        var c2 = input.charCodeAt(i++);
        var c3 = input.charCodeAt(i++);
        var e1 = c1 >> 2;
        var e2 = ((c1 & 3) << 4) | (c2 >> 4);
        var e3 = ((c2 & 15) << 2) | (c3 >> 6);
        var e4 = c3 & 63;
        if (isNaN(c2)) { e3 = e4 = 64; }
        else if (isNaN(c3)) { e4 = 64; }
        str += B64.charAt(e1) + B64.charAt(e2) + B64.charAt(e3) + B64.charAt(e4);
      }
      return str;
    };
  }
  if (typeof globalThis.atob !== "function") {
    globalThis.atob = function (input) {
      input = String(input).replace(/[^A-Za-z0-9\+\/\=]/g, "");
      var str = "", i = 0;
      while (i < input.length) {
        var e1 = B64.indexOf(input.charAt(i++));
        var e2 = B64.indexOf(input.charAt(i++));
        var e3 = B64.indexOf(input.charAt(i++));
        var e4 = B64.indexOf(input.charAt(i++));
        var c1 = (e1 << 2) | (e2 >> 4);
        var c2 = ((e2 & 15) << 4) | (e3 >> 2);
        var c3 = ((e3 & 3) << 6) | e4;
        str += String.fromCharCode(c1);
        if (e3 !== 64) str += String.fromCharCode(c2);
        if (e4 !== 64) str += String.fromCharCode(c3);
      }
      return str;
    };
  }
  if (typeof globalThis.TextEncoder !== "function") {
    globalThis.TextEncoder = function TextEncoder() {};
    globalThis.TextEncoder.prototype.encode = function (s) {
      s = String(s);
      var arr = [];
      for (var i = 0; i < s.length; i++) {
        var c = s.charCodeAt(i);
        if (c < 128) arr.push(c);
        else if (c < 2048) arr.push(192 | (c >> 6), 128 | (c & 63));
        else arr.push(224 | (c >> 12), 128 | ((c >> 6) & 63), 128 | (c & 63));
      }
      return new Uint8Array(arr);
    };
  }
  if (typeof globalThis.TextDecoder !== "function") {
    globalThis.TextDecoder = function TextDecoder() {};
    globalThis.TextDecoder.prototype.decode = function (buf) {
      if (!buf) return "";
      var a = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
      var out = "", i = 0;
      while (i < a.length) {
        var c = a[i++];
        if (c < 128) out += String.fromCharCode(c);
        else if (c > 191 && c < 224) {
          out += String.fromCharCode(((c & 31) << 6) | (a[i++] & 63));
        } else {
          out += String.fromCharCode(((c & 15) << 12) | ((a[i++] & 63) << 6) | (a[i++] & 63));
        }
      }
      return out;
    };
  }
  if (typeof globalThis.URL === "undefined" || typeof globalThis.URL.canParse !== "function") {
    var URLImpl = function URL(url, base) {
      url = String(url);
      if (base && url.indexOf("://") === -1) {
        base = String(base);
        if (url.charAt(0) === "/") {
          var m = base.match(/^(https?:\/\/[^\/]+)/);
          url = (m ? m[1] : base) + url;
        } else {
          url = base.replace(/\/?$/, "/") + url;
        }
      }
      this.href = url;
      var m2 = url.match(/^(https?:)\/\/([^\/\?#]+)([^?#]*)(\?[^#]*)?(#.*)?$/);
      if (m2) {
        this.protocol = m2[1];
        this.host = m2[2];
        this.hostname = m2[2].split(":")[0];
        this.port = m2[2].indexOf(":") >= 0 ? m2[2].split(":")[1] : "";
        this.pathname = m2[3] || "/";
        this.search = m2[4] || "";
        this.hash = m2[5] || "";
        this.origin = this.protocol + "//" + this.host;
      } else {
        this.protocol = "";
        this.host = "";
        this.hostname = "";
        this.port = "";
        this.pathname = url.charAt(0) === "/" ? url : "/" + url;
        this.search = "";
        this.hash = "";
        this.origin = "";
        // Relative path used as pathname for asset resolution.
        if (url.indexOf("://") === -1) {
          this.pathname = url.charAt(0) === "/" ? url : "/" + url;
          this.href = url;
        }
      }
      this.searchParams = {
        get: function () { return null; },
        set: function () {},
        toString: function () { return ""; }
      };
    };
    URLImpl.canParse = function (url, base) {
      try {
        new URLImpl(url, base);
        return true;
      } catch (e) {
        return false;
      }
    };
    URLImpl.prototype.toString = function () { return this.href; };
    URLImpl.prototype.toJSON = function () { return this.href; };
    globalThis.URL = URLImpl;
  }
  if (typeof globalThis.URLSearchParams === "undefined") {
    globalThis.URLSearchParams = function URLSearchParams() {
      this.get = function () { return null; };
      this.set = function () {};
      this.toString = function () { return ""; };
    };
  }
  if (typeof globalThis.queueMicrotask !== "function") {
    globalThis.queueMicrotask = function (fn) {
      Promise.resolve().then(fn);
    };
  }
  if (typeof globalThis.structuredClone !== "function") {
    globalThis.structuredClone = function (v) {
      return JSON.parse(JSON.stringify(v));
    };
  }
  if (typeof globalThis.performance === "undefined") {
    globalThis.performance = { now: function () { return Date.now(); } };
  }
  if (typeof globalThis.crypto === "undefined" || !globalThis.crypto) {
    globalThis.crypto = {};
  }
  if (typeof globalThis.crypto.getRandomValues !== "function") {
    globalThis.crypto.getRandomValues = function (arr) {
      for (var i = 0; i < arr.length; i++) arr[i] = (Math.random() * 256) | 0;
      return arr;
    };
  }
  if (typeof globalThis.crypto.subtle === "undefined") {
    globalThis.crypto.subtle = {
      digest: function () { return Promise.resolve(new ArrayBuffer(32)); },
      encrypt: function () { return Promise.resolve(new ArrayBuffer(0)); },
      decrypt: function () { return Promise.resolve(new ArrayBuffer(0)); },
      sign: function () { return Promise.resolve(new ArrayBuffer(32)); },
      verify: function () { return Promise.resolve(true); },
      generateKey: function () { return Promise.resolve({}); },
      importKey: function () { return Promise.resolve({}); },
      exportKey: function () { return Promise.resolve(new ArrayBuffer(0)); },
    };
  }
  if (typeof globalThis.crypto.randomUUID !== "function") {
    globalThis.crypto.randomUUID = function () {
      return "00000000-0000-4000-8000-000000000000";
    };
  }
  // Minimal Streams (Astro SSR). Buffers enqueued chunks for host Response.text().
  if (typeof globalThis.ReadableStream === "undefined") {
    function chunkToString(c) {
      if (c == null) return "";
      if (typeof c === "string") return c;
      if (c instanceof Uint8Array) return new TextDecoder().decode(c);
      if (typeof ArrayBuffer !== "undefined" && c instanceof ArrayBuffer) {
        return new TextDecoder().decode(new Uint8Array(c));
      }
      return String(c);
    }
    globalThis.ReadableStream = function ReadableStream(underlying) {
      this._underlying = underlying || {};
      this._chunks = [];
      this._closed = false;
      this.locked = false;
      var self = this;
      var controller = {
        enqueue: function (c) { self._chunks.push(c); },
        close: function () { self._closed = true; },
        error: function () { self._closed = true; },
      };
      if (typeof this._underlying.start === "function") {
        try { this._underlying.start(controller); } catch (e) {}
      }
      this.getReader = function () {
        self.locked = true;
        var i = 0;
        return {
          read: function () {
            if (typeof self._underlying.pull === "function" && !self._closed) {
              try { self._underlying.pull(controller); } catch (e) {}
            }
            if (i < self._chunks.length) {
              return Promise.resolve({ done: false, value: self._chunks[i++] });
            }
            return Promise.resolve({ done: true, value: undefined });
          },
          cancel: function () { return Promise.resolve(); },
          releaseLock: function () { self.locked = false; },
        };
      };
      this.cancel = function () { return Promise.resolve(); };
      this.tee = function () { return [self, self]; };
      this._orvalhoText = function () {
        var out = "";
        for (var j = 0; j < self._chunks.length; j++) out += chunkToString(self._chunks[j]);
        return out;
      };
    };
    globalThis.WritableStream = function WritableStream() {
      this.getWriter = function () {
        return {
          write: function () { return Promise.resolve(); },
          close: function () { return Promise.resolve(); },
          abort: function () { return Promise.resolve(); },
          releaseLock: function () {},
        };
      };
    };
    globalThis.TransformStream = function TransformStream() {
      this.readable = new globalThis.ReadableStream();
      this.writable = new globalThis.WritableStream();
    };
  }
  // Stub WebAssembly (devalue/etc. compile WASM at load; goja has none).
  if (typeof globalThis.WebAssembly === "undefined") {
    var emptyExports = {};
    globalThis.WebAssembly = {
      compile: function () { return Promise.resolve({}); },
      instantiate: function () {
        return Promise.resolve({ instance: { exports: emptyExports }, module: {} });
      },
      compileStreaming: function () { return Promise.resolve({}); },
      instantiateStreaming: function () {
        return Promise.resolve({ instance: { exports: emptyExports }, module: {} });
      },
      Module: function () {},
      Instance: function () { this.exports = emptyExports; },
      Memory: function () { this.buffer = new ArrayBuffer(65536); },
      Table: function () {},
      validate: function () { return false; },
    };
  }
  // Minimal Intl for Astro/CF bundles (goja has no Intl).
  if (typeof globalThis.Intl === "undefined") {
    function DTF() {}
    DTF.prototype.format = function (d) { return String(d); };
    DTF.prototype.formatToParts = function () { return []; };
    DTF.prototype.resolvedOptions = function () {
      return { locale: "en", timeZone: "UTC", numberingSystem: "latn" };
    };
    function NF() {}
    NF.prototype.format = function (n) { return String(n); };
    NF.prototype.resolvedOptions = function () { return { locale: "en" }; };
    function RTF() {}
    RTF.prototype.format = function () { return ""; };
    function LF() {}
    LF.prototype.format = function (a) { return (a || []).join(", "); };
    function Collator() {}
    Collator.prototype.compare = function (a, b) {
      return a < b ? -1 : a > b ? 1 : 0;
    };
    function PR() {}
    PR.prototype.select = function () { return "other"; };
    function DN() {}
    DN.prototype.of = function (x) { return x; };
    globalThis.Intl = {
      DateTimeFormat: DTF,
      NumberFormat: NF,
      RelativeTimeFormat: RTF,
      ListFormat: LF,
      Collator: Collator,
      PluralRules: PR,
      DisplayNames: DN,
      Locale: function (t) { this.baseName = String(t || "en"); },
      supportedValuesOf: function () { return []; },
      getCanonicalLocales: function (x) {
        return Array.isArray(x) ? x.slice() : [x];
      },
    };
  }
})();
`)
}
