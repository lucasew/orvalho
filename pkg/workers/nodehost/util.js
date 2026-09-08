'use strict';

if (typeof Error.captureStackTrace !== 'function') {
  Error.captureStackTrace = function (err, ctor) {
    if (!err || (typeof err !== 'object' && typeof err !== 'function')) {
      return;
    }
    var probe = new Error();
    var stack = probe.stack || '';
    if (ctor && ctor.name) {
      var lines = String(stack).split('\n');
      var i;
      for (i = 1; i < lines.length; i++) {
        if (lines[i].indexOf(ctor.name) !== -1) {
          lines.splice(1, i);
          break;
        }
      }
      stack = lines.join('\n');
    }
    try {
      Object.defineProperty(err, 'stack', {
        value: stack,
        writable: true,
        configurable: true,
      });
    } catch (e) {
      err.stack = stack;
    }
  };
}

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

var inspectDefaultOptions = {
  showHidden: false,
  depth: 2,
  colors: false,
  customInspect: true,
  showProxy: false,
  maxArrayLength: 100,
  maxStringLength: 10000,
  breakLength: 80,
  compact: 3,
  sorted: false,
  getters: false,
  numericSeparator: false,
};

var kObjectType = 0;
var kArrayType = 1;
var kArrayExtrasType = 2;

var keyStrRegExp = /^[a-zA-Z_][a-zA-Z_0-9]*$/;
var numberRegExp = /^(0|[1-9][0-9]*)$/;
var builtInNameRe = /^[A-Z][a-zA-Z0-9]+$/;

var builtInObjects = Object.create(null);
(function initBuiltIns() {
  var names = Object.getOwnPropertyNames(globalThis);
  var i;
  for (i = 0; i < names.length; i++) {
    if (builtInNameRe.test(names[i])) {
      builtInObjects[names[i]] = true;
    }
  }
})();

var wellKnownProtos = [];
function addWellKnown(proto, name) {
  if (proto) {
    wellKnownProtos.push({ proto: proto, name: name });
  }
}
addWellKnown(Array.prototype, 'Array');
addWellKnown(Object.prototype, 'Object');
addWellKnown(Function.prototype, 'Function');
addWellKnown(Date.prototype, 'Date');
addWellKnown(RegExp.prototype, 'RegExp');
addWellKnown(Error.prototype, 'Error');
if (typeof ArrayBuffer === 'function') {
  addWellKnown(ArrayBuffer.prototype, 'ArrayBuffer');
}
if (typeof SharedArrayBuffer === 'function') {
  addWellKnown(SharedArrayBuffer.prototype, 'SharedArrayBuffer');
}

function wellKnownName(obj) {
  var i;
  for (i = 0; i < wellKnownProtos.length; i++) {
    if (wellKnownProtos[i].proto === obj) {
      return wellKnownProtos[i].name;
    }
  }
  return undefined;
}

function stylizeWithColor(str, styleType) {
  var style = inspect.styles[styleType];
  if (style !== undefined) {
    var color = inspect.colors[style];
    if (color !== undefined) {
      return '\u001b[' + color[0] + 'm' + str + '\u001b[' + color[1] + 'm';
    }
  }
  return str;
}

function stylizeNoColor(str) {
  return str;
}

function addNumericSeparator(integerString) {
  var result = '';
  var i = integerString.length;
  var start = integerString.charAt(0) === '-' ? 1 : 0;
  for (; i >= start + 4; i -= 3) {
    result = '_' + integerString.slice(i - 3, i) + result;
  }
  return i === integerString.length ? integerString : integerString.slice(0, i) + result;
}

function addNumericSeparatorEnd(integerString) {
  var result = '';
  var i = 0;
  for (; i < integerString.length - 3; i += 3) {
    result += integerString.slice(i, i + 3) + '_';
  }
  return i === 0 ? integerString : result + integerString.slice(i);
}

function formatNumber(fn, number, numericSeparator) {
  if (!numericSeparator) {
    if (Object.is(number, -0)) {
      return fn('-0', 'number');
    }
    return fn('' + number, 'number');
  }
  var numberString = String(number);
  var integer = number < 0 ? Math.ceil(number) : Math.floor(number);
  if (Object.is(number, -0)) {
    integer = -0;
  }
  if (integer === number) {
    if (!isFinite(number) || numberString.indexOf('e') !== -1) {
      return fn(numberString, 'number');
    }
    return fn(addNumericSeparator(numberString), 'number');
  }
  if (number !== number) {
    return fn(numberString, 'number');
  }
  var decimalIndex = numberString.indexOf('.');
  if (decimalIndex === -1) {
    return fn(numberString, 'number');
  }
  return fn(
    addNumericSeparator(numberString.slice(0, decimalIndex)) +
      '.' +
      addNumericSeparatorEnd(numberString.slice(decimalIndex + 1)),
    'number'
  );
}

