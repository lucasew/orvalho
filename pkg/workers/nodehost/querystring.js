'use strict';

function unescape(s) {
  try {
    return decodeURIComponent(String(s).replace(/\+/g, ' '));
  } catch (e) {
    return String(s).replace(/\+/g, ' ');
  }
}

function escape(s) {
  return encodeURIComponent(String(s)).replace(/%20/g, '+');
}

function parse(qs, sep, eq, options) {
  sep = sep || '&';
  eq = eq || '=';
  var obj = Object.create(null);
  if (qs == null || qs === '') return obj;
  qs = String(qs);
  if (qs.charAt(0) === '?') qs = qs.slice(1);
  var maxKeys = 1000;
  if (options && typeof options.maxKeys === 'number') maxKeys = options.maxKeys;
  var pairs = qs.split(sep);
  var n = 0;
  for (var i = 0; i < pairs.length; i++) {
    if (maxKeys > 0 && n >= maxKeys) break;
    var p = pairs[i];
    if (!p) continue;
    var idx = p.indexOf(eq);
    var k, v;
    if (idx >= 0) {
      k = unescape(p.slice(0, idx));
      v = unescape(p.slice(idx + eq.length));
    } else {
      k = unescape(p);
      v = '';
    }
    if (Object.prototype.hasOwnProperty.call(obj, k)) {
      if (Array.isArray(obj[k])) obj[k].push(v);
      else obj[k] = [obj[k], v];
    } else {
      obj[k] = v;
    }
    n++;
  }
  return obj;
}

function stringify(obj, sep, eq) {
  sep = sep || '&';
  eq = eq || '=';
  if (obj == null || typeof obj !== 'object') return '';
  var parts = [];
  var keys = Object.keys(obj);
  for (var i = 0; i < keys.length; i++) {
    var k = keys[i];
    var v = obj[k];
    if (v == null) {
      parts.push(escape(k) + eq);
      continue;
    }
    if (Array.isArray(v)) {
      for (var j = 0; j < v.length; j++) parts.push(escape(k) + eq + escape(v[j]));
    } else {
      parts.push(escape(k) + eq + escape(v));
    }
  }
  return parts.join(sep);
}

exports.parse = parse;
exports.stringify = stringify;
exports.encode = stringify;
exports.decode = parse;
exports.escape = escape;
exports.unescape = unescape;
