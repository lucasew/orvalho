'use strict';

var toString = Object.prototype.toString;

function tagOf(value) {
  try {
    return toString.call(value);
  } catch (e) {
    return '';
  }
}

function isObjectLike(value) {
  var t = typeof value;
  return value !== null && (t === 'object' || t === 'function');
}

function protoGetter(proto, name) {
  try {
    var desc = Object.getOwnPropertyDescriptor(proto, name);
    if (desc && typeof desc.get === 'function') {
      return desc.get;
    }
  } catch (e) {
    return undefined;
  }
  return undefined;
}

function hasBrand(fn, value) {
  try {
    fn(value);
    return true;
  } catch (e) {
    return false;
  }
}

function isDate(value) {
  return hasBrand(function (v) { Date.prototype.getTime.call(v); }, value);
}

function isRegExp(value) {
  return hasBrand(function (v) { RegExp.prototype.exec.call(v, ''); }, value);
}

function isNativeError(value) {
  if (!isObjectLike(value)) {
    return false;
  }
  var tag = tagOf(value);
  return tag === '[object Error]' ||
    tag === '[object DOMException]' ||
    tag === '[object AggregateError]';
}

var arrayBufferByteLength = typeof ArrayBuffer === 'function'
  ? protoGetter(ArrayBuffer.prototype, 'byteLength')
  : undefined;

function isArrayBuffer(value) {
  if (arrayBufferByteLength) {
    return hasBrand(function (v) { arrayBufferByteLength.call(v); }, value);
  }
  return isObjectLike(value) && tagOf(value) === '[object ArrayBuffer]';
}

var sharedArrayBufferByteLength = typeof SharedArrayBuffer === 'function'
  ? protoGetter(SharedArrayBuffer.prototype, 'byteLength')
  : undefined;

function isSharedArrayBuffer(value) {
  if (sharedArrayBufferByteLength) {
    return hasBrand(function (v) { sharedArrayBufferByteLength.call(v); }, value);
  }
  return isObjectLike(value) && tagOf(value) === '[object SharedArrayBuffer]';
}

function isAnyArrayBuffer(value) {
  return isArrayBuffer(value) || isSharedArrayBuffer(value);
}

var dataViewByteLength = typeof DataView === 'function'
  ? protoGetter(DataView.prototype, 'byteLength')
  : undefined;

function isDataView(value) {
  if (dataViewByteLength) {
    return hasBrand(function (v) { dataViewByteLength.call(v); }, value);
  }
  return isObjectLike(value) && tagOf(value) === '[object DataView]';
}

var getTypedArrayTag = (function () {
  try {
    if (typeof Uint8Array !== 'function') {
      return undefined;
    }
    var proto = Object.getPrototypeOf(Uint8Array.prototype);
    var desc = Object.getOwnPropertyDescriptor(proto, Symbol.toStringTag);
    if (desc && typeof desc.get === 'function') {
      return desc.get;
    }
  } catch (e) {
    return undefined;
  }
  return undefined;
})();

function typedArrayKind(value) {
  if (getTypedArrayTag) {
    try {
      var kind = getTypedArrayTag.call(value);
      return kind ? kind : '';
    } catch (e) {
      return '';
    }
  }
  if (typeof ArrayBuffer === 'function' &&
      typeof ArrayBuffer.isView === 'function' &&
      ArrayBuffer.isView(value) &&
      !isDataView(value)) {
    var tag = tagOf(value);
    if (tag.indexOf('[object ') === 0 && tag.charAt(tag.length - 1) === ']') {
      return tag.slice(8, -1);
    }
  }
  return '';
}

function isTypedArray(value) {
  return typedArrayKind(value) !== '';
}

function isArrayBufferView(value) {
  if (typeof ArrayBuffer === 'function' && typeof ArrayBuffer.isView === 'function') {
    return ArrayBuffer.isView(value);
  }
  return isTypedArray(value) || isDataView(value);
}

function isUint8Array(value) { return typedArrayKind(value) === 'Uint8Array'; }
function isUint8ClampedArray(value) { return typedArrayKind(value) === 'Uint8ClampedArray'; }
function isUint16Array(value) { return typedArrayKind(value) === 'Uint16Array'; }
function isUint32Array(value) { return typedArrayKind(value) === 'Uint32Array'; }
function isInt8Array(value) { return typedArrayKind(value) === 'Int8Array'; }
function isInt16Array(value) { return typedArrayKind(value) === 'Int16Array'; }
function isInt32Array(value) { return typedArrayKind(value) === 'Int32Array'; }
function isFloat16Array(value) { return typedArrayKind(value) === 'Float16Array'; }
function isFloat32Array(value) { return typedArrayKind(value) === 'Float32Array'; }
function isFloat64Array(value) { return typedArrayKind(value) === 'Float64Array'; }
function isBigInt64Array(value) { return typedArrayKind(value) === 'BigInt64Array'; }
function isBigUint64Array(value) { return typedArrayKind(value) === 'BigUint64Array'; }