function formatBigInt(fn, bigint, numericSeparator) {
  var string = String(bigint);
  if (!numericSeparator) {
    return fn(string + 'n', 'bigint');
  }
  return fn(addNumericSeparator(string) + 'n', 'bigint');
}

function strEscape(str) {
  var out = '';
  var i;
  var c;
  var code;
  var hex;
  for (i = 0; i < str.length; i++) {
    c = str.charAt(i);
    code = str.charCodeAt(i);
    if (c === "'" || c === '\\') {
      out += '\\' + c;
    } else if (c === '\n') {
      out += '\\n';
    } else if (c === '\r') {
      out += '\\r';
    } else if (c === '\t') {
      out += '\\t';
    } else if (c === '\b') {
      out += '\\b';
    } else if (c === '\f') {
      out += '\\f';
    } else if (code < 32 || (code > 126 && code < 160)) {
      hex = code.toString(16);
      out += '\\x' + (hex.length < 2 ? '0' + hex : hex);
    } else {
      out += c;
    }
  }
  return "'" + out + "'";
}

function formatPrimitive(fn, value, ctx) {
  var t = typeof value;
  if (t === 'string') {
    var trailer = '';
    if (value.length > ctx.maxStringLength) {
      var remaining = value.length - ctx.maxStringLength;
      value = value.slice(0, ctx.maxStringLength);
      trailer = '... ' + remaining + ' more character' + (remaining > 1 ? 's' : '');
    }
    return fn(strEscape(value), 'string') + trailer;
  }
  if (t === 'number') {
    return formatNumber(fn, value, ctx.numericSeparator);
  }
  if (t === 'bigint') {
    return formatBigInt(fn, value, ctx.numericSeparator);
  }
  if (t === 'boolean') {
    return fn('' + value, 'boolean');
  }
  if (t === 'undefined') {
    return fn('undefined', 'undefined');
  }
  return fn(String(value), 'symbol');
}

function getPrefix(constructor, tag, fallback, size) {
  if (size === undefined) {
    size = '';
  }
  if (constructor === null) {
    if (tag !== '' && fallback !== tag) {
      return '[' + fallback + size + ': null prototype] [' + tag + '] ';
    }
    return '[' + fallback + size + ': null prototype] ';
  }
  var result = constructor + size + ' ';
  if (tag !== '') {
    var position = constructor.indexOf(tag);
    if (position === -1) {
      result += '[' + tag + '] ';
    } else {
      var endPos = position + tag.length;
      if (endPos !== constructor.length && constructor.charAt(endPos) === constructor.charAt(endPos).toLowerCase()) {
        result += '[' + tag + '] ';
      }
    }
  }
  return result;
}

function getConstructorName(value) {
  var tmp = value;
  var obj = value;
  var firstProto;
  var known;
  var desc;
  while (obj) {
    known = wellKnownName(obj);
    if (known !== undefined) {
      try {
        if (known === 'Array' && Array.isArray(tmp)) {
          return 'Array';
        }
        if (known === 'Function' && typeof tmp === 'function') {
          return 'Function';
        }
        if (known === 'Date' && tmp instanceof Date) {
          return 'Date';
        }
        if (known === 'RegExp' && tmp instanceof RegExp) {
          return 'RegExp';
        }
        if (known === 'Error' && tmp instanceof Error) {
          return 'Error';
        }
        if (known === 'ArrayBuffer' && typeof ArrayBuffer === 'function' && tmp instanceof ArrayBuffer) {
          return 'ArrayBuffer';
        }
        if (
          known === 'SharedArrayBuffer' &&
          typeof SharedArrayBuffer === 'function' &&
          tmp instanceof SharedArrayBuffer
        ) {
          return 'SharedArrayBuffer';
        }
        if (known === 'Object' && tmp instanceof Object) {
          return 'Object';
        }
      } catch (e) {
        // ignore cross-realm instanceof
      }
    }
    try {
      desc = Object.getOwnPropertyDescriptor(obj, 'constructor');
    } catch (e2) {
      desc = undefined;
    }
    if (desc !== undefined && typeof desc.value === 'function' && desc.value.name !== '') {
      try {
        if (tmp instanceof desc.value) {
          return String(desc.value.name);
        }
      } catch (e3) {
        // ignore
      }
    }
    obj = Object.getPrototypeOf(obj);
    if (firstProto === undefined) {
      firstProto = obj;
    }
  }
  if (firstProto === null) {
    return null;
  }
  if (Array.isArray(tmp)) {
    return 'Array';
  }
  if (typeof tmp === 'function') {
    return 'Function';
  }
  return 'Object';
}

