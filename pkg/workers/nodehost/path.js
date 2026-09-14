'use strict';

var CHAR_DOT = 46;
var CHAR_FORWARD_SLASH = 47;

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

function isPosixPathSeparator(code) {
  return code === CHAR_FORWARD_SLASH;
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

function posixCwd() {
  return process.cwd();
}

if (require.__posix) {
  module.exports = require.__posix;
} else {
var posix = module.exports;
require.__posix = posix;

posix.resolve = function resolve() {
    var args = arguments;
    if (args.length === 0 || (args.length === 1 && (args[0] === '' || args[0] === '.'))) {
      var fast = posixCwd();
      if (fast.charCodeAt(0) === CHAR_FORWARD_SLASH) {
        return fast;
      }
    }
    var resolvedPath = '';
    var resolvedAbsolute = false;
    var i;
    var path;
    var cwd;

    for (i = args.length - 1; i >= 0 && !resolvedAbsolute; i--) {
      path = args[i];
      validateString(path, 'paths[' + i + ']');
      if (path.length === 0) {
        continue;
      }
      resolvedPath = path + '/' + resolvedPath;
      resolvedAbsolute = path.charCodeAt(0) === CHAR_FORWARD_SLASH;
    }

    if (!resolvedAbsolute) {
      cwd = posixCwd();
      resolvedPath = cwd + '/' + resolvedPath;
      resolvedAbsolute = cwd.charCodeAt(0) === CHAR_FORWARD_SLASH;
    }

    resolvedPath = normalizeString(resolvedPath, !resolvedAbsolute, '/', isPosixPathSeparator);

    if (resolvedAbsolute) {
      return '/' + resolvedPath;
    }
    return resolvedPath.length > 0 ? resolvedPath : '.';
  };

posix.normalize = function normalize(path) {
    validateString(path, 'path');
    if (path.length === 0) {
      return '.';
    }

    var isAbsolute = path.charCodeAt(0) === CHAR_FORWARD_SLASH;
    var trailingSeparator = path.charCodeAt(path.length - 1) === CHAR_FORWARD_SLASH;

    path = normalizeString(path, !isAbsolute, '/', isPosixPathSeparator);

    if (path.length === 0) {
      if (isAbsolute) {
        return '/';
      }
      return trailingSeparator ? './' : '.';
    }
    if (trailingSeparator) {
      path += '/';
    }
    return isAbsolute ? '/' + path : path;
  };

posix.isAbsolute = function isAbsolute(path) {
    validateString(path, 'path');
    return path.length > 0 && path.charCodeAt(0) === CHAR_FORWARD_SLASH;
  };

posix.join = function join() {
    var args = arguments;
    if (args.length === 0) {
      return '.';
    }
    var path = [];
    var i;
    var arg;
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
    return posix.normalize(path.join('/'));
  };

posix.relative = function relative(from, to) {
    validateString(from, 'from');
    validateString(to, 'to');

    if (from === to) {
      return '';
    }

    from = posix.resolve(from);
    to = posix.resolve(to);

    if (from === to) {
      return '';
    }

    var fromStart = 1;
    var fromEnd = from.length;
    var fromLen = fromEnd - fromStart;
    var toStart = 1;
    var toLen = to.length - toStart;
    var length = fromLen < toLen ? fromLen : toLen;
    var lastCommonSep = -1;
    var i = 0;
    var fromCode;
    var out;

    for (; i < length; i++) {
      fromCode = from.charCodeAt(fromStart + i);
      if (fromCode !== to.charCodeAt(toStart + i)) {
        break;
      } else if (fromCode === CHAR_FORWARD_SLASH) {
        lastCommonSep = i;
      }
    }
    if (i === length) {
      if (toLen > length) {
        if (to.charCodeAt(toStart + i) === CHAR_FORWARD_SLASH) {
          return to.slice(toStart + i + 1);
        }
        if (i === 0) {
          return to.slice(toStart + i);
        }
      } else if (fromLen > length) {
        if (from.charCodeAt(fromStart + i) === CHAR_FORWARD_SLASH) {
          lastCommonSep = i;
        } else if (i === 0) {
          lastCommonSep = 0;
        }
      }
    }

    out = '';
    for (i = fromStart + lastCommonSep + 1; i <= fromEnd; ++i) {
      if (i === fromEnd || from.charCodeAt(i) === CHAR_FORWARD_SLASH) {
        out += out.length === 0 ? '..' : '/..';
      }
    }
    return out + to.slice(toStart + lastCommonSep);
  };

posix.toNamespacedPath = function toNamespacedPath(path) {
    return path;
  };

posix.dirname = function dirname(path) {
    validateString(path, 'path');
    if (path.length === 0) {
      return '.';
    }
    var hasRoot = path.charCodeAt(0) === CHAR_FORWARD_SLASH;
    var end = -1;
    var matchedSlash = true;
    var i;
    for (i = path.length - 1; i >= 1; --i) {
      if (path.charCodeAt(i) === CHAR_FORWARD_SLASH) {
        if (!matchedSlash) {
          end = i;
          break;
        }
      } else {
        matchedSlash = false;
      }
    }

    if (end === -1) {
      return hasRoot ? '/' : '.';
    }
    if (hasRoot && end === 1) {
      return '//';
    }
    return path.slice(0, end);
  };

posix.basename = function basename(path, suffix) {
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

    if (suffix !== undefined && suffix.length > 0 && suffix.length <= path.length) {
      if (suffix === path) {
        return '';
      }
      extIdx = suffix.length - 1;
      firstNonSlashEnd = -1;
      for (i = path.length - 1; i >= 0; --i) {
        code = path.charCodeAt(i);
        if (code === CHAR_FORWARD_SLASH) {
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
    for (i = path.length - 1; i >= 0; --i) {
      if (path.charCodeAt(i) === CHAR_FORWARD_SLASH) {
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

posix.extname = function extname(path) {
    validateString(path, 'path');
    var startDot = -1;
    var startPart = 0;
    var end = -1;
    var matchedSlash = true;
    var preDotState = 0;
    var i;
    var ch;
    for (i = path.length - 1; i >= 0; --i) {
      ch = path.charAt(i);
      if (ch === '/') {
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
      if (ch === '.') {
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

posix.format = function format(pathObject) {
    return formatWithSep('/', pathObject);
  };

posix.parse = function parse(path) {
    validateString(path, 'path');

    var ret = { root: '', dir: '', base: '', ext: '', name: '' };
    if (path.length === 0) {
      return ret;
    }
    var isAbsolute = path.charCodeAt(0) === CHAR_FORWARD_SLASH;
    var start = isAbsolute ? 1 : 0;
    if (isAbsolute) {
      ret.root = '/';
    }
    var startDot = -1;
    var startPart = 0;
    var end = -1;
    var matchedSlash = true;
    var i = path.length - 1;
    var preDotState = 0;
    var code;
    var nameStart;

    for (; i >= start; --i) {
      code = path.charCodeAt(i);
      if (code === CHAR_FORWARD_SLASH) {
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
      nameStart = startPart === 0 && isAbsolute ? 1 : startPart;
      if (startDot === -1 ||
          preDotState === 0 ||
          (preDotState === 1 &&
          startDot === end - 1 &&
          startDot === startPart + 1)) {
        ret.base = ret.name = path.slice(nameStart, end);
      } else {
        ret.name = path.slice(nameStart, startDot);
        ret.base = path.slice(nameStart, end);
        ret.ext = path.slice(startDot, end);
      }
    }

    if (startPart > 0) {
      ret.dir = path.slice(0, startPart - 1);
    } else if (isAbsolute) {
      ret.dir = '/';
    }

    return ret;
  };

posix.sep = '/';
posix.delimiter = ':';
posix.posix = posix;
posix._makeLong = posix.toNamespacedPath;
posix.win32 = require("path/win32");
}
