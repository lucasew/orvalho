'use strict';

function EventEmitter() {
  this._events = Object.create(null);
  this._maxListeners = EventEmitter.defaultMaxListeners;
}

EventEmitter.defaultMaxListeners = 10;
EventEmitter.EventEmitter = EventEmitter;
EventEmitter.once = once;
EventEmitter.on = on;
EventEmitter.listenerCount = function (ee, type) {
  return ee.listenerCount(type);
};
EventEmitter.setMaxListeners = function (n) {
  EventEmitter.defaultMaxListeners = n;
  return EventEmitter;
};
EventEmitter.getEventListeners = function (ee, type) {
  return ee.listeners(type);
};

function list(ee, type, create) {
  var ev = ee._events;
  if (!ev) {
    if (!create) return null;
    ev = ee._events = Object.create(null);
  }
  var arr = ev[type];
  if (!arr && create) {
    arr = ev[type] = [];
  }
  return arr || null;
}

EventEmitter.prototype.on = EventEmitter.prototype.addListener = function (type, fn) {
  if (typeof fn !== 'function') {
    var err = new TypeError('The "listener" argument must be of type Function');
    err.code = 'ERR_INVALID_ARG_TYPE';
    throw err;
  }
  list(this, type, true).push(fn);
  return this;
};

EventEmitter.prototype.prependListener = function (type, fn) {
  if (typeof fn !== 'function') {
    var err = new TypeError('The "listener" argument must be of type Function');
    err.code = 'ERR_INVALID_ARG_TYPE';
    throw err;
  }
  list(this, type, true).unshift(fn);
  return this;
};

EventEmitter.prototype.once = function (type, fn) {
  var self = this;
  function wrap() {
    self.removeListener(type, wrap);
    return fn.apply(this, arguments);
  }
  wrap.listener = fn;
  return this.on(type, wrap);
};

EventEmitter.prototype.prependOnceListener = function (type, fn) {
  var self = this;
  function wrap() {
    self.removeListener(type, wrap);
    return fn.apply(this, arguments);
  }
  wrap.listener = fn;
  return this.prependListener(type, wrap);
};

EventEmitter.prototype.off = EventEmitter.prototype.removeListener = function (type, fn) {
  var arr = list(this, type, false);
  if (!arr) return this;
  for (var i = 0; i < arr.length; i++) {
    if (arr[i] === fn || arr[i].listener === fn) {
      arr.splice(i, 1);
      break;
    }
  }
  if (arr.length === 0) delete this._events[type];
  return this;
};

EventEmitter.prototype.removeAllListeners = function (type) {
  if (type === undefined) this._events = Object.create(null);
  else if (this._events) delete this._events[type];
  return this;
};

EventEmitter.prototype.emit = function (type) {
  var arr = list(this, type, false);
  if (!arr || arr.length === 0) {
    if (type === 'error') {
      var er = arguments[1];
      if (er instanceof Error) throw er;
      var e = new Error('Unhandled error.');
      throw e;
    }
    return false;
  }
  var args = [];
  for (var i = 1; i < arguments.length; i++) args.push(arguments[i]);
  arr = arr.slice();
  for (var j = 0; j < arr.length; j++) arr[j].apply(this, args);
  return true;
};

EventEmitter.prototype.listeners = function (type) {
  var arr = list(this, type, false);
  if (!arr) return [];
  var out = [];
  for (var i = 0; i < arr.length; i++) out.push(arr[i].listener || arr[i]);
  return out;
};

EventEmitter.prototype.rawListeners = function (type) {
  var arr = list(this, type, false);
  return arr ? arr.slice() : [];
};

EventEmitter.prototype.listenerCount = function (type) {
  var arr = list(this, type, false);
  return arr ? arr.length : 0;
};

EventEmitter.prototype.eventNames = function () {
  if (!this._events) return [];
  return Object.getOwnPropertyNames(this._events).concat(Object.getOwnPropertySymbols(this._events));
};

EventEmitter.prototype.setMaxListeners = function (n) {
  this._maxListeners = n;
  return this;
};

EventEmitter.prototype.getMaxListeners = function () {
  return this._maxListeners == null ? EventEmitter.defaultMaxListeners : this._maxListeners;
};

function once(emitter, name) {
  return new Promise(function (resolve, reject) {
    function on() {
      cleanup();
      if (arguments.length === 1) resolve(arguments[0]);
      else {
        var a = [];
        for (var i = 0; i < arguments.length; i++) a.push(arguments[i]);
        resolve(a);
      }
    }
    function onErr(err) {
      cleanup();
      reject(err);
    }
    function cleanup() {
      emitter.removeListener(name, on);
      emitter.removeListener('error', onErr);
    }
    emitter.on(name, on);
    if (name !== 'error') emitter.on('error', onErr);
  });
}

function on() {
  throw new Error('events.on async iterator is not implemented');
}

module.exports = EventEmitter;
module.exports.EventEmitter = EventEmitter;
module.exports.once = once;
module.exports.on = EventEmitter.on;
module.exports.listenerCount = EventEmitter.listenerCount;
module.exports.setMaxListeners = EventEmitter.setMaxListeners;
module.exports.getEventListeners = EventEmitter.getEventListeners;
module.exports.captureRejectionSymbol = Symbol.for('nodejs.rejection');
module.exports.errorMonitor = Symbol.for('events.errorMonitor');
module.exports.default = EventEmitter;
