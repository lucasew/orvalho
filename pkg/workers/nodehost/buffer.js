'use strict';

var MAX_LENGTH = 4294967295;
var MAX_STRING_LENGTH = 536870888;
var B64 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';

function normEnc(enc) {
  if (enc == null) return 'utf8';
  enc = String(enc).toLowerCase();
  if (enc === 'utf-8') return 'utf8';
  if (enc === 'ucs2' || enc === 'ucs-2' || enc === 'utf16le' || enc === 'utf-16le') return 'utf16le';
  if (enc === 'binary') return 'latin1';
  return enc;
}

function isEncoding(enc) {
  if (typeof enc !== 'string') return false;
  switch (normEnc(enc)) {
    case 'hex':
    case 'utf8':
    case 'ascii':
    case 'latin1':
    case 'base64':
    case 'base64url':
    case 'utf16le':
      return true;
    default:
      return false;
  }
}

function mark(u8) {
  Object.setPrototypeOf(u8, Buffer.prototype);
  return u8;
}

function checkSize(size) {
  if (typeof size !== 'number' || size !== size || size < 0 || size > MAX_LENGTH) {
    var err = new RangeError('The value of "size" is out of range.');
    err.code = 'ERR_OUT_OF_RANGE';
    throw err;
  }
  return size >>> 0;
}

function unknownEnc(enc) {
  var err = new TypeError('Unknown encoding: ' + enc);
  err.code = 'ERR_UNKNOWN_ENCODING';
  throw err;
}

function utf8Bytes(s) {
  return new TextEncoder().encode(String(s));
}

function fromHex(s) {
  s = String(s);
  var out = [];
  for (var i = 0; i + 1 < s.length; i += 2) {
    var n = parseInt(s.substring(i, i + 2), 16);
    if (n !== n) break;
    out.push(n);
  }
  return mark(Uint8Array.from(out));
}

function fromLatin1(s) {
  s = String(s);
  var out = new Uint8Array(s.length);
  for (var i = 0; i < s.length; i++) out[i] = s.charCodeAt(i) & 255;
  return mark(out);
}

function fromUtf16le(s) {
  s = String(s);
  var out = new Uint8Array(s.length * 2);
  for (var i = 0; i < s.length; i++) {
    var c = s.charCodeAt(i);
    out[i * 2] = c & 255;
    out[i * 2 + 1] = c >> 8;
  }
  return mark(out);
}

function fromBase64(s, url) {
  s = String(s);
  if (url) s = s.replace(/-/g, '+').replace(/_/g, '/');
  s = s.replace(/[^A-Za-z0-9+/=]/g, '');
  while (s.length % 4) s += '=';
  var bin = atob(s);
  return fromLatin1(bin);
}

function Buffer(arg, encOrOffset, length) {
  if (!(this instanceof Buffer)) {
    return Buffer.from(arg, encOrOffset, length);
  }
  return Buffer.from(arg, encOrOffset, length);
}

Buffer.poolSize = 8192;
Buffer.constants = { MAX_LENGTH: MAX_LENGTH, MAX_STRING_LENGTH: MAX_STRING_LENGTH };
Buffer.kMaxLength = MAX_LENGTH;
Buffer.kStringMaxLength = MAX_STRING_LENGTH;

Buffer.isEncoding = isEncoding;

Buffer.isBuffer = function (b) {
  return b instanceof Buffer;
};

Buffer.alloc = function (size, fill, enc) {
  size = checkSize(size);
  var buf = mark(new Uint8Array(size));
  if (fill !== undefined && fill !== 0) buf.fill(fill, 0, size, enc);
  return buf;
};

Buffer.allocUnsafe = function (size) {
  return mark(new Uint8Array(checkSize(size)));
};

Buffer.allocUnsafeSlow = Buffer.allocUnsafe;

Buffer.from = function (value, encOrOffset, length) {
  if (typeof value === 'string') {
    var enc = normEnc(encOrOffset);
    if (encOrOffset != null && !isEncoding(encOrOffset)) unknownEnc(encOrOffset);
    if (enc === 'hex') return fromHex(value);
    if (enc === 'base64') return fromBase64(value, false);
    if (enc === 'base64url') return fromBase64(value, true);
    if (enc === 'latin1' || enc === 'ascii') return fromLatin1(value);
    if (enc === 'utf16le') return fromUtf16le(value);
    return mark(utf8Bytes(value));
  }
  if (value instanceof ArrayBuffer) {
    var off = encOrOffset >>> 0;
    var view = length === undefined ? new Uint8Array(value, off) : new Uint8Array(value, off, length >>> 0);
    return mark(view);
  }
  if (ArrayBuffer.isView(value)) {
    return mark(new Uint8Array(value));
  }
  if (value && typeof value.length === 'number') {
    return mark(Uint8Array.from(value));
  }
  var err = new TypeError(
    'The first argument must be of type string or an instance of Buffer, ArrayBuffer, or Array or an Array-like Object.'
  );
  err.code = 'ERR_INVALID_ARG_TYPE';
  throw err;
};

