'use strict';

function setImmediate(fn) {
  var args = [];
  for (var i = 1; i < arguments.length; i++) args.push(arguments[i]);
  return setTimeout(function () { fn.apply(null, args); }, 0);
}

function clearImmediate(id) {
  return clearTimeout(id);
}

var mod = {
  setTimeout: setTimeout,
  clearTimeout: clearTimeout,
  setInterval: setInterval,
  clearInterval: clearInterval,
  setImmediate: setImmediate,
  clearImmediate: clearImmediate
};
mod.promises = require('node:timers/promises');
mod.default = mod;
module.exports = mod;
