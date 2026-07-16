export function AsyncLocalStorage() {
  this._s = undefined;
}
AsyncLocalStorage.prototype.getStore = function () {
  return this._s;
};
AsyncLocalStorage.prototype.run = function (store, fn) {
  var prev = this._s;
  this._s = store;
  try {
    return fn.apply(null, Array.prototype.slice.call(arguments, 2));
  } finally {
    this._s = prev;
  }
};
AsyncLocalStorage.prototype.enterWith = function (store) {
  this._s = store;
};
AsyncLocalStorage.prototype.disable = function () {
  this._s = undefined;
};
export function createHook() {
  return {
    enable: function () {},
    disable: function () {},
  };
}
export function executionAsyncId() {
  return 0;
}
export function triggerAsyncId() {
  return 0;
}
export default {
  AsyncLocalStorage: AsyncLocalStorage,
  createHook: createHook,
  executionAsyncId: executionAsyncId,
  triggerAsyncId: triggerAsyncId,
};
