export function Buffer(arg) {
  if (arg instanceof Uint8Array) return arg;
  if (typeof arg === "number") return new Uint8Array(arg);
  if (typeof arg === "string") return new TextEncoder().encode(arg);
  return new Uint8Array(arg || 0);
}
Buffer.from = function (data) {
  if (typeof data === "string") return new TextEncoder().encode(data);
  return new Uint8Array(data);
};
Buffer.alloc = function (n) {
  return new Uint8Array(n);
};
Buffer.isBuffer = function (b) {
  return b instanceof Uint8Array;
};
Buffer.prototype = Uint8Array.prototype;
export default { Buffer: Buffer };