// goja does not expose V8's constructor name after the prototype is stripped.
// Remember the name when Object.setPrototypeOf(..., null) is used.
var nullProtoNames = typeof WeakMap === 'function' ? new WeakMap() : null;
(function trackNullProtoNames() {
  var orig = Object.setPrototypeOf;
  if (typeof orig !== 'function' || !nullProtoNames) {
    return;
  }
  Object.setPrototypeOf = function (obj, proto) {
    if (proto === null && obj !== null && (typeof obj === 'object' || typeof obj === 'function')) {
      try {
        if (!nullProtoNames.has(obj)) {
          var ctor = obj.constructor;
          if (typeof ctor === 'function' && ctor.name) {
            nullProtoNames.set(obj, String(ctor.name));
          }
        }
      } catch (e) {
        // ignore
      }
    }
    return orig.apply(Object, arguments);
  };
})();

function internalGetConstructorName(value) {
  if (nullProtoNames) {
    try {
      var remembered = nullProtoNames.get(value);
      if (remembered) {
        return remembered;
      }
    } catch (e) {
      // ignore
    }
  }
  if (Array.isArray(value)) {
    return 'Array';
  }
  if (typeof value === 'function') {
    return 'Function';
  }
  return 'Object';
}

function getCtxStyle(value, constructor, tag) {
  var fallback = '';
  if (constructor === null) {
    fallback = internalGetConstructorName(value);
    if (fallback === tag) {
      fallback = 'Object';
    }
  }
  return getPrefix(constructor, tag, fallback);
}

function getKeys(value, showHidden) {
  var keys;
  var symbols;
  try {
    symbols = Object.getOwnPropertySymbols(value);
  } catch (e) {
    symbols = [];
  }
  if (showHidden) {
    keys = Object.getOwnPropertyNames(value);
  } else {
    try {
      keys = Object.keys(value);
    } catch (e2) {
      keys = Object.getOwnPropertyNames(value);
    }
    symbols = symbols.filter(function (key) {
      return Object.prototype.propertyIsEnumerable.call(value, key);
    });
  }
  if (symbols.length !== 0) {
    keys = keys.concat(symbols);
  }
  return keys;
}

function isIndexName(key, length) {
  if (typeof key !== 'string' || numberRegExp.exec(key) === null) {
    return false;
  }
  var n = +key;
  return n < length && n < 4294967294;
}

function getOwnNonIndexProperties(value, showHidden) {
  var names = showHidden ? Object.getOwnPropertyNames(value) : Object.keys(value);
  var symbols;
  try {
    symbols = Object.getOwnPropertySymbols(value);
  } catch (e) {
    symbols = [];
  }
  var out = [];
  var i;
  var key;
  for (i = 0; i < names.length; i++) {
    key = names[i];
    if (!isIndexName(key, value.length)) {
      out.push(key);
    }
  }
  for (i = 0; i < symbols.length; i++) {
    if (showHidden || Object.prototype.propertyIsEnumerable.call(value, symbols[i])) {
      out.push(symbols[i]);
    }
  }
  return out;
}

function orderFunctionKeys(keys) {
  var preferred = ['length', 'name', 'arguments', 'caller', 'prototype'];
  var seen = Object.create(null);
  var out = [];
  var i;
  var j;
  var key;
  for (i = 0; i < preferred.length; i++) {
    for (j = 0; j < keys.length; j++) {
      if (keys[j] === preferred[i]) {
        out.push(keys[j]);
        seen[preferred[i]] = true;
      }
    }
  }
  for (i = 0; i < keys.length; i++) {
    key = keys[i];
    if (typeof key !== 'string' || !seen[key]) {
      out.push(key);
    }
  }
  return out;
}

function getFunctionBase(ctx, value, constructor, tag) {
  var type = 'Function';
  var base = '[' + type;
  if (constructor === null) {
    base += ' (null prototype)';
  }
  if (value.name === '' || value.name === undefined) {
    base += ' (anonymous)';
  } else {
    base += ': ' + (typeof value.name === 'string' ? value.name : formatValue(ctx, value.name, 0));
  }
  base += ']';
  if (constructor !== type && constructor !== null) {
    base += ' ' + constructor;
  }
  if (tag !== '' && constructor !== tag) {
    base += ' [' + tag + ']';
  }
  return base;
}

