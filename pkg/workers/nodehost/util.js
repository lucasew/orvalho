'use strict';

function invalidArgType(name, expected, actual) {
  var kind = name.indexOf('.') >= 0 ? 'property' : 'argument';
  var rec = actual === null ? 'null' : actual === undefined ? 'undefined' : typeof actual;
  var err = new TypeError(
    'The "' + name + '" ' + kind + ' must be of type ' + expected + '. Received ' + rec
  );
  err.code = 'ERR_INVALID_ARG_TYPE';
  return err;
}

function inherits(ctor, superCtor) {
  if (ctor === undefined || ctor === null) {
    throw invalidArgType('ctor', 'function', ctor);
  }
  if (superCtor === undefined || superCtor === null) {
    throw invalidArgType('superCtor', 'function', superCtor);
  }
  if (superCtor.prototype === undefined) {
    throw invalidArgType('superCtor.prototype', 'object', superCtor.prototype);
  }
  Object.defineProperty(ctor, 'super_', {
    value: superCtor,
    writable: true,
    configurable: true,
  });
  Object.setPrototypeOf(ctor.prototype, superCtor.prototype);
}

function format() {
  var args = Array.prototype.slice.call(arguments);
  if (args.length === 0) {
    return '';
  }
  return args.map(function (a) {
    if (typeof a === 'string') {
      return a;
    }
    if (typeof a === 'symbol') {
      return String(a);
    }
    try {
      return String(a);
    } catch (e) {
      return Object.prototype.toString.call(a);
    }
  }).join(' ');
}

module.exports = {
  inherits: inherits,
  format: format,
  inspect: {
    defaultOptions: {
      numericSeparator: false,
    },
  },
};