function isMap(value) {
  return typeof Map === 'function' &&
    hasBrand(function (v) { Map.prototype.has.call(v); }, value);
}

function isSet(value) {
  return typeof Set === 'function' &&
    hasBrand(function (v) { Set.prototype.has.call(v); }, value);
}

function isWeakMap(value) {
  return typeof WeakMap === 'function' &&
    hasBrand(function (v) { WeakMap.prototype.has.call(v, {}); }, value);
}

function isWeakSet(value) {
  return typeof WeakSet === 'function' &&
    hasBrand(function (v) { WeakSet.prototype.has.call(v, {}); }, value);
}

function isPromise(value) {
  return isObjectLike(value) && tagOf(value) === '[object Promise]';
}

function isProxy(value) {
  if (!isObjectLike(value)) {
    return false;
  }
  try {
    Object.getPrototypeOf(value);
    return false;
  } catch (e) {
    return true;
  }
}

function isAsyncFunction(value) {
  return typeof value === 'function' && tagOf(value) === '[object AsyncFunction]';
}

function isGeneratorFunction(value) {
  return typeof value === 'function' && tagOf(value) === '[object GeneratorFunction]';
}

function isGeneratorObject(value) {
  return isObjectLike(value) && tagOf(value) === '[object Generator]';
}

function isMapIterator(value) {
  return isObjectLike(value) && tagOf(value) === '[object Map Iterator]';
}

function isSetIterator(value) {
  return isObjectLike(value) && tagOf(value) === '[object Set Iterator]';
}

function isArgumentsObject(value) {
  return isObjectLike(value) && tagOf(value) === '[object Arguments]';
}

function isBooleanObject(value) {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  return hasBrand(function (v) { Boolean.prototype.valueOf.call(v); }, value);
}

function isNumberObject(value) {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  return hasBrand(function (v) { Number.prototype.valueOf.call(v); }, value);
}

function isStringObject(value) {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  return hasBrand(function (v) { String.prototype.valueOf.call(v); }, value);
}

function isSymbolObject(value) {
  if (typeof value !== 'object' || value === null || typeof Symbol !== 'function') {
    return false;
  }
  return hasBrand(function (v) { Symbol.prototype.valueOf.call(v); }, value);
}

function isBigIntObject(value) {
  if (typeof value !== 'object' || value === null || typeof BigInt !== 'function') {
    return false;
  }
  return hasBrand(function (v) { BigInt.prototype.valueOf.call(v); }, value);
}

function isBoxedPrimitive(value) {
  return isBooleanObject(value) ||
    isNumberObject(value) ||
    isStringObject(value) ||
    isSymbolObject(value) ||
    isBigIntObject(value);
}

module.exports = {
  isDate: isDate,
  isRegExp: isRegExp,
  isNativeError: isNativeError,
  isArrayBuffer: isArrayBuffer,
  isSharedArrayBuffer: isSharedArrayBuffer,
  isAnyArrayBuffer: isAnyArrayBuffer,
  isArrayBufferView: isArrayBufferView,
  isDataView: isDataView,
  isTypedArray: isTypedArray,
  isUint8Array: isUint8Array,
  isUint8ClampedArray: isUint8ClampedArray,
  isUint16Array: isUint16Array,
  isUint32Array: isUint32Array,
  isInt8Array: isInt8Array,
  isInt16Array: isInt16Array,
  isInt32Array: isInt32Array,
  isFloat16Array: isFloat16Array,
  isFloat32Array: isFloat32Array,
  isFloat64Array: isFloat64Array,
  isBigInt64Array: isBigInt64Array,
  isBigUint64Array: isBigUint64Array,
  isMap: isMap,
  isSet: isSet,
  isWeakMap: isWeakMap,
  isWeakSet: isWeakSet,
  isPromise: isPromise,
  isProxy: isProxy,
  isAsyncFunction: isAsyncFunction,
  isGeneratorFunction: isGeneratorFunction,
  isGeneratorObject: isGeneratorObject,
  isMapIterator: isMapIterator,
  isSetIterator: isSetIterator,
  isArgumentsObject: isArgumentsObject,
  isBooleanObject: isBooleanObject,
  isNumberObject: isNumberObject,
  isStringObject: isStringObject,
  isSymbolObject: isSymbolObject,
  isBigIntObject: isBigIntObject,
  isBoxedPrimitive: isBoxedPrimitive,
};