function formatError(err) {
  var stack = err.stack;
  if (typeof stack === 'string' && stack !== '') {
    return stack;
  }
  var name = err.name != null ? err.name : 'Error';
  var msg = err.message;
  var base = msg ? name + ': ' + msg : String(name);
  return '[' + base + ']';
}

function hexContents(buf, max) {
  var n = Math.min(buf.length, max);
  var parts = [];
  var i;
  var h;
  for (i = 0; i < n; i++) {
    h = buf[i].toString(16);
    parts.push(h.length === 1 ? '0' + h : h);
  }
  var str = parts.join(' ');
  var remaining = buf.length - n;
  if (remaining > 0) {
    str += ' ... ' + remaining + ' more byte' + (remaining > 1 ? 's' : '');
  }
  return str;
}

function formatArrayBuffer(ctx, value) {
  var buffer;
  try {
    buffer = new Uint8Array(value);
  } catch (e) {
    return [ctx.stylize('(detached)', 'special')];
  }
  return [ctx.stylize('[Uint8Contents]', 'special') + ': <' + hexContents(buffer, ctx.maxArrayLength) + '>'];
}

function formatSpecialArray(ctx, value, recurseTimes, maxLength, output, i) {
  var keys = Object.keys(value);
  var index = i;
  var key;
  var tmp;
  var emptyItems;
  var remaining;
  for (; i < keys.length && output.length < maxLength; i++) {
    key = keys[i];
    tmp = +key;
    if (tmp > 4294967294) {
      break;
    }
    if ('' + index !== key) {
      if (numberRegExp.exec(key) === null) {
        break;
      }
      emptyItems = tmp - index;
      output.push(ctx.stylize('<' + emptyItems + ' empty item' + (emptyItems > 1 ? 's' : '') + '>', 'undefined'));
      index = tmp;
      if (output.length === maxLength) {
        break;
      }
    }
    output.push(formatProperty(ctx, value, recurseTimes, key, kArrayType));
    index++;
  }
  remaining = value.length - index;
  if (output.length !== maxLength) {
    if (remaining > 0) {
      output.push(ctx.stylize('<' + remaining + ' empty item' + (remaining > 1 ? 's' : '') + '>', 'undefined'));
    }
  } else if (remaining > 0) {
    output.push('... ' + remaining + ' more item' + (remaining > 1 ? 's' : ''));
  }
  return output;
}

function formatArray(ctx, value, recurseTimes) {
  var valLen = value.length;
  var len = Math.min(Math.max(0, ctx.maxArrayLength), valLen);
  var remaining = valLen - len;
  var output = [];
  var i;
  var desc;
  for (i = 0; i < len; i++) {
    desc = Object.getOwnPropertyDescriptor(value, i);
    if (desc === undefined) {
      return formatSpecialArray(ctx, value, recurseTimes, len, output, i);
    }
    output.push(formatProperty(ctx, value, recurseTimes, i, kArrayType, desc));
  }
  if (remaining > 0) {
    output.push('... ' + remaining + ' more item' + (remaining > 1 ? 's' : ''));
  }
  return output;
}

function formatProperty(ctx, value, recurseTimes, key, type, desc) {
  var name;
  var str;
  var extra = ' ';
  if (!desc) {
    try {
      desc = Object.getOwnPropertyDescriptor(value, key);
    } catch (e) {
      desc = undefined;
    }
  }
  if (!desc) {
    desc = { value: value[key], enumerable: true };
  }
  if (desc.value !== undefined) {
    var diff = ctx.compact !== true || type !== kObjectType ? 2 : 3;
    ctx.indentationLvl += diff;
    str = formatValue(ctx, desc.value, recurseTimes);
    ctx.indentationLvl -= diff;
  } else if (desc.get !== undefined) {
    str = ctx.stylize(desc.set !== undefined ? '[Getter/Setter]' : '[Getter]', 'special');
  } else if (desc.set !== undefined) {
    str = ctx.stylize('[Setter]', 'special');
  } else {
    str = ctx.stylize('undefined', 'undefined');
  }
  if (type === kArrayType) {
    return str;
  }
  if (typeof key === 'symbol') {
    name = ctx.stylize(String(key), 'symbol');
  } else if (keyStrRegExp.exec(key) !== null) {
    name = key === '__proto__' ? "['__proto__']" : ctx.stylize(key, 'name');
  } else {
    name = ctx.stylize(strEscape(String(key)), 'string');
  }
  if (desc.enumerable === false) {
    name = '[' + name + ']';
  }
  return name + ':' + extra + str;
}

