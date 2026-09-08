'use strict';

// Compact Node punycode: ASCII hosts are identity. ucs2 is whatwg-url's
// code-point view of a string; encode/decode implement RFC 3492.

var base = 36;
var tMin = 1;
var tMax = 26;
var skew = 38;
var damp = 700;
var initialBias = 72;
var initialN = 128;
var delimiter = '-';
var maxInt = 2147483647;

function ucs2decode(string) {
  string = String(string == null ? '' : string);
  var output = [];
  var i = 0;
  while (i < string.length) {
    var value = string.charCodeAt(i++);
    if (value >= 0xd800 && value <= 0xdbff && i < string.length) {
      var extra = string.charCodeAt(i++);
      if ((extra & 0xfc00) === 0xdc00) {
        output.push(((value & 0x3ff) << 10) + (extra & 0x3ff) + 0x10000);
      } else {
        output.push(value);
        i--;
      }
    } else {
      output.push(value);
    }
  }
  return output;
}

function ucs2encode(codePoints) {
  var s = '';
  if (codePoints == null) return s;
  for (var i = 0; i < codePoints.length; i++) {
    var cp = codePoints[i] | 0;
    if (cp > 0xffff) {
      cp -= 0x10000;
      s += String.fromCharCode(((cp >>> 10) & 0x3ff) | 0xd800);
      s += String.fromCharCode((cp & 0x3ff) | 0xdc00);
    } else {
      s += String.fromCharCode(cp);
    }
  }
  return s;
}

function adapt(delta, numPoints, firstTime) {
  delta = firstTime ? Math.floor(delta / damp) : delta >> 1;
  delta += Math.floor(delta / numPoints);
  var k = 0;
  while (delta > ((base - tMin) * tMax) >> 1) {
    delta = Math.floor(delta / (base - tMin));
    k += base;
  }
  return k + Math.floor(((base - tMin + 1) * delta) / (delta + skew));
}

function basicToDigit(codePoint) {
  if (codePoint - 0x30 < 10) return codePoint - 0x16;
  if (codePoint - 0x41 < 26) return codePoint - 0x41;
  if (codePoint - 0x61 < 26) return codePoint - 0x61;
  return base;
}

function digitToBasic(digit, flag) {
  return digit + 22 + 75 * (digit < 26) - ((flag !== 0) << 5);
}

function encode(input) {
  input = ucs2decode(input);
  var output = [];
  var n = initialN;
  var delta = 0;
  var bias = initialBias;
  var handled = 0;
  var i;
  for (i = 0; i < input.length; i++) {
    if (input[i] < 0x80) {
      output.push(String.fromCharCode(input[i]));
      handled++;
    }
  }
  var basic = handled;
  if (basic) output.push(delimiter);
  while (handled < input.length) {
    var m = maxInt;
    for (i = 0; i < input.length; i++) {
      if (input[i] >= n && input[i] < m) m = input[i];
    }
    var handledPlus1 = handled + 1;
    if (m - n > Math.floor((maxInt - delta) / handledPlus1)) {
      throw new RangeError('Overflow: input needs wider integers to process');
    }
    delta += (m - n) * handledPlus1;
    n = m;
    for (i = 0; i < input.length; i++) {
      var cur = input[i];
      if (cur < n) {
        delta++;
        if (delta > maxInt) throw new RangeError('Overflow');
      }
      if (cur === n) {
        var q = delta;
        for (var k = base; ; k += base) {
          var t = k <= bias ? tMin : k >= bias + tMax ? tMax : k - bias;
          if (q < t) break;
          var qMinusT = q - t;
          var baseMinusT = base - t;
          output.push(String.fromCharCode(digitToBasic(t + (qMinusT % baseMinusT), 0)));
          q = Math.floor(qMinusT / baseMinusT);
        }
        output.push(String.fromCharCode(digitToBasic(q, 0)));
        bias = adapt(delta, handledPlus1, handled === basic);
        delta = 0;
        handled++;
      }
    }
    delta++;
    n++;
  }
  return output.join('');
}

function decode(input) {
  input = String(input);
  var output = [];
  var i = 0;
  var n = initialN;
  var bias = initialBias;
  var basic = input.lastIndexOf(delimiter);
  if (basic < 0) basic = 0;
  var j;
  for (j = 0; j < basic; ++j) {
    if (input.charCodeAt(j) >= 0x80) {
      throw new RangeError('Illegal input >= 0x80 (not a basic code point)');
    }
    output.push(input.charCodeAt(j));
  }
  var index = basic > 0 ? basic + 1 : 0;
  while (index < input.length) {
    var oldi = i;
    var w = 1;
    for (var k = base; ; k += base) {
      if (index >= input.length) throw new RangeError('Invalid input');
      var digit = basicToDigit(input.charCodeAt(index++));
      if (digit >= base) throw new RangeError('Invalid input');
      if (digit > Math.floor((maxInt - i) / w)) throw new RangeError('Overflow');
      i += digit * w;
      var t = k <= bias ? tMin : k >= bias + tMax ? tMax : k - bias;
      if (digit < t) break;
      var baseMinusT = base - t;
      if (w > Math.floor(maxInt / baseMinusT)) throw new RangeError('Overflow');
      w *= baseMinusT;
    }
    var out = output.length + 1;
    bias = adapt(i - oldi, out, oldi === 0);
    if (Math.floor(i / out) > maxInt - n) throw new RangeError('Overflow');
    n += Math.floor(i / out);
    i %= out;
    output.splice(i++, 0, n);
  }
  return ucs2encode(output);
}

function mapDomain(domain, fn) {
  domain = String(domain);
  var parts = domain.split('@');
  var result = '';
  if (parts.length > 1) {
    result = parts[0] + '@';
    domain = parts[1];
  }
  domain = domain.replace(/[\x2E\u3002\uFF0E\uFF61]/g, '.');
  var labels = domain.split('.');
  for (var i = 0; i < labels.length; i++) {
    labels[i] = fn(labels[i]);
  }
  return result + labels.join('.');
}

function toASCII(domain) {
  return mapDomain(domain, function (label) {
    if (!/[^\x00-\x7F]/.test(label)) return label;
    return 'xn--' + encode(label);
  });
}

function toUnicode(domain) {
  return mapDomain(domain, function (label) {
    if (/^xn--/i.test(label)) {
      try {
        return decode(label.slice(4).toLowerCase());
      } catch (e) {
        return label;
      }
    }
    return label;
  });
}

module.exports = {
  version: '2.3.1',
  ucs2: { decode: ucs2decode, encode: ucs2encode },
  decode: decode,
  encode: encode,
  toASCII: toASCII,
  toUnicode: toUnicode,
};
module.exports.default = module.exports;
