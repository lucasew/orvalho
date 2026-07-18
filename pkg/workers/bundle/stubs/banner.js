// Injected before esbuild IIFE: workerd-style builtins for Astro/CF bundles.
(function () {
  function getBuiltinModule(name) {
    if (name === "node:process" || name === "process") {
      return {
        env: (globalThis.process && globalThis.process.env) || {},
        version: "v20.19.0",
        versions: { node: "20.19.0" },
        platform: "linux",
        features: {},
        exit: function () {},
        cwd: function () {
          return "/";
        },
        nextTick: function (fn) {
          var args = Array.prototype.slice.call(arguments, 1);
          Promise.resolve().then(function () {
            fn.apply(null, args);
          });
        },
      };
    }
    if (name === "node:async_hooks" || name === "async_hooks") {
      function ALS() {
        this._s = undefined;
      }
      ALS.prototype.getStore = function () {
        return this._s;
      };
      ALS.prototype.run = function (s, fn) {
        var p = this._s;
        this._s = s;
        try {
          return fn();
        } finally {
          this._s = p;
        }
      };
      return { AsyncLocalStorage: ALS };
    }
    return {};
  }
  globalThis.getBuiltinModule = getBuiltinModule;
  if (typeof globalThis.process === "undefined" || !globalThis.process) {
    globalThis.process = {};
  }
  if (!globalThis.process.env) {
    globalThis.process.env = globalThis.__orvalhoProcessEnv || {};
  }
  globalThis.process.getBuiltinModule = getBuiltinModule;
})();
