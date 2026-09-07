'use strict';

function mustCall(fn) {
  if (typeof fn !== 'function') {
    return function () {};
  }
  return fn;
}

function mustNotCall(msg) {
  return function () {
    throw new Error('function should not have been called' + (msg ? ': ' + msg : ''));
  };
}

function skip(msg) {
  throw new Error('skipped: ' + (msg || ''));
}

function platformTimeout(ms) {
  return ms;
}

module.exports = {
  mustCall: mustCall,
  mustNotCall: mustNotCall,
  skip: skip,
  platformTimeout: platformTimeout,
  isWindows: false,
  isMainThread: true,
};