function visibleWidth(str) {
  return String(str).replace(/\u001b\[[0-9;]*m/g, '').length;
}

function isBelowBreakLength(ctx, output, start, base) {
  var totalLength = output.length + start;
  var i;
  if (totalLength + output.length > ctx.breakLength) {
    return false;
  }
  for (i = 0; i < output.length; i++) {
    totalLength += visibleWidth(output[i]);
    if (totalLength > ctx.breakLength) {
      return false;
    }
  }
  return base === '' || base.indexOf('\n') === -1;
}

function reduceToSingleString(ctx, output, base, braces, extrasType, recurseTimes) {
  var indentation;
  var joined;
  var start;
  if (ctx.compact !== true) {
    if (typeof ctx.compact === 'number' && ctx.compact >= 1) {
      if (ctx.currentDepth - recurseTimes < ctx.compact) {
        start = output.length + ctx.indentationLvl + braces[0].length + base.length + 10;
        if (isBelowBreakLength(ctx, output, start, base)) {
          joined = output.join(', ');
          if (joined.indexOf('\n') === -1) {
            return (base ? base + ' ' : '') + braces[0] + ' ' + joined + ' ' + braces[1];
          }
        }
      }
    }
    indentation = '\n' + ' '.repeat(ctx.indentationLvl);
    return (
      (base ? base + ' ' : '') +
      braces[0] +
      indentation +
      '  ' +
      output.join(',' + indentation + '  ') +
      indentation +
      braces[1]
    );
  }
  if (isBelowBreakLength(ctx, output, 0, base)) {
    return braces[0] + (base ? ' ' + base : '') + ' ' + output.join(', ') + ' ' + braces[1];
  }
  indentation = ' '.repeat(ctx.indentationLvl);
  var ln = base === '' && braces[0].length === 1 ? ' ' : (base ? ' ' + base : '') + '\n' + indentation + '  ';
  return braces[0] + ln + output.join(',\n' + indentation + '  ') + ' ' + braces[1];
}

function isAnyArrayBuffer(value) {
  if (typeof ArrayBuffer === 'function' && value instanceof ArrayBuffer) {
    return true;
  }
  if (typeof SharedArrayBuffer === 'function' && value instanceof SharedArrayBuffer) {
    return true;
  }
  return false;
}

function isArrayBufferValue(value) {
  return typeof ArrayBuffer === 'function' && value instanceof ArrayBuffer;
}

function formatValue(ctx, value, recurseTimes) {
  if (typeof value !== 'object' && typeof value !== 'function') {
    return formatPrimitive(ctx.stylize, value, ctx);
  }
  if (value === null) {
    return ctx.stylize('null', 'null');
  }

  if (ctx.seen.indexOf(value) !== -1) {
    var index = 1;
    if (ctx.circular === undefined) {
      ctx.circular = new Map();
      ctx.circular.set(value, index);
    } else {
      index = ctx.circular.get(value);
      if (index === undefined) {
        index = ctx.circular.size + 1;
        ctx.circular.set(value, index);
      }
    }
    return ctx.stylize('[Circular *' + index + ']', 'special');
  }

  return formatRaw(ctx, value, recurseTimes);
}

function formatRaw(ctx, value, recurseTimes) {
  var constructor = getConstructorName(value);
  var tag = '';
  try {
    tag = value[Symbol.toStringTag];
  } catch (e) {
    tag = '';
  }
  if (typeof tag !== 'string') {
    tag = '';
  } else if (
    tag !== '' &&
    (ctx.showHidden ? Object.prototype.hasOwnProperty : Object.prototype.propertyIsEnumerable).call(
      value,
      Symbol.toStringTag
    )
  ) {
    tag = '';
  }

  var keys;
  var braces;
  var formatter = function () {
    return [];
  };
  var extrasType = kObjectType;
  var base = '';
  var prefix;

  if (Array.isArray(value)) {
    prefix = constructor !== 'Array' || tag !== '' ? getPrefix(constructor, tag, 'Array', '(' + value.length + ')') : '';
    keys = getOwnNonIndexProperties(value, ctx.showHidden);
    braces = [prefix + '[', ']'];
    if (value.length === 0 && keys.length === 0) {
      return braces[0] + ']';
    }
    extrasType = kArrayExtrasType;
    formatter = formatArray;
  } else {
    keys = getKeys(value, ctx.showHidden);
    braces = ['{', '}'];
    if (typeof value === 'function') {
      keys = orderFunctionKeys(keys);
      base = getFunctionBase(ctx, value, constructor, tag);
      if (keys.length === 0) {
        return ctx.stylize(base, 'special');
      }
    } else if (constructor === 'Object') {
      if (tag !== '') {
        braces[0] = getPrefix(constructor, tag, 'Object') + '{';
      }
      if (keys.length === 0) {
        return braces[0] + '}';
      }
    } else if (value instanceof RegExp) {
      base = String(value);
      prefix = getPrefix(constructor, tag, 'RegExp');
      if (prefix !== 'RegExp ') {
        base = prefix + base;
      }
      if (keys.length === 0 || (ctx.depth !== null && recurseTimes > ctx.depth)) {
        return ctx.stylize(base, 'regexp');
      }
    } else if (value instanceof Date) {
      base = isNaN(value.getTime()) ? String(value) : value.toISOString();
      prefix = getPrefix(constructor, tag, 'Date');
      if (prefix !== 'Date ') {
        base = prefix + base;
      }
      if (keys.length === 0) {
        return ctx.stylize(base, 'date');
      }
    } else if (value instanceof Error) {
      base = formatError(value);
      if (keys.length === 0) {
        return base;
      }
    } else if (isAnyArrayBuffer(value)) {
      var arrayType = isArrayBufferValue(value) ? 'ArrayBuffer' : 'SharedArrayBuffer';
      prefix = getPrefix(constructor, tag, arrayType);
      formatter = formatArrayBuffer;
      braces[0] = prefix + '{';
      if (keys.indexOf('byteLength') === -1) {
        keys.unshift('byteLength');
      }
    } else {
      if (keys.length === 0) {
        return getCtxStyle(value, constructor, tag) + '{}';
      }
      braces[0] = getCtxStyle(value, constructor, tag) + '{';
    }
  }

  if (ctx.depth !== null && recurseTimes > ctx.depth) {
    var constructorName = getCtxStyle(value, constructor, tag).slice(0, -1);
    if (constructor !== null) {
      constructorName = '[' + constructorName + ']';
    }
    return ctx.stylize(constructorName, 'special');
  }

  recurseTimes += 1;
  ctx.seen.push(value);
  ctx.currentDepth = recurseTimes;
  var output = formatter(ctx, value, recurseTimes);
  var i;
  for (i = 0; i < keys.length; i++) {
    output.push(formatProperty(ctx, value, recurseTimes, keys[i], extrasType));
  }
  if (ctx.circular !== undefined) {
    var ref = ctx.circular.get(value);
    if (ref !== undefined) {
      var reference = ctx.stylize('<ref *' + ref + '>', 'special');
      if (ctx.compact !== true) {
        base = base === '' ? reference : reference + ' ' + base;
      } else {
        braces[0] = reference + ' ' + braces[0];
      }
    }
  }
  ctx.seen.pop();
  return reduceToSingleString(ctx, output, base, braces, extrasType, recurseTimes);
}

function inspect(value, opts) {
  var ctx = {
    budget: {},
    indentationLvl: 0,
    seen: [],
    currentDepth: 0,
    stylize: stylizeNoColor,
    showHidden: inspectDefaultOptions.showHidden,
    depth: inspectDefaultOptions.depth,
    colors: inspectDefaultOptions.colors,
    customInspect: inspectDefaultOptions.customInspect,
    showProxy: inspectDefaultOptions.showProxy,
    maxArrayLength: inspectDefaultOptions.maxArrayLength,
    maxStringLength: inspectDefaultOptions.maxStringLength,
    breakLength: inspectDefaultOptions.breakLength,
    compact: inspectDefaultOptions.compact,
    sorted: inspectDefaultOptions.sorted,
    getters: inspectDefaultOptions.getters,
    numericSeparator: inspectDefaultOptions.numericSeparator,
  };
  if (opts && typeof opts === 'object') {
    var keys = Object.keys(opts);
    var i;
    var key;
    for (i = 0; i < keys.length; i++) {
      key = keys[i];
      if (Object.prototype.hasOwnProperty.call(inspectDefaultOptions, key) || key === 'stylize') {
        ctx[key] = opts[key];
      }
    }
  }
  if (ctx.colors) {
    ctx.stylize = stylizeWithColor;
  }
  if (ctx.maxArrayLength === null) {
    ctx.maxArrayLength = Infinity;
  }
  if (ctx.maxStringLength === null) {
    ctx.maxStringLength = Infinity;
  }
  return formatValue(ctx, value, 0);
}

inspect.defaultOptions = inspectDefaultOptions;
inspect.custom = Symbol.for('nodejs.util.inspect.custom');
inspect.colors = {
  reset: [0, 0],
  bold: [1, 22],
  dim: [2, 22],
  italic: [3, 23],
  underline: [4, 24],
  yellow: [33, 39],
  green: [32, 39],
  cyan: [36, 39],
  magenta: [35, 39],
  red: [31, 39],
  grey: [90, 39],
  gray: [90, 39],
  white: [37, 39],
  blue: [34, 39],
};
inspect.styles = {
  special: 'cyan',
  number: 'yellow',
  bigint: 'yellow',
  boolean: 'yellow',
  undefined: 'grey',
  null: 'bold',
  string: 'green',
  symbol: 'green',
  date: 'magenta',
  regexp: 'red',
  module: 'underline',
};

function returnFalse() {
  return false;
}

function hasBuiltInToString(value) {
  var hasOwnToString = Object.prototype.hasOwnProperty;
  var hasOwnToPrimitive = Object.prototype.hasOwnProperty;
  if (typeof value.toString !== 'function') {
    if (typeof value[Symbol.toPrimitive] !== 'function') {
      return true;
    }
    if (Object.prototype.hasOwnProperty.call(value, Symbol.toPrimitive)) {
      return false;
    }
    hasOwnToString = returnFalse;
  } else if (Object.prototype.hasOwnProperty.call(value, 'toString')) {
    return false;
  } else if (typeof value[Symbol.toPrimitive] !== 'function') {
    hasOwnToPrimitive = returnFalse;
  } else if (Object.prototype.hasOwnProperty.call(value, Symbol.toPrimitive)) {
    return false;
  }
  var pointer = value;
  do {
    pointer = Object.getPrototypeOf(pointer);
    if (pointer === null) {
      return true;
    }
  } while (!hasOwnToString.call(pointer, 'toString') && !hasOwnToPrimitive.call(pointer, Symbol.toPrimitive));
  var descriptor;
  try {
    descriptor = Object.getOwnPropertyDescriptor(pointer, 'constructor');
  } catch (e) {
    return false;
  }
  return (
    descriptor !== undefined &&
    typeof descriptor.value === 'function' &&
    !!builtInObjects[descriptor.value.name]
  );
}

var CIRCULAR_ERROR_MESSAGE;
function firstErrorLine(error) {
  return String(error.message).split('\n')[0];
}

function tryStringify(arg) {
  try {
    return JSON.stringify(arg);
  } catch (err) {
    if (!CIRCULAR_ERROR_MESSAGE) {
      try {
        var a = {};
        a.a = a;
        JSON.stringify(a);
      } catch (circularError) {
        CIRCULAR_ERROR_MESSAGE = firstErrorLine(circularError);
      }
    }
    if (err.name === 'TypeError' && firstErrorLine(err) === CIRCULAR_ERROR_MESSAGE) {
      return '[Circular]';
    }
    throw err;
  }
}

function formatNumberNoColor(number, options) {
  var sep = inspectDefaultOptions.numericSeparator;
  if (options && options.numericSeparator !== undefined) {
    sep = options.numericSeparator;
  }
  return formatNumber(stylizeNoColor, number, sep);
}

function formatBigIntNoColor(bigint, options) {
  var sep = inspectDefaultOptions.numericSeparator;
  if (options && options.numericSeparator !== undefined) {
    sep = options.numericSeparator;
  }
  return formatBigInt(stylizeNoColor, bigint, sep);
}

function parseIntCompat(value) {
  if (typeof value === 'number') {
    if (!isFinite(value)) {
      return NaN;
    }
    // goja's parseInt("-0.5") is +0; Node's Number.parseInt(-0.5) is -0.
    if (Object.is(value, -0) || (value < 0 && value > -1)) {
      return -0;
    }
  }
  return parseInt(value, 10);
}

function assignOpts(base, extra) {
  var out = {};
  var key;
  if (base) {
    for (key in base) {
      if (Object.prototype.hasOwnProperty.call(base, key)) {
        out[key] = base[key];
      }
    }
  }
  if (extra) {
    for (key in extra) {
      if (Object.prototype.hasOwnProperty.call(extra, key)) {
        out[key] = extra[key];
      }
    }
  }
  return out;
}

function formatWithOptionsInternal(inspectOptions, args) {
  var first = args[0];
  var a = 0;
  var str = '';
  var join = '';
  var tempStr;
  var lastPos;
  var i;
  var nextChar;
  var tempArg;
  var tempNum;
  var opts;

  if (typeof first === 'string') {
    if (args.length === 1) {
      return first;
    }
    lastPos = 0;
    for (i = 0; i < first.length - 1; i++) {
      if (first.charCodeAt(i) === 37) {
        nextChar = first.charCodeAt(++i);
        if (a + 1 !== args.length) {
          switch (nextChar) {
            case 115:
              tempArg = args[++a];
              if (typeof tempArg === 'number') {
                tempStr = formatNumberNoColor(tempArg, inspectOptions);
              } else if (typeof tempArg === 'bigint') {
                tempStr = formatBigIntNoColor(tempArg, inspectOptions);
              } else if (typeof tempArg !== 'object' || tempArg === null || !hasBuiltInToString(tempArg)) {
                tempStr = String(tempArg);
              } else {
                tempStr = inspect(
                  tempArg,
                  assignOpts(inspectOptions, { compact: 3, colors: false, depth: 0 })
                );
              }
              break;
            case 106:
              tempStr = tryStringify(args[++a]);
              break;
            case 100:
              tempNum = args[++a];
              if (typeof tempNum === 'bigint') {
                tempStr = formatBigIntNoColor(tempNum, inspectOptions);
              } else if (typeof tempNum === 'symbol') {
                tempStr = 'NaN';
              } else {
                tempStr = formatNumberNoColor(Number(tempNum), inspectOptions);
              }
              break;
            case 79:
              tempStr = inspect(args[++a], inspectOptions);
              break;
            case 111:
              tempStr = inspect(
                args[++a],
                assignOpts(inspectOptions, { showHidden: true, showProxy: true, depth: 4 })
              );
              break;
            case 105:
              tempNum = args[++a];
              if (typeof tempNum === 'bigint') {
                tempStr = formatBigIntNoColor(tempNum, inspectOptions);
              } else if (typeof tempNum === 'symbol') {
                tempStr = 'NaN';
              } else {
                tempStr = formatNumberNoColor(parseIntCompat(tempNum), inspectOptions);
              }
              break;
            case 102:
              tempNum = args[++a];
              if (typeof tempNum === 'symbol') {
                tempStr = 'NaN';
              } else {
                tempStr = formatNumberNoColor(parseFloat(tempNum), inspectOptions);
              }
              break;
            case 99:
              a += 1;
              tempStr = '';
              break;
            case 37:
              str += first.slice(lastPos, i);
              lastPos = i + 1;
              continue;
            default:
              continue;
          }
          if (lastPos !== i - 1) {
            str += first.slice(lastPos, i - 1);
          }
          str += tempStr;
          lastPos = i + 1;
        } else if (nextChar === 37) {
          str += first.slice(lastPos, i);
          lastPos = i + 1;
        }
      }
    }
    if (lastPos !== 0) {
      a++;
      join = ' ';
      if (lastPos < first.length) {
        str += first.slice(lastPos);
      }
    }
  }

  opts = inspectOptions;
  while (a < args.length) {
    tempArg = args[a];
    str += join;
    str += typeof tempArg !== 'string' ? inspect(tempArg, opts) : tempArg;
    join = ' ';
    a++;
  }
  return str;
}

function format() {
  return formatWithOptionsInternal(undefined, Array.prototype.slice.call(arguments));
}

function formatWithOptions(inspectOptions) {
  if (inspectOptions === null || typeof inspectOptions !== 'object') {
    throw invalidArgType('inspectOptions', 'object', inspectOptions);
  }
  return formatWithOptionsInternal(inspectOptions, Array.prototype.slice.call(arguments, 1));
}

var promisifyCustom = Symbol.for('nodejs.util.promisify.custom');

function promisify(original) {
  if (typeof original !== 'function') {
    throw invalidArgType('original', 'Function', original);
  }
  if (original[promisifyCustom]) {
    var custom = original[promisifyCustom];
    if (typeof custom !== 'function') {
      throw invalidArgType('util.promisify.custom', 'Function', custom);
    }
    return custom;
  }
  function fn() {
    var args = [];
    for (var i = 0; i < arguments.length; i++) args.push(arguments[i]);
    var self = this;
    return new Promise(function (resolve, reject) {
      args.push(function (err, value) {
        if (err) reject(err);
        else resolve(value);
      });
      original.apply(self, args);
    });
  }
  Object.setPrototypeOf(fn, Object.getPrototypeOf(original));
  return fn;
}
promisify.custom = promisifyCustom;

function stripVTControlCharacters(str) {
  return String(str).replace(/\x1B\[[0-9;]*[A-Za-z]/g, '');
}

module.exports = {
  inherits: inherits,
  format: format,
  formatWithOptions: formatWithOptions,
  inspect: inspect,
  promisify: promisify,
  stripVTControlCharacters: stripVTControlCharacters,
  types: require("util/types"),
};
