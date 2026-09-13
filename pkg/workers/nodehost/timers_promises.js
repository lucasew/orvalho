'use strict';

function delay(ms, value) {
  return new Promise(function (resolve) {
    setTimeout(function () { resolve(value); }, ms || 0);
  });
}

function setImmediateP(value) {
  return delay(0, value);
}

function setIntervalP() {
  return {
    next: function () {
      return Promise.resolve({ done: true, value: undefined });
    }
  };
}

var mod = {
  setTimeout: delay,
  setImmediate: setImmediateP,
  setInterval: setIntervalP,
  scheduler: {
    wait: delay,
    yield: function () { return delay(0); }
  }
};
mod.default = mod;
module.exports = mod;
