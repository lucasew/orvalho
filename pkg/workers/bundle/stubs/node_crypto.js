export var webcrypto = globalThis.crypto;
export function randomBytes(n) {
  var a = new Uint8Array(n);
  if (globalThis.crypto && globalThis.crypto.getRandomValues) {
    globalThis.crypto.getRandomValues(a);
  }
  return a;
}
export function createHash() {
  return {
    update: function () {
      return this;
    },
    digest: function () {
      return "";
    },
  };
}
export default { webcrypto: webcrypto, randomBytes: randomBytes, createHash: createHash };