Buffer.concat = function (list, total) {
  if (!Array.isArray(list)) {
    var e = new TypeError('The "list" argument must be an instance of Array');
    e.code = 'ERR_INVALID_ARG_TYPE';
    throw e;
  }
  if (total == null) {
    total = 0;
    for (var i = 0; i < list.length; i++) total += list[i].length;
  } else {
    total = checkSize(total);
  }
  var out = Buffer.allocUnsafe(total);
  var pos = 0;
  for (var j = 0; j < list.length; j++) {
    var item = list[j];
    out.set(item, pos);
    pos += item.length;
  }
  return out;
};

Buffer.byteLength = function (val, enc) {
  if (typeof val !== 'string') {
    if (val && typeof val.byteLength === 'number') return val.byteLength;
    return 0;
  }
  enc = normEnc(enc);
  if (enc === 'ascii' || enc === 'latin1') return val.length;
  if (enc === 'hex') return Math.floor(val.length / 2);
  if (enc === 'utf16le') return val.length * 2;
  if (enc === 'base64' || enc === 'base64url') {
    var s = String(val).replace(/=+$/, '');
    return Math.floor((s.length * 3) / 4);
  }
  return utf8Bytes(val).length;
};

Buffer.compare = function (a, b) {
  var n = Math.min(a.length, b.length);
  for (var i = 0; i < n; i++) {
    if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
  }
  if (a.length === b.length) return 0;
  return a.length < b.length ? -1 : 1;
};

Buffer.prototype = Object.create(Uint8Array.prototype);
Buffer.prototype.constructor = Buffer;

Buffer.prototype.equals = function (other) {
  return Buffer.compare(this, other) === 0;
};

Buffer.prototype.compare = function (other) {
  return Buffer.compare(this, other);
};

Buffer.prototype.copy = function (target, targetStart, sourceStart, sourceEnd) {
  targetStart = targetStart >>> 0;
  sourceStart = sourceStart >>> 0;
  sourceEnd = sourceEnd == null ? this.length : sourceEnd >>> 0;
  var n = Math.min(sourceEnd - sourceStart, target.length - targetStart);
  if (n <= 0) return 0;
  target.set(this.subarray(sourceStart, sourceStart + n), targetStart);
  return n;
};

Buffer.prototype.slice = function (start, end) {
  return mark(Uint8Array.prototype.subarray.call(this, start, end));
};

Buffer.prototype.subarray = function (start, end) {
  return mark(Uint8Array.prototype.subarray.call(this, start, end));
};

Buffer.prototype.fill = function (val, start, end, enc) {
  start = start >>> 0;
  end = end == null ? this.length : end >>> 0;
  if (typeof val === 'number') {
    for (var i = start; i < end; i++) this[i] = val & 255;
    return this;
  }
  var src = Buffer.from(String(val), enc);
  if (src.length === 0) return this;
  var k = 0;
  for (var j = start; j < end; j++) {
    this[j] = src[k];
    k = (k + 1) % src.length;
  }
  return this;
};

Buffer.prototype.write = function (str, offset, length, enc) {
  if (typeof offset === 'string') {
    enc = offset;
    offset = 0;
    length = this.length;
  } else if (typeof length === 'string') {
    enc = length;
    length = this.length - (offset >>> 0);
  }
  offset = offset >>> 0;
  if (length == null) length = this.length - offset;
  var src = Buffer.from(String(str), enc);
  var n = Math.min(src.length, length, this.length - offset);
  this.set(src.subarray(0, n), offset);
  return n;
};

Buffer.prototype.toString = function (enc, start, end) {
  start = start >>> 0;
  end = end == null ? this.length : end >>> 0;
  if (end > this.length) end = this.length;
  if (start >= end) return '';
  var slice = Uint8Array.prototype.slice.call(this, start, end);
  enc = normEnc(enc);
  if (enc === 'hex') {
    var hex = '';
    for (var i = 0; i < slice.length; i++) {
      var h = slice[i].toString(16);
      hex += h.length === 1 ? '0' + h : h;
    }
    return hex;
  }
  if (enc === 'base64' || enc === 'base64url') {
    var bin = '';
    for (var j = 0; j < slice.length; j++) bin += String.fromCharCode(slice[j]);
    var b64 = btoa(bin);
    if (enc === 'base64url') return b64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
    return b64;
  }
  if (enc === 'latin1' || enc === 'ascii') {
    var s = '';
    var mask = enc === 'ascii' ? 127 : 255;
    for (var k = 0; k < slice.length; k++) s += String.fromCharCode(slice[k] & mask);
    return s;
  }
  if (enc === 'utf16le') {
    var u = '';
    for (var p = 0; p + 1 < slice.length; p += 2) {
      u += String.fromCharCode(slice[p] | (slice[p + 1] << 8));
    }
    return u;
  }
  return new TextDecoder().decode(slice);
};

Buffer.prototype.toJSON = function () {
  var data = [];
  for (var i = 0; i < this.length; i++) data.push(this[i]);
  return { type: 'Buffer', data: data };
};

exports.Buffer = Buffer;
exports.kMaxLength = MAX_LENGTH;
exports.kStringMaxLength = MAX_STRING_LENGTH;
exports.constants = Buffer.constants;
