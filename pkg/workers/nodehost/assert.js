'use strict';

function AssertionError(opts) {
  var msg = (opts && opts.message) || 'assertion failed';
  var err = new Error(msg);
  err.name = 'AssertionError';
  err.actual = opts && opts.actual;
  err.expected = opts && opts.expected;
  err.operator = opts && opts.operator;
  return err;
}

function inspect(v) {
  if (typeof v === 'string') {
    return JSON.stringify(v);
  }
  try {
    return String(v);
  } catch (e) {
    return Object.prototype.toString.call(v);
  }
}

function fail(actual, expected, message, operator) {
  throw AssertionError({
    actual: actual,
    expected: expected,
    message: message || inspect(actual) + ' ' + (operator || '!==') + ' ' + inspect(expected),
    operator: operator || '!==',
  });
}

function strictEqual(actual, expected, message) {
  if (!Object.is(actual, expected)) {
    fail(actual, expected, message, 'strictEqual');
  }
}

function notStrictEqual(actual, expected, message) {
  if (Object.is(actual, expected)) {
    fail(actual, expected, message, 'notStrictEqual');
  }
}

function ok(value, message) {
  if (!value) {
    fail(value, true, message, '==');
  }
}

function isDeepEqual(a, b) {
  if (Object.is(a, b)) {
    return true;
  }
  if (typeof a !== 'object' || typeof b !== 'object' || a === null || b === null) {
    return false;
  }
  if (Object.getPrototypeOf(a) !== Object.getPrototypeOf(b)) {
    return false;
  }
  var ka = Object.keys(a);
  var kb = Object.keys(b);
  if (ka.length !== kb.length) {
    return false;
  }
  ka.sort();
  kb.sort();
  var i;
  for (i = 0; i < ka.length; i++) {
    if (ka[i] !== kb[i]) {
      return false;
    }
    if (!isDeepEqual(a[ka[i]], b[kb[i]])) {
      return false;
    }
  }
  return true;
}

function deepStrictEqual(actual, expected, message) {
  if (!isDeepEqual(actual, expected)) {
    fail(actual, expected, message, 'deepStrictEqual');
  }
}

function throws(fn, expected) {
  var err;
  try {
    fn();
  } catch (e) {
    err = e;
  }
  if (!err) {
    throw AssertionError({ message: 'Missing expected exception.' });
  }
  if (!expected) {
    return;
  }
  if (typeof expected === 'function') {
    if (!(err instanceof expected)) {
      throw AssertionError({ message: 'Expected error to be instance of ' + expected.name });
    }
    return;
  }
  if (typeof expected === 'object') {
    if (expected.code && err.code !== expected.code) {
      fail(err.code, expected.code, 'error code mismatch', 'throws');
    }
    if (expected.name && err.name !== expected.name) {
      fail(err.name, expected.name, 'error name mismatch', 'throws');
    }
    if (expected.message && err.message !== expected.message) {
      fail(err.message, expected.message, 'error message mismatch', 'throws');
    }
  }
}

var assert = ok;
assert.ok = ok;
assert.fail = function (msg) {
  throw AssertionError({ message: msg || 'Failed' });
};
assert.strictEqual = strictEqual;
assert.notStrictEqual = notStrictEqual;
assert.deepStrictEqual = deepStrictEqual;
assert.throws = throws;
assert.AssertionError = AssertionError;
assert.ifError = function (err) {
  if (err) {
    throw err;
  }
};

module.exports = assert;
