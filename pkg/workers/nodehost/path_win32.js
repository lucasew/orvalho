'use strict';

var CHAR_UPPERCASE_A = 65;
var CHAR_LOWERCASE_A = 97;
var CHAR_UPPERCASE_Z = 90;
var CHAR_LOWERCASE_Z = 122;
var CHAR_DOT = 46;
var CHAR_FORWARD_SLASH = 47;
var CHAR_BACKWARD_SLASH = 92;
var CHAR_COLON = 58;
var CHAR_QUESTION_MARK = 63;

var WINDOWS_RESERVED_NAMES = [
  'CON', 'PRN', 'AUX', 'NUL',
  'COM1', 'COM2', 'COM3', 'COM4', 'COM5', 'COM6', 'COM7', 'COM8', 'COM9',
  'LPT1', 'LPT2', 'LPT3', 'LPT4', 'LPT5', 'LPT6', 'LPT7', 'LPT8', 'LPT9',
  'COM\xb9', 'COM\xb2', 'COM\xb3',
  'LPT\xb9', 'LPT\xb2', 'LPT\xb3',
];

function invalidArgType(name, expected, actual) {
  var kind = name.indexOf('.') >= 0 ? 'property' : 'argument';
  var rec = actual === null ? 'null' : actual === undefined ? 'undefined' : typeof actual;
  var err = new TypeError(
    'The "' + name + '" ' + kind + ' must be of type ' + expected + '. Received ' + rec
  );
  err.code = 'ERR_INVALID_ARG_TYPE';
  return err;
}

function validateString(value, name) {
  if (typeof value !== 'string') {
    throw invalidArgType(name, 'string', value);
  }
}

function validateObject(value, name) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw invalidArgType(name, 'Object', value);
  }
}

function isPathSeparator(code) {
  return code === CHAR_FORWARD_SLASH || code === CHAR_BACKWARD_SLASH;
}

function isPosixPathSeparator(code) {
  return code === CHAR_FORWARD_SLASH;
}

function isWindowsDeviceRoot(code) {
  return (code >= CHAR_UPPERCASE_A && code <= CHAR_UPPERCASE_Z) ||
         (code >= CHAR_LOWERCASE_A && code <= CHAR_LOWERCASE_Z);
}

function isWindowsReservedName(path, colonIndex) {
  var devicePart = path.slice(0, colonIndex).toUpperCase();
  return WINDOWS_RESERVED_NAMES.indexOf(devicePart) !== -1;
}

function normalizeString(path, allowAboveRoot, separator, isSep) {
  var res = '';
  var lastSegmentLength = 0;
  var lastSlash = -1;
  var dots = 0;
  var code = 0;
  var i;
  var lastSlashIndex;
  for (i = 0; i <= path.length; ++i) {
    if (i < path.length) {
      code = path.charCodeAt(i);
    } else if (isSep(code)) {
      break;
    } else {
      code = CHAR_FORWARD_SLASH;
    }

    if (isSep(code)) {
      if (lastSlash === i - 1 || dots === 1) {
        // NOOP
      } else if (dots === 2) {
        if (res.length < 2 || lastSegmentLength !== 2 ||
            res.charCodeAt(res.length - 1) !== CHAR_DOT ||
            res.charCodeAt(res.length - 2) !== CHAR_DOT) {
          if (res.length > 2) {
            lastSlashIndex = res.length - lastSegmentLength - 1;
            if (lastSlashIndex === -1) {
              res = '';
              lastSegmentLength = 0;
            } else {
              res = res.slice(0, lastSlashIndex);
              lastSegmentLength = res.length - 1 - res.lastIndexOf(separator);
            }
            lastSlash = i;
            dots = 0;
            continue;
          } else if (res.length !== 0) {
            res = '';
            lastSegmentLength = 0;
            lastSlash = i;
            dots = 0;
            continue;
          }
        }
        if (allowAboveRoot) {
          res += res.length > 0 ? separator + '..' : '..';
          lastSegmentLength = 2;
        }
      } else {
        if (res.length > 0) {
          res += separator + path.slice(lastSlash + 1, i);
        } else {
          res = path.slice(lastSlash + 1, i);
        }
        lastSegmentLength = i - lastSlash - 1;
      }
      lastSlash = i;
      dots = 0;
    } else if (code === CHAR_DOT && dots !== -1) {
      ++dots;
    } else {
      dots = -1;
    }
  }
  return res;
}

