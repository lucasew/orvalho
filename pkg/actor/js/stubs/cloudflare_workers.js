// Minimal cloudflare:workers for orvalho. Live env is re-bound per request
// via globalThis.__orvalhoCFEnv (set by host before default.fetch).
function liveEnv() {
  return globalThis.__orvalhoCFEnv || {};
}
// Proxy so property access always hits the current request env.
export const env = new Proxy(
  {},
  {
    get(_t, prop) {
      var e = liveEnv();
      var v = e[prop];
      return typeof v === "undefined" ? e[prop] : v;
    },
    has(_t, prop) {
      return prop in liveEnv();
    },
    ownKeys() {
      return Reflect.ownKeys(liveEnv());
    },
    getOwnPropertyDescriptor(_t, prop) {
      var e = liveEnv();
      if (!(prop in e)) return undefined;
      return { configurable: true, enumerable: true, value: e[prop], writable: false };
    },
  }
);
export default { env: env };
