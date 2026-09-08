package workers

import (
	"fmt"
	"os"
	"strings"

	"github.com/dop251/goja"
)

// installHostPolyfills adds minimal globals Astro/CF Workers bundles expect
// that are outside our WinterTC subset but required for real adapter output.
func (iso *Isolate) installHostPolyfills() {
	iso.bindConsole()
	if g := iso.vm.Get("globalThis"); g != nil {
		mustRuntimeSet(iso.vm, "global", g)
	}

	// atob / btoa / URL / streams / crypto / Intl — one script for guest globals.
	_, _ = iso.vm.RunString(hostPolyfillScript)
	iso.installWebAssembly()
	iso.installEvalHook()
}

// bindConsole installs console.* that write to the host process stderr.
// Re-run after guest script init: CF unenv may replace console methods.
func (iso *Isolate) bindConsole() {
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
	con := iso.vm.NewObject()
	mustSet(con, "log", logFn("log"))
	mustSet(con, "info", logFn("info"))
	mustSet(con, "warn", logFn("warn"))
	mustSet(con, "error", logFn("error"))
	mustSet(con, "debug", logFn("debug"))
	mustSet(con, "trace", logFn("trace"))
	mustRuntimeSet(iso.vm, "console", con)
}

// hostPolyfillScript is injected into every isolate (before guest code).
const hostPolyfillScript = `
(function () {
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
  // Always install URLSearchParams: goja has none, and a stub get() that
  // returns null breaks Astro.url.searchParams (e.g. /search?query=…).
  function OrvalhoURLSearchParams(init) {
    this._pairs = [];
    var self = this;
    function appendPair(k, v) {
      self._pairs.push([String(k), String(v)]);
    }
    if (init == null || init === "") {
      // empty
    } else if (typeof init === "string") {
      var s = init.charAt(0) === "?" ? init.slice(1) : init;
      if (s) {
        var parts = s.split("&");
        for (var i = 0; i < parts.length; i++) {
          if (!parts[i]) continue;
          var eq = parts[i].indexOf("=");
          var k, v;
          if (eq < 0) {
            k = parts[i];
            v = "";
          } else {
            k = parts[i].slice(0, eq);
            v = parts[i].slice(eq + 1);
          }
          try {
            k = decodeURIComponent(k.replace(/\+/g, " "));
            v = decodeURIComponent(v.replace(/\+/g, " "));
          } catch (e) {}
          appendPair(k, v);
        }
      }
    } else if (typeof init === "object" && init !== null) {
      if (typeof init.forEach === "function") {
        init.forEach(function (v, k) { appendPair(k, v); });
      } else if (Array.isArray(init)) {
        for (var j = 0; j < init.length; j++) {
          appendPair(init[j][0], init[j][1]);
        }
      } else {
        for (var key in init) {
          if (Object.prototype.hasOwnProperty.call(init, key)) appendPair(key, init[key]);
        }
      }
    }
    this.get = function (name) {
      name = String(name);
      for (var i = 0; i < self._pairs.length; i++) {
        if (self._pairs[i][0] === name) return self._pairs[i][1];
      }
      return null;
    };
    this.getAll = function (name) {
      name = String(name);
      var out = [];
      for (var i = 0; i < self._pairs.length; i++) {
        if (self._pairs[i][0] === name) out.push(self._pairs[i][1]);
      }
      return out;
    };
    this.has = function (name) {
      return self.get(name) !== null;
    };
    this.set = function (name, value) {
      name = String(name);
      var found = false;
      for (var i = 0; i < self._pairs.length; i++) {
        if (self._pairs[i][0] === name) {
          if (!found) {
            self._pairs[i][1] = String(value);
            found = true;
          } else {
            self._pairs.splice(i, 1);
            i--;
          }
        }
      }
      if (!found) appendPair(name, value);
    };
    this.append = function (name, value) { appendPair(name, value); };
    this.delete = function (name) {
      name = String(name);
      self._pairs = self._pairs.filter(function (p) { return p[0] !== name; });
    };
    this.toString = function () {
      return self._pairs
        .map(function (p) {
          return encodeURIComponent(p[0]) + "=" + encodeURIComponent(p[1]);
        })
        .join("&");
    };
    this.forEach = function (fn) {
      for (var i = 0; i < self._pairs.length; i++) fn(self._pairs[i][1], self._pairs[i][0], self);
    };
    this.entries = function () {
      var i = 0, pairs = self._pairs;
      return {
        next: function () {
          if (i >= pairs.length) return { done: true, value: undefined };
          var p = pairs[i++];
          return { done: false, value: [p[0], p[1]] };
        },
      };
    };
    this.keys = function () {
      var i = 0, pairs = self._pairs;
      return {
        next: function () {
          if (i >= pairs.length) return { done: true, value: undefined };
          return { done: false, value: pairs[i++][0] };
        },
      };
    };
    this.values = function () {
      var i = 0, pairs = self._pairs;
      return {
        next: function () {
          if (i >= pairs.length) return { done: true, value: undefined };
          return { done: false, value: pairs[i++][1] };
        },
      };
    };
  }
  globalThis.URLSearchParams = OrvalhoURLSearchParams;

  if (typeof globalThis.URL === "undefined" || typeof globalThis.URL.canParse !== "function") {
    function orvalhoParseAbsURL(s) {
      var m = String(s).match(/^([a-zA-Z][a-zA-Z0-9+.-]*:)\/\/([^\/\?#]*)([^?#]*)(\?[^#]*)?(#.*)?$/);
      if (!m) return null;
      return { protocol: m[1], host: m[2], pathname: m[3] || "/", search: m[4] || "", hash: m[5] || "" };
    }
    function orvalhoNormalizePath(p) {
      var parts = p.split("/");
      var out = [];
      for (var i = 0; i < parts.length; i++) {
        var part = parts[i];
        if (part === ".") continue;
        if (part === "..") {
          if (out.length > 0 && out[out.length - 1] !== "") out.pop();
          continue;
        }
        if (part === "" && i !== 0 && i !== parts.length - 1) continue;
        out.push(part);
      }
      if (out.length === 0) return "/";
      var s = out.join("/");
      if (s.charAt(0) !== "/") s = "/" + s;
      return s;
    }
    function orvalhoResolveURL(base, rel) {
      var b = orvalhoParseAbsURL(base);
      if (!b) return String(base).replace(/\/?$/, "/") + rel;
      var path = rel;
      var search = "";
      var hash = "";
      var hashAt = path.indexOf("#");
      if (hashAt >= 0) {
        hash = path.slice(hashAt);
        path = path.slice(0, hashAt);
      }
      var qAt = path.indexOf("?");
      if (qAt >= 0) {
        search = path.slice(qAt);
        path = path.slice(0, qAt);
      }
      if (path.indexOf("//") === 0) return b.protocol + path + search + hash;
      var pathname;
      if (path.charAt(0) === "/") {
        pathname = orvalhoNormalizePath(path);
      } else if (path === "") {
        pathname = b.pathname;
        if (!search) search = b.search;
      } else {
        var dir = b.pathname;
        var slash = dir.lastIndexOf("/");
        dir = slash >= 0 ? dir.slice(0, slash + 1) : "/";
        pathname = orvalhoNormalizePath(dir + path);
      }
      return b.protocol + "//" + b.host + pathname + search + hash;
    }
    var URLImpl = function URL(url, base) {
      url = String(url);
      if (base != null && !/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(url)) {
        url = orvalhoResolveURL(String(base), url);
      }
      var parsed = orvalhoParseAbsURL(url);
      if (parsed) {
        this.protocol = parsed.protocol;
        this.host = parsed.host;
        this.hostname = parsed.host.split(":")[0];
        this.port = parsed.host.indexOf(":") >= 0 ? parsed.host.split(":").slice(1).join(":") : "";
        this.pathname = parsed.pathname;
        this.search = parsed.search;
        this.hash = parsed.hash;
        this.origin = parsed.protocol === "file:" ? "null" : parsed.protocol + "//" + parsed.host;
        this.href = parsed.protocol + "//" + parsed.host + parsed.pathname + parsed.search + parsed.hash;
      } else {
        var qAt = url.indexOf("?");
        var hAt = url.indexOf("#");
        var pathEnd = url.length;
        if (qAt >= 0) pathEnd = qAt;
        if (hAt >= 0 && hAt < pathEnd) pathEnd = hAt;
        this.protocol = "";
        this.host = "";
        this.hostname = "";
        this.port = "";
        this.pathname = url.slice(0, pathEnd) || "/";
        if (this.pathname.charAt(0) !== "/" && this.pathname.indexOf("://") === -1) {
          this.pathname = "/" + this.pathname.replace(/^\//, "");
        }
        this.search = qAt >= 0 ? url.slice(qAt, hAt >= 0 ? hAt : url.length) : "";
        this.hash = hAt >= 0 ? url.slice(hAt) : "";
        this.origin = "";
        this.href = url;
      }
      this.searchParams = new OrvalhoURLSearchParams(this.search);
    };
    URLImpl.canParse = function (url, base) {
      try { new URLImpl(url, base); return true; } catch (e) { return false; }
    };
    URLImpl.prototype.toString = function () { return this.href; };
    URLImpl.prototype.toJSON = function () { return this.href; };
    globalThis.URL = URLImpl;
  } else if (globalThis.URL && globalThis.URL.prototype) {
    // Ensure instances get real searchParams if a broken URL already exists.
  }
  if (typeof globalThis.Blob !== "function") {
    function OrvalhoBlob(parts, opts) {
      this._parts = parts || [];
      this.type = (opts && opts.type) || "";
      var n = 0;
      for (var i = 0; i < this._parts.length; i++) {
        var p = this._parts[i];
        n += p && typeof p.byteLength === "number" ? p.byteLength : String(p).length;
      }
      this.size = n;
    }
    OrvalhoBlob.prototype.arrayBuffer = function () { return Promise.resolve(new ArrayBuffer(this.size)); };
    OrvalhoBlob.prototype.text = function () {
      var out = "";
      for (var i = 0; i < this._parts.length; i++) out += String(this._parts[i]);
      return Promise.resolve(out);
    };
    OrvalhoBlob.prototype.slice = function () { return new OrvalhoBlob([], { type: this.type }); };
    OrvalhoBlob.prototype.stream = function () {
      return typeof ReadableStream === "function" ? new ReadableStream() : { getReader: function () { return { read: function () { return Promise.resolve({ done: true }); } }; } };
    };
    globalThis.Blob = OrvalhoBlob;
  }
  if (typeof globalThis.File !== "function") {
    function OrvalhoFile(parts, name, opts) {
      Blob.call(this, parts, opts);
      this.name = name || "";
      this.lastModified = (opts && opts.lastModified) || Date.now();
    }
    OrvalhoFile.prototype = Object.create(globalThis.Blob.prototype);
    OrvalhoFile.prototype.constructor = OrvalhoFile;
    globalThis.File = OrvalhoFile;
  }
  if (typeof globalThis.FormData !== "function") {
    function OrvalhoFormData() { this._pairs = []; }
    OrvalhoFormData.prototype.append = function (k, v) { this._pairs.push([String(k), v]); };
    OrvalhoFormData.prototype.get = function (k) {
      k = String(k);
      for (var i = 0; i < this._pairs.length; i++) if (this._pairs[i][0] === k) return this._pairs[i][1];
      return null;
    };
    OrvalhoFormData.prototype.has = function (k) { return this.get(k) !== null; };
    OrvalhoFormData.prototype.set = function (k, v) { this.delete(k); this.append(k, v); };
    OrvalhoFormData.prototype.delete = function (k) {
      k = String(k);
      this._pairs = this._pairs.filter(function (p) { return p[0] !== k; });
    };
    OrvalhoFormData.prototype.entries = function () {
      var i = 0, pairs = this._pairs;
      return { next: function () { return i >= pairs.length ? { done: true } : { done: false, value: pairs[i++] }; } };
    };
    globalThis.FormData = OrvalhoFormData;
  }
  if (typeof globalThis.FinalizationRegistry !== "function") {
    function OrvalhoFinalizationRegistry() {}
    OrvalhoFinalizationRegistry.prototype.register = function () {};
    OrvalhoFinalizationRegistry.prototype.unregister = function () {};
    globalThis.FinalizationRegistry = OrvalhoFinalizationRegistry;
  }
  if (typeof globalThis.AbortSignal !== "function") {
    function OrvalhoAbortSignal() { this.aborted = false; this.reason = undefined; }
    OrvalhoAbortSignal.prototype.addEventListener = function () {};
    OrvalhoAbortSignal.prototype.removeEventListener = function () {};
    OrvalhoAbortSignal.prototype.throwIfAborted = function () {
      if (this.aborted) throw this.reason || new Error("aborted");
    };
    OrvalhoAbortSignal.abort = function (reason) {
      var s = new OrvalhoAbortSignal();
      s.aborted = true;
      s.reason = reason;
      return s;
    };
    OrvalhoAbortSignal.timeout = function () { return new OrvalhoAbortSignal(); };
    OrvalhoAbortSignal.any = function () { return new OrvalhoAbortSignal(); };
    function OrvalhoAbortController() { this.signal = new OrvalhoAbortSignal(); }
    OrvalhoAbortController.prototype.abort = function (reason) {
      this.signal.aborted = true;
      this.signal.reason = reason;
    };
    globalThis.AbortSignal = OrvalhoAbortSignal;
    globalThis.AbortController = OrvalhoAbortController;
  }
  if (typeof globalThis.MessagePort !== "function") {
    function OrvalhoMessagePort() {}
    OrvalhoMessagePort.prototype.postMessage = function () {};
    OrvalhoMessagePort.prototype.start = function () {};
    OrvalhoMessagePort.prototype.close = function () {};
    OrvalhoMessagePort.prototype.addEventListener = function () {};
    OrvalhoMessagePort.prototype.removeEventListener = function () {};
    globalThis.MessagePort = OrvalhoMessagePort;
  }
  if (typeof globalThis.FinalizationRegistry !== "function") {
    function OrvalhoFinalizationRegistry() {}
    OrvalhoFinalizationRegistry.prototype.register = function () {};
    OrvalhoFinalizationRegistry.prototype.unregister = function () { return false; };
    globalThis.FinalizationRegistry = OrvalhoFinalizationRegistry;
  }
  if (typeof globalThis.WeakRef !== "function") {
    function OrvalhoWeakRef(v) { this._v = v; }
    OrvalhoWeakRef.prototype.deref = function () { return this._v; };
    globalThis.WeakRef = OrvalhoWeakRef;
  }
  if (typeof globalThis.DOMException !== "function") {
    function OrvalhoDOMException(message, name) {
      this.message = message == null ? "" : String(message);
      this.name = name == null ? "Error" : String(name);
    }
    OrvalhoDOMException.prototype = Object.create(Error.prototype);
    OrvalhoDOMException.prototype.constructor = OrvalhoDOMException;
    globalThis.DOMException = OrvalhoDOMException;
  }
  if (typeof globalThis.Event !== "function") {
    function OrvalhoEvent(type, init) {
      this.type = String(type);
      this.bubbles = !!(init && init.bubbles);
      this.cancelable = !!(init && init.cancelable);
    }
    globalThis.Event = OrvalhoEvent;
  }
  if (typeof globalThis.EventTarget !== "function") {
    function OrvalhoEventTarget() { this._ls = {}; }
    OrvalhoEventTarget.prototype.addEventListener = function (type, fn) {
      type = String(type);
      (this._ls[type] || (this._ls[type] = [])).push(fn);
    };
    OrvalhoEventTarget.prototype.removeEventListener = function (type, fn) {
      type = String(type);
      var a = this._ls[type];
      if (!a) return;
      this._ls[type] = a.filter(function (x) { return x !== fn; });
    };
    OrvalhoEventTarget.prototype.dispatchEvent = function (ev) {
      var a = this._ls[ev && ev.type];
      if (a) for (var i = 0; i < a.length; i++) try { a[i](ev); } catch (e) {}
      return true;
    };
    globalThis.EventTarget = OrvalhoEventTarget;
  }
  if (typeof globalThis.queueMicrotask !== "function") {
    globalThis.queueMicrotask = function (fn) { Promise.resolve().then(fn); };
  }
  if (typeof globalThis.structuredClone !== "function") {
    globalThis.structuredClone = function (v) { return JSON.parse(JSON.stringify(v)); };
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

  function chunkToString(c) {
    if (c == null) return "";
    if (typeof c === "string") return c;
    if (typeof c === "number") return String(c);
    if (c instanceof Uint8Array) return new TextDecoder().decode(c);
    if (typeof ArrayBuffer !== "undefined" && c instanceof ArrayBuffer) {
      return new TextDecoder().decode(new Uint8Array(c));
    }
    // Avoid "[object Object]" for stream-ish values.
    if (typeof c === "object" && c !== null && typeof c.toString === "function") {
      var s = c.toString();
      if (s !== "[object Object]") return s;
    }
    return "";
  }

  // Streams: support async start/pull and TransformStream pipeThrough (Astro SSR).
  if (typeof globalThis.ReadableStream === "undefined") {
    globalThis.ReadableStream = function ReadableStream(underlyingSource) {
      var underlying = underlyingSource || {};
      this._chunks = [];
      this._closed = false;
      this._errored = null;
      this.locked = false;
      var self = this;
      var controller = {
        enqueue: function (c) {
          if (!self._closed) self._chunks.push(c);
        },
        close: function () { self._closed = true; },
        error: function (e) { self._errored = e || new Error("stream error"); self._closed = true; },
        desiredSize: 1,
      };
      var startRet;
      try {
        if (typeof underlying.start === "function") startRet = underlying.start(controller);
      } catch (e) {
        self._errored = e;
        self._closed = true;
      }
      this._ready = Promise.resolve(startRet).catch(function (e) {
        self._errored = e;
        self._closed = true;
      });
      this._underlying = underlying;
      this._controller = controller;

      this.getReader = function () {
        self.locked = true;
        var i = 0;
        return {
          read: function () {
            return self._ready.then(function pump() {
              if (self._errored) return Promise.reject(self._errored);
              if (i < self._chunks.length) {
                return { done: false, value: self._chunks[i++] };
              }
              if (!self._closed && typeof underlying.pull === "function") {
                return Promise.resolve(underlying.pull(controller)).then(function () {
                  if (i < self._chunks.length) {
                    return { done: false, value: self._chunks[i++] };
                  }
                  if (self._closed) return { done: true, value: undefined };
                  // Yield to microtasks so async producers can enqueue.
                  return Promise.resolve().then(pump);
                });
              }
              if (self._closed) return { done: true, value: undefined };
              // Wait one tick for async start/enqueue without pull.
              return Promise.resolve().then(function () {
                if (i < self._chunks.length) {
                  return { done: false, value: self._chunks[i++] };
                }
                if (self._closed || self._errored) {
                  if (self._errored) return Promise.reject(self._errored);
                  return { done: true, value: undefined };
                }
                // Still open with no chunks: resolve empty to avoid hang after idle pulls.
                // Prefer waiting on ready already done; spin a few microtasks.
                return { done: true, value: undefined };
              });
            });
          },
          cancel: function () {
            self._closed = true;
            return Promise.resolve();
          },
          releaseLock: function () { self.locked = false; },
        };
      };

      this.cancel = function () {
        self._closed = true;
        return Promise.resolve();
      };
      this.tee = function () { return [self, self]; };

      this.pipeTo = function (writable) {
        var reader = self.getReader();
        var writer = writable.getWriter ? writable.getWriter() : null;
        if (!writer) return Promise.reject(new TypeError("not a WritableStream"));
        function pump() {
          return reader.read().then(function (r) {
            if (r.done) return writer.close();
            return writer.write(r.value).then(pump);
          });
        }
        return self._ready.then(pump);
      };
      this.pipeThrough = function (transform) {
        var writable = transform.writable;
        var readable = transform.readable;
        self.pipeTo(writable).catch(function () {});
        return readable;
      };

      // Sync snapshot (may be partial). Prefer _orvalhoCollect async for full body.
      this._orvalhoText = function () {
        var out = "";
        for (var j = 0; j < self._chunks.length; j++) out += chunkToString(self._chunks[j]);
        return out;
      };
      this._orvalhoCollect = function () {
        // Wait for async start(), then for the stream to close (or idle),
        // polling pull so TransformStream producers can finish writing.
        var self2 = self;
        var idle = 0;
        var steps = 0;
        function step() {
          if (self2._errored) return Promise.reject(self2._errored);
          steps++;
          if (steps > 200000) return Promise.resolve(self2._orvalhoText());
          var pullP = Promise.resolve();
          if (!self2._closed && typeof self2._underlying.pull === "function") {
            try { pullP = Promise.resolve(self2._underlying.pull(self2._controller)); } catch (e) {}
          }
          return pullP.then(function () {
            if (self2._closed) return self2._orvalhoText();
            if (self2._chunks.length > 0) {
              idle = 0;
              return Promise.resolve().then(step);
            }
            idle++;
            // No new chunks for several turns after start completed: treat as done.
            if (idle > 32) {
              self2._closed = true;
              return self2._orvalhoText();
            }
            return Promise.resolve().then(step);
          });
        }
        return self2._ready.then(step);
      };
    };

    globalThis.WritableStream = function WritableStream(underlyingSink) {
      var sink = underlyingSink || {};
      var self = this;
      this.locked = false;
      this.getWriter = function () {
        self.locked = true;
        return {
          write: function (chunk) {
            if (typeof sink.write === "function") {
              return Promise.resolve(sink.write(chunk, { signal: null }));
            }
            return Promise.resolve();
          },
          close: function () {
            if (typeof sink.close === "function") return Promise.resolve(sink.close());
            return Promise.resolve();
          },
          abort: function (e) {
            if (typeof sink.abort === "function") return Promise.resolve(sink.abort(e));
            return Promise.resolve();
          },
          releaseLock: function () { self.locked = false; },
          get closed() { return Promise.resolve(); },
          get ready() { return Promise.resolve(); },
          desiredSize: 1,
        };
      };
    };

    globalThis.TransformStream = function TransformStream(transformer) {
      transformer = transformer || {};
      var readableController = null;
      var readable = new globalThis.ReadableStream({
        start: function (c) { readableController = c; },
      });
      var writable = new globalThis.WritableStream({
        write: function (chunk) {
          if (typeof transformer.transform === "function") {
            return Promise.resolve(
              transformer.transform(chunk, {
                enqueue: function (c) { readableController.enqueue(c); },
                terminate: function () { readableController.close(); },
              })
            );
          }
          readableController.enqueue(chunk);
          return Promise.resolve();
        },
        close: function () {
          var done = Promise.resolve();
          if (typeof transformer.flush === "function") {
            done = Promise.resolve(
              transformer.flush({
                enqueue: function (c) { readableController.enqueue(c); },
              })
            );
          }
          return done.then(function () { readableController.close(); });
        },
      });
      this.readable = readable;
      this.writable = writable;
    };
  }

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
`