function formatExt(ext) {
  return ext ? (ext.charAt(0) === '.' ? ext : '.' + ext) : '';
}

function formatWithSep(sep, pathObject) {
  validateObject(pathObject, 'pathObject');
  var dir = pathObject.dir || pathObject.root;
  var base = pathObject.base || ((pathObject.name || '') + formatExt(pathObject.ext));
  if (!dir) {
    return base;
  }
  return dir === pathObject.root ? dir + base : dir + sep + base;
}

function repeatStr(str, n) {
  var out = '';
  var i;
  for (i = 0; i < n; i++) {
    out += str;
  }
  return out;
}

function isWindowsPlatform() {
  return typeof process !== 'undefined' && process.platform === 'win32';
}

var forwardSlashRegExp = /\//g;

if (require.__win32) {
  module.exports = require.__win32;
} else {
var win32 = module.exports;
require.__win32 = win32;

win32.resolve = function resolve() {
    var args = arguments;
    var resolvedDevice = '';
    var resolvedTail = '';
    var resolvedAbsolute = false;
    var i;
    var path;
    var len;
    var rootEnd;
    var device;
    var isAbsolute;
    var code;
    var j;
    var last;
    var firstPart;
    var envKey;
    var envPath;

    for (i = args.length - 1; i >= -1; i--) {
      if (i >= 0) {
        path = args[i];
        validateString(path, 'paths[' + i + ']');
        if (path.length === 0) {
          continue;
        }
      } else if (resolvedDevice.length === 0) {
        path = process.cwd();
        if (args.length === 0 || ((args.length === 1 && (args[0] === '' || args[0] === '.')) &&
            isPathSeparator(path.charCodeAt(0)))) {
          if (!isWindowsPlatform()) {
            path = path.replace(forwardSlashRegExp, '\\');
          }
          return path;
        }
      } else {
        envKey = '=' + resolvedDevice;
        envPath = process.env[envKey];
        path = envPath || process.cwd();
        if (path === undefined ||
            (path.slice(0, 2).toLowerCase() !== resolvedDevice.toLowerCase() &&
            path.charCodeAt(2) === CHAR_BACKWARD_SLASH)) {
          path = resolvedDevice + '\\';
        }
      }

      len = path.length;
      rootEnd = 0;
      device = '';
      isAbsolute = false;
      code = path.charCodeAt(0);

      if (len === 1) {
        if (isPathSeparator(code)) {
          rootEnd = 1;
          isAbsolute = true;
        }
      } else if (isPathSeparator(code)) {
        isAbsolute = true;
        if (isPathSeparator(path.charCodeAt(1))) {
          j = 2;
          last = j;
          while (j < len && !isPathSeparator(path.charCodeAt(j))) {
            j++;
          }
          if (j < len && j !== last) {
            firstPart = path.slice(last, j);
            last = j;
            while (j < len && isPathSeparator(path.charCodeAt(j))) {
              j++;
            }
            if (j < len && j !== last) {
              last = j;
              while (j < len && !isPathSeparator(path.charCodeAt(j))) {
                j++;
              }
              if (j === len || j !== last) {
                if (firstPart !== '.' && firstPart !== '?') {
                  device = '\\\\' + firstPart + '\\' + path.slice(last, j);
                  rootEnd = j;
                } else {
                  device = '\\\\' + firstPart;
                  rootEnd = 4;
                }
              }
            }
          }
        } else {
          rootEnd = 1;
        }
      } else if (isWindowsDeviceRoot(code) && path.charCodeAt(1) === CHAR_COLON) {
        device = path.slice(0, 2);
        rootEnd = 2;
        if (len > 2 && isPathSeparator(path.charCodeAt(2))) {
          isAbsolute = true;
          rootEnd = 3;
        }
      }

      if (device.length > 0) {
        if (resolvedDevice.length > 0) {
          if (device.toLowerCase() !== resolvedDevice.toLowerCase()) {
            continue;
          }
        } else {
          resolvedDevice = device;
        }
      }

      if (resolvedAbsolute) {
        if (resolvedDevice.length > 0) {
          break;
        }
      } else {
        resolvedTail = path.slice(rootEnd) + '\\' + resolvedTail;
        resolvedAbsolute = isAbsolute;
        if (isAbsolute && resolvedDevice.length > 0) {
          break;
        }
      }
    }

    resolvedTail = normalizeString(resolvedTail, !resolvedAbsolute, '\\', isPathSeparator);

    return resolvedAbsolute ?
      resolvedDevice + '\\' + resolvedTail :
      resolvedDevice + resolvedTail || '.';
  };

win32.normalize = function normalize(path) {
    validateString(path, 'path');
    var len = path.length;
    if (len === 0) {
      return '.';
    }
    var rootEnd = 0;
    var device;
    var isAbsolute = false;
    var code = path.charCodeAt(0);
    var j;
    var last;
    var firstPart;
    var colonIndex;
    var possibleDevice;
    var tail;
    var index;

    if (len === 1) {
      return isPosixPathSeparator(code) ? '\\' : path;
    }
    if (isPathSeparator(code)) {
      isAbsolute = true;
      if (isPathSeparator(path.charCodeAt(1))) {
        j = 2;
        last = j;
        while (j < len && !isPathSeparator(path.charCodeAt(j))) {
          j++;
        }
        if (j < len && j !== last) {
          firstPart = path.slice(last, j);
          last = j;
          while (j < len && isPathSeparator(path.charCodeAt(j))) {
            j++;
          }
          if (j < len && j !== last) {
            last = j;
            while (j < len && !isPathSeparator(path.charCodeAt(j))) {
              j++;
            }
            if (j === len || j !== last) {
              if (firstPart === '.' || firstPart === '?') {
                device = '\\\\' + firstPart;
                rootEnd = 4;
                colonIndex = path.indexOf(':');
                possibleDevice = path.slice(4, colonIndex + 1);
                if (isWindowsReservedName(possibleDevice, possibleDevice.length - 1)) {
                  device = '\\\\?\\' + possibleDevice;
                  rootEnd = 4 + possibleDevice.length;
                }
              } else if (j === len) {
                return '\\\\' + firstPart + '\\' + path.slice(last) + '\\';
              } else {
                device = '\\\\' + firstPart + '\\' + path.slice(last, j);
                rootEnd = j;
              }
            }
          }
        }
      } else {
        rootEnd = 1;
      }
    } else {
      colonIndex = path.indexOf(':');
      if (colonIndex > 0) {
        if (isWindowsDeviceRoot(code) && colonIndex === 1) {
          device = path.slice(0, 2);
          rootEnd = 2;
          if (len > 2 && isPathSeparator(path.charCodeAt(2))) {
            isAbsolute = true;
            rootEnd = 3;
          }
        } else if (isWindowsReservedName(path, colonIndex)) {
          device = path.slice(0, colonIndex + 1);
          rootEnd = colonIndex + 1;
        }
      }
    }

    tail = rootEnd < len ?
      normalizeString(path.slice(rootEnd), !isAbsolute, '\\', isPathSeparator) :
      '';
    if (tail.length === 0 && !isAbsolute) {
      tail = '.';
    }
    if (tail.length > 0 && isPathSeparator(path.charCodeAt(len - 1))) {
      tail += '\\';
    }
    if (!isAbsolute && device === undefined && path.indexOf(':') !== -1) {
      if (tail.length >= 2 &&
          isWindowsDeviceRoot(tail.charCodeAt(0)) &&
          tail.charCodeAt(1) === CHAR_COLON) {
        return '.\\' + tail;
      }
      index = path.indexOf(':');
      do {
        if (index === len - 1 || isPathSeparator(path.charCodeAt(index + 1))) {
          return '.\\' + tail;
        }
      } while ((index = path.indexOf(':', index + 1)) !== -1);
    }
    colonIndex = path.indexOf(':');
    if (isWindowsReservedName(path, colonIndex)) {
      return '.\\' + (device == null ? '' : device) + tail;
    }
    if (device === undefined) {
      return isAbsolute ? '\\' + tail : tail;
    }
    return isAbsolute ? device + '\\' + tail : device + tail;
  };

win32.isAbsolute = function isAbsolute(path) {
    validateString(path, 'path');
    var len = path.length;
    if (len === 0) {
      return false;
    }
    var code = path.charCodeAt(0);
    return isPathSeparator(code) ||
      (len > 2 &&
      isWindowsDeviceRoot(code) &&
      path.charCodeAt(1) === CHAR_COLON &&
      isPathSeparator(path.charCodeAt(2)));
  };

win32.join = function join() {
    var args = arguments;
    if (args.length === 0) {
      return '.';
    }

    var path = [];
    var i;
    var arg;
    var firstPart;
    var joined;
    var needsReplace;
    var slashCount;
    var firstLen;
    var parts;
    var part;
    var result;
    var colonIndex;
    var reserved;

    for (i = 0; i < args.length; ++i) {
      arg = args[i];
      validateString(arg, 'path');
      if (arg.length > 0) {
        path.push(arg);
      }
    }

    if (path.length === 0) {
      return '.';
    }

    firstPart = path[0];
    joined = path.join('\\');

    needsReplace = true;
    slashCount = 0;
    if (isPathSeparator(firstPart.charCodeAt(0))) {
      ++slashCount;
      firstLen = firstPart.length;
      if (firstLen > 1 && isPathSeparator(firstPart.charCodeAt(1))) {
        ++slashCount;
        if (firstLen > 2) {
          if (isPathSeparator(firstPart.charCodeAt(2))) {
            ++slashCount;
          } else {
            needsReplace = false;
          }
        }
      }
    }
    if (needsReplace) {
      while (slashCount < joined.length && isPathSeparator(joined.charCodeAt(slashCount))) {
        slashCount++;
      }
      if (slashCount >= 2) {
        joined = '\\' + joined.slice(slashCount);
      }
    }

    parts = [];
    part = '';
    for (i = 0; i < joined.length; i++) {
      if (joined.charAt(i) === '\\') {
        if (part) {
          parts.push(part);
        }
        part = '';
        while (i + 1 < joined.length && joined.charAt(i + 1) === '\\') {
          i++;
        }
      } else {
        part += joined.charAt(i);
      }
    }
    if (part) {
      parts.push(part);
    }

    reserved = false;
    for (i = 0; i < parts.length; i++) {
      colonIndex = parts[i].indexOf(':');
      if (colonIndex !== -1 && isWindowsReservedName(parts[i], colonIndex)) {
        reserved = true;
        break;
      }
    }
    if (reserved) {
      result = '';
      for (i = 0; i < joined.length; i++) {
        result += joined.charAt(i) === '/' ? '\\' : joined.charAt(i);
      }
      return result;
    }

    return win32.normalize(joined);
  };

win32.relative = function relative(from, to) {
    validateString(from, 'from');
    validateString(to, 'to');

    if (from === to) {
      return '';
    }

    var fromOrig = win32.resolve(from);
    var toOrig = win32.resolve(to);

    if (fromOrig === toOrig) {
      return '';
    }

    from = fromOrig.toLowerCase();
    to = toOrig.toLowerCase();

    if (from === to) {
      return '';
    }

    var fromSplit;
    var toSplit;
    var fromLen;
    var toLen;
    var length;
    var i;
    var fromStart;
    var fromEnd;
    var toStart;
    var toEnd;
    var lastCommonSep;
    var fromCode;
    var out;

    if (fromOrig.length !== from.length || toOrig.length !== to.length) {
      fromSplit = fromOrig.split('\\');
      toSplit = toOrig.split('\\');
      if (fromSplit[fromSplit.length - 1] === '') {
        fromSplit.pop();
      }
      if (toSplit[toSplit.length - 1] === '') {
        toSplit.pop();
      }

      fromLen = fromSplit.length;
      toLen = toSplit.length;
      length = fromLen < toLen ? fromLen : toLen;

      for (i = 0; i < length; i++) {
        if (fromSplit[i].toLowerCase() !== toSplit[i].toLowerCase()) {
          break;
        }
      }

      if (i === 0) {
        return toOrig;
      } else if (i === length) {
        if (toLen > length) {
          return toSplit.slice(i).join('\\');
        }
        if (fromLen > length) {
          return repeatStr('..\\', fromLen - 1 - i) + '..';
        }
        return '';
      }

      return repeatStr('..\\', fromLen - i) + toSplit.slice(i).join('\\');
    }

    fromStart = 0;
    while (fromStart < from.length && from.charCodeAt(fromStart) === CHAR_BACKWARD_SLASH) {
      fromStart++;
    }
    fromEnd = from.length;
    while (fromEnd - 1 > fromStart && from.charCodeAt(fromEnd - 1) === CHAR_BACKWARD_SLASH) {
      fromEnd--;
    }
    fromLen = fromEnd - fromStart;

    toStart = 0;
    while (toStart < to.length && to.charCodeAt(toStart) === CHAR_BACKWARD_SLASH) {
      toStart++;
    }
    toEnd = to.length;
    while (toEnd - 1 > toStart && to.charCodeAt(toEnd - 1) === CHAR_BACKWARD_SLASH) {
      toEnd--;
    }
    toLen = toEnd - toStart;

    length = fromLen < toLen ? fromLen : toLen;
    lastCommonSep = -1;
    i = 0;
    for (; i < length; i++) {
      fromCode = from.charCodeAt(fromStart + i);
      if (fromCode !== to.charCodeAt(toStart + i)) {
        break;
      } else if (fromCode === CHAR_BACKWARD_SLASH) {
        lastCommonSep = i;
      }
    }

    if (i !== length) {
      if (lastCommonSep === -1) {
        return toOrig;
      }
    } else {
      if (toLen > length) {
        if (to.charCodeAt(toStart + i) === CHAR_BACKWARD_SLASH) {
          return toOrig.slice(toStart + i + 1);
        }
        if (i === 2) {
          return toOrig.slice(toStart + i);
        }
      }
      if (fromLen > length) {
        if (from.charCodeAt(fromStart + i) === CHAR_BACKWARD_SLASH) {
          lastCommonSep = i;
        } else if (i === 2) {
          lastCommonSep = 3;
        }
      }
      if (lastCommonSep === -1) {
        lastCommonSep = 0;
      }
    }

    out = '';
    for (i = fromStart + lastCommonSep + 1; i <= fromEnd; ++i) {
      if (i === fromEnd || from.charCodeAt(i) === CHAR_BACKWARD_SLASH) {
        out += out.length === 0 ? '..' : '\\..';
      }
    }

    toStart += lastCommonSep;

    if (out.length > 0) {
      return out + toOrig.slice(toStart, toEnd);
    }

    if (toOrig.charCodeAt(toStart) === CHAR_BACKWARD_SLASH) {
      ++toStart;
    }
    return toOrig.slice(toStart, toEnd);
  };

win32.toNamespacedPath = function toNamespacedPath(path) {
    if (typeof path !== 'string' || path.length === 0) {
      return path;
    }

    var resolvedPath = win32.resolve(path);
    var code;

    if (resolvedPath.length <= 2) {
      return path;
    }

    if (resolvedPath.charCodeAt(0) === CHAR_BACKWARD_SLASH) {
      if (resolvedPath.charCodeAt(1) === CHAR_BACKWARD_SLASH) {
        code = resolvedPath.charCodeAt(2);
        if (code !== CHAR_QUESTION_MARK && code !== CHAR_DOT) {
          return '\\\\?\\UNC\\' + resolvedPath.slice(2);
        }
      }
    } else if (
      isWindowsDeviceRoot(resolvedPath.charCodeAt(0)) &&
      resolvedPath.charCodeAt(1) === CHAR_COLON &&
      resolvedPath.charCodeAt(2) === CHAR_BACKWARD_SLASH
    ) {
      return '\\\\?\\' + resolvedPath;
    }

    return resolvedPath;
  };

win32.dirname = function dirname(path) {
    validateString(path, 'path');
    var len = path.length;
    if (len === 0) {
      return '.';
    }
    var rootEnd = -1;
    var offset = 0;
    var code = path.charCodeAt(0);
    var j;
    var last;
    var end;
    var matchedSlash;
    var i;

    if (len === 1) {
      return isPathSeparator(code) ? path : '.';
    }

    if (isPathSeparator(code)) {
      rootEnd = offset = 1;
      if (isPathSeparator(path.charCodeAt(1))) {
        j = 2;
        last = j;
        while (j < len && !isPathSeparator(path.charCodeAt(j))) {
          j++;
        }
        if (j < len && j !== last) {
          last = j;
          while (j < len && isPathSeparator(path.charCodeAt(j))) {
            j++;
          }
          if (j < len && j !== last) {
            last = j;
            while (j < len && !isPathSeparator(path.charCodeAt(j))) {
              j++;
            }
            if (j === len) {
              return path;
            }
            if (j !== last) {
              rootEnd = offset = j + 1;
            }
          }
        }
      }
    } else if (isWindowsDeviceRoot(code) && path.charCodeAt(1) === CHAR_COLON) {
      rootEnd = len > 2 && isPathSeparator(path.charCodeAt(2)) ? 3 : 2;
      offset = rootEnd;
    }

    end = -1;
    matchedSlash = true;
    for (i = len - 1; i >= offset; --i) {
      if (isPathSeparator(path.charCodeAt(i))) {
        if (!matchedSlash) {
          end = i;
          break;
        }
      } else {
        matchedSlash = false;
      }
    }

    if (end === -1) {
      if (rootEnd === -1) {
        return '.';
      }
      end = rootEnd;
    }
    return path.slice(0, end);
  };

win32.basename = function basename(path, suffix) {
    if (suffix !== undefined) {
      validateString(suffix, 'suffix');
    }
    validateString(path, 'path');
    var start = 0;
    var end = -1;
    var matchedSlash = true;
    var i;
    var code;
    var extIdx;
    var firstNonSlashEnd;

    if (path.length >= 2 &&
        isWindowsDeviceRoot(path.charCodeAt(0)) &&
        path.charCodeAt(1) === CHAR_COLON) {
      start = 2;
    }

    if (suffix !== undefined && suffix.length > 0 && suffix.length <= path.length) {
      if (suffix === path) {
        return '';
      }
      extIdx = suffix.length - 1;
      firstNonSlashEnd = -1;
      for (i = path.length - 1; i >= start; --i) {
        code = path.charCodeAt(i);
        if (isPathSeparator(code)) {
          if (!matchedSlash) {
            start = i + 1;
            break;
          }
        } else {
          if (firstNonSlashEnd === -1) {
            matchedSlash = false;
            firstNonSlashEnd = i + 1;
          }
          if (extIdx >= 0) {
            if (code === suffix.charCodeAt(extIdx)) {
              if (--extIdx === -1) {
                end = i;
              }
            } else {
              extIdx = -1;
              end = firstNonSlashEnd;
            }
          }
        }
      }

      if (start === end) {
        end = firstNonSlashEnd;
      } else if (end === -1) {
        end = path.length;
      }
      return path.slice(start, end);
    }
    for (i = path.length - 1; i >= start; --i) {
      if (isPathSeparator(path.charCodeAt(i))) {
        if (!matchedSlash) {
          start = i + 1;
          break;
        }
      } else if (end === -1) {
        matchedSlash = false;
        end = i + 1;
      }
    }

    if (end === -1) {
      return '';
    }
    return path.slice(start, end);
  };

win32.extname = function extname(path) {
    validateString(path, 'path');
    var start = 0;
    var startDot = -1;
    var startPart = 0;
    var end = -1;
    var matchedSlash = true;
    var preDotState = 0;
    var i;
    var code;

    if (path.length >= 2 &&
        path.charCodeAt(1) === CHAR_COLON &&
        isWindowsDeviceRoot(path.charCodeAt(0))) {
      start = startPart = 2;
    }

    for (i = path.length - 1; i >= start; --i) {
      code = path.charCodeAt(i);
      if (isPathSeparator(code)) {
        if (!matchedSlash) {
          startPart = i + 1;
          break;
        }
        continue;
      }
      if (end === -1) {
        matchedSlash = false;
        end = i + 1;
      }
      if (code === CHAR_DOT) {
        if (startDot === -1) {
          startDot = i;
        } else if (preDotState !== 1) {
          preDotState = 1;
        }
      } else if (startDot !== -1) {
        preDotState = -1;
      }
    }

    if (startDot === -1 ||
        end === -1 ||
        preDotState === 0 ||
        (preDotState === 1 &&
         startDot === end - 1 &&
         startDot === startPart + 1)) {
      return '';
    }
    return path.slice(startDot, end);
  };

win32.format = function format(pathObject) {
    return formatWithSep('\\', pathObject);
  };

win32.parse = function parse(path) {
    validateString(path, 'path');

    var ret = { root: '', dir: '', base: '', ext: '', name: '' };
    if (path.length === 0) {
      return ret;
    }

    var len = path.length;
    var rootEnd = 0;
    var code = path.charCodeAt(0);
    var j;
    var last;
    var startDot;
    var startPart;
    var end;
    var matchedSlash;
    var i;
    var preDotState;

    if (len === 1) {
      if (isPathSeparator(code)) {
        ret.root = ret.dir = path;
        return ret;
      }
      ret.base = ret.name = path;
      return ret;
    }
    if (isPathSeparator(code)) {
      rootEnd = 1;
      if (isPathSeparator(path.charCodeAt(1))) {
        j = 2;
        last = j;
        while (j < len && !isPathSeparator(path.charCodeAt(j))) {
          j++;
        }
        if (j < len && j !== last) {
          last = j;
          while (j < len && isPathSeparator(path.charCodeAt(j))) {
            j++;
          }
          if (j < len && j !== last) {
            last = j;
            while (j < len && !isPathSeparator(path.charCodeAt(j))) {
              j++;
            }
            if (j === len) {
              rootEnd = j;
            } else if (j !== last) {
              rootEnd = j + 1;
            }
          }
        }
      }
    } else if (isWindowsDeviceRoot(code) && path.charCodeAt(1) === CHAR_COLON) {
      if (len <= 2) {
        ret.root = ret.dir = path;
        return ret;
      }
      rootEnd = 2;
      if (isPathSeparator(path.charCodeAt(2))) {
        if (len === 3) {
          ret.root = ret.dir = path;
          return ret;
        }
        rootEnd = 3;
      }
    }
    if (rootEnd > 0) {
      ret.root = path.slice(0, rootEnd);
    }

    startDot = -1;
    startPart = rootEnd;
    end = -1;
    matchedSlash = true;
    i = path.length - 1;
    preDotState = 0;

    for (; i >= rootEnd; --i) {
      code = path.charCodeAt(i);
      if (isPathSeparator(code)) {
        if (!matchedSlash) {
          startPart = i + 1;
          break;
        }
        continue;
      }
      if (end === -1) {
        matchedSlash = false;
        end = i + 1;
      }
      if (code === CHAR_DOT) {
        if (startDot === -1) {
          startDot = i;
        } else if (preDotState !== 1) {
          preDotState = 1;
        }
      } else if (startDot !== -1) {
        preDotState = -1;
      }
    }

    if (end !== -1) {
      if (startDot === -1 ||
          preDotState === 0 ||
          (preDotState === 1 &&
           startDot === end - 1 &&
           startDot === startPart + 1)) {
        ret.base = ret.name = path.slice(startPart, end);
      } else {
        ret.name = path.slice(startPart, startDot);
        ret.base = path.slice(startPart, end);
        ret.ext = path.slice(startDot, end);
      }
    }

    if (startPart > 0 && startPart !== rootEnd) {
      ret.dir = path.slice(0, startPart - 1);
    } else {
      ret.dir = ret.root;
    }

    return ret;
  };

win32.sep = '\\';
win32.delimiter = ';';
win32.win32 = win32;
win32._makeLong = win32.toNamespacedPath;
win32.posix = require("path");
}
