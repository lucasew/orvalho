'use strict';

function AsyncLocalStorage() {
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
AsyncLocalStorage.prototype.exit = function (fn) {
  var prev = this._s;
  this._s = undefined;
  try {
    return fn.apply(null, Array.prototype.slice.call(arguments, 1));
  } finally {
    this._s = prev;
  }
};

function AsyncResource(type) {
  this.type = type;
}
AsyncResource.prototype.runInAsyncScope = function (fn, thisArg) {
  var args = Array.prototype.slice.call(arguments, 2);
  return fn.apply(thisArg, args);
};
AsyncResource.prototype.emitDestroy = function () {
  return this;
};
AsyncResource.prototype.asyncId = function () {
  return 0;
};
AsyncResource.prototype.triggerAsyncId = function () {
  return 0;
};
AsyncResource.bind = function (fn) {
  return fn;
};

function createHook() {
  return {
    enable: function () { return this; },
    disable: function () { return this; },
  };
}

function executionAsyncId() {
  return 0;
}
function triggerAsyncId() {
  return 0;
}
function executionAsyncResource() {
  return {};
}

module.exports = {
  AsyncLocalStorage: AsyncLocalStorage,
  AsyncResource: AsyncResource,
  createHook: createHook,
  executionAsyncId: executionAsyncId,
  triggerAsyncId: triggerAsyncId,
  executionAsyncResource: executionAsyncResource,
};
module.exports.default = module.exports;
