package workers

import "github.com/dop251/goja"

// nodeURLBinding materializes guest require("url") / require("node:url").
// WHATWG URL lives on the isolate global; this Binding re-exports it and
// adds pathToFileURL, fileURLToPath, and legacy format.
type nodeURLBinding struct{}

var _ Binding = nodeURLBinding{}

func (nodeURLBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"url", "node:url"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	v, err := runNamedScript(iso.vm, "node:url", "("+nodeURLSource+")()")
	if err != nil {
		return nil, err
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return iso.vm.NewObject(), nil
	}
	return obj, nil
}

// nodeURLSource is ES2015. Encoding is done in JS so UTF-16 code units
// (including lone surrogates in official pathToFileURL fixtures) stay intact.
const nodeURLSource = `
function () {
  function invalidArgType(name, expected, actual) {
    var rec = actual === null ? "null" : typeof actual;
    var err = new TypeError(
      "The \"" + name + "\" argument must be of type " + expected + ". Received " + rec
    );
    err.code = "ERR_INVALID_ARG_TYPE";
    return err;
  }

  function invalidArgValue(name, value, reason) {
    var err = new TypeError("The argument '" + name + "' " + reason + ". Received " + value);
    err.code = "ERR_INVALID_ARG_VALUE";
    return err;
  }

  function hex(n) {
    var s = n.toString(16).toUpperCase();
    return s.length === 1 ? "0" + s : s;
  }

  // Kept unencoded in Node's pathToFileURL C++ table.
  function isSafe(b) {
    if (b === 0x21 || b === 0x24 || b === 0x3A || b === 0x3B || b === 0x3D || b === 0x40 || b === 0x5F) {
      return true;
    }
    if (b >= 0x26 && b <= 0x39) {
      return true;
    }
    if (b >= 0x41 && b <= 0x5A) {
      return true;
    }
    if (b >= 0x61 && b <= 0x7A) {
      return true;
    }
    return false;
  }

  function encodeCodePoint(cp) {
    if (cp < 0x80) {
      if (isSafe(cp)) {
        return String.fromCharCode(cp);
      }
      return "%" + hex(cp);
    }
    if (cp < 0x800) {
      return "%" + hex(0xC0 | (cp >> 6)) + "%" + hex(0x80 | (cp & 0x3F));
    }
    if (cp < 0x10000) {
      return "%" + hex(0xE0 | (cp >> 12)) + "%" + hex(0x80 | ((cp >> 6) & 0x3F)) + "%" + hex(0x80 | (cp & 0x3F));
    }
    return "%" + hex(0xF0 | (cp >> 18)) +
      "%" + hex(0x80 | ((cp >> 12) & 0x3F)) +
      "%" + hex(0x80 | ((cp >> 6) & 0x3F)) +
      "%" + hex(0x80 | (cp & 0x3F));
  }

  function encodePath(p) {
    var out = "";
    var i = 0;
    while (i < p.length) {
      var c1 = p.charCodeAt(i);
      var cp = c1;
      if (c1 >= 0xD800 && c1 <= 0xDBFF && i + 1 < p.length) {
        var c2 = p.charCodeAt(i + 1);
        if (c2 >= 0xDC00 && c2 <= 0xDFFF) {
          cp = 0x10000 + ((c1 - 0xD800) << 10) + (c2 - 0xDC00);
          i += 2;
        } else {
          i += 1;
        }
      } else {
        i += 1;
      }
      out += encodeCodePoint(cp);
    }
    return out;
  }

  function guestCwd() {
    try {
      if (typeof process !== "undefined" && typeof process.cwd === "function") {
        var c = process.cwd();
        if (c) {
          return c;
        }
      }
    } catch (e) {}
    return "/";
  }

  function posixNormalizeAbs(p) {
    var parts = p.split("/");
    var out = [];
    var i;
    for (i = 0; i < parts.length; i++) {
      if (parts[i] === "" || parts[i] === ".") {
        continue;
      }
      if (parts[i] === "..") {
        if (out.length) {
          out.pop();
        }
        continue;
      }
      out.push(parts[i]);
    }
    return "/" + out.join("/");
  }

  function posixResolve(p) {
    if (p.charAt(0) !== "/") {
      var cwd = guestCwd();
      if (cwd.charAt(cwd.length - 1) === "/") {
        p = cwd + p;
      } else {
        p = cwd + "/" + p;
      }
    }
    return posixNormalizeAbs(p);
  }

  function win32NormalizeAbs(p) {
    var drive = "";
    if (/^[A-Za-z]:/.test(p)) {
      drive = p.slice(0, 2);
      p = p.slice(2);
    }
    if (p.charAt(0) === "\\") {
      p = p.slice(1);
    }
    var parts = p.split("\\");
    var out = [];
    var i;
    for (i = 0; i < parts.length; i++) {
      if (parts[i] === "" || parts[i] === ".") {
        continue;
      }
      if (parts[i] === "..") {
        if (out.length) {
          out.pop();
        }
        continue;
      }
      out.push(parts[i]);
    }
    return drive + "\\" + out.join("\\");
  }

  function win32Resolve(p) {
    p = String(p).replace(/\//g, "\\");
    if (/^[A-Za-z]:\\/.test(p) || /^[A-Za-z]:$/.test(p)) {
      return win32NormalizeAbs(p);
    }
    if (p.indexOf("\\\\") === 0) {
      return p;
    }
    var cwd = guestCwd().replace(/\//g, "\\");
    if (cwd.charAt(cwd.length - 1) !== "\\") {
      cwd += "\\";
    }
    return win32NormalizeAbs(cwd + p);
  }

  function parseFileURL(href) {
    var rest = String(href);
    var protocol = "";
    var colon = rest.indexOf(":");
    if (colon >= 0) {
      protocol = rest.slice(0, colon + 1).toLowerCase();
      rest = rest.slice(colon + 1);
    }
    var hostname = "";
    var pathname = rest;
    if (rest.indexOf("//") === 0) {
      rest = rest.slice(2);
      var slash = rest.indexOf("/");
      if (slash === -1) {
        hostname = rest;
        pathname = "/";
      } else {
        hostname = rest.slice(0, slash);
        pathname = rest.slice(slash);
      }
    }
    var q = pathname.indexOf("?");
    var h = pathname.indexOf("#");
    var end = pathname.length;
    if (q >= 0 && q < end) {
      end = q;
    }
    if (h >= 0 && h < end) {
      end = h;
    }
    pathname = pathname.slice(0, end);
    return { protocol: protocol, hostname: hostname, pathname: pathname };
  }

  function makeFileURL(href) {
    var Ctor = globalThis.URL;
    var u;
    if (typeof Ctor === "function") {
      u = new Ctor(href);
    } else {
      u = {};
    }
    var parsed = parseFileURL(href);
    u.href = href;
    u.protocol = "file:";
    u.hostname = parsed.hostname;
    u.host = parsed.hostname;
    u.pathname = parsed.pathname;
    u.toString = function () { return href; };
    u.toJSON = function () { return href; };
    return u;
  }

  function optionsWindows(options) {
    if (options == null || typeof options !== "object") {
      return false;
    }
    return !!options.windows;
  }

  function windowsUNCToHref(unc, original) {
    var slash = unc.indexOf("\\");
    if (slash === -1) {
      throw invalidArgValue("path", original, "is invalid. Missing UNC resource path");
    }
    if (slash === 0) {
      throw invalidArgValue("path", original, "is invalid. Empty UNC servername");
    }
    var host = unc.slice(0, slash);
    var rest = unc.slice(slash).replace(/\\/g, "/");
    return "file://" + host + encodePath(rest);
  }

  function pathToFileURL(filepath, options) {
    if (typeof filepath !== "string") {
      throw invalidArgType("path", "string", filepath);
    }
    var windows = optionsWindows(options);
    if (windows) {
      if (filepath.indexOf("\\\\?\\UNC\\") === 0) {
        return makeFileURL(windowsUNCToHref(filepath.slice(8), filepath));
      }
      if (filepath.indexOf("\\\\?\\") === 0) {
        filepath = filepath.slice(4);
      }
      if (filepath.indexOf("\\\\") === 0) {
        return makeFileURL(windowsUNCToHref(filepath.slice(2), filepath));
      }
      var wresolved = win32Resolve(filepath);
      var wlast = filepath.charCodeAt(filepath.length - 1);
      if ((wlast === 47 || wlast === 92) && wresolved.charAt(wresolved.length - 1) !== "\\") {
        wresolved += "/";
      }
      var wp = wresolved.replace(/\\/g, "/");
      if (wp.charAt(0) !== "/") {
        wp = "/" + wp;
      }
      return makeFileURL("file://" + encodePath(wp));
    }
    var resolved = posixResolve(filepath);
    var last = filepath.charCodeAt(filepath.length - 1);
    if (last === 47 && resolved.charAt(resolved.length - 1) !== "/") {
      resolved += "/";
    }
    return makeFileURL("file://" + encodePath(resolved));
  }

  function fileURLToPosixPath(hostname, pathname) {
    if (hostname && hostname !== "localhost") {
      var hostErr = new TypeError('File URL host must be "localhost" or empty');
      hostErr.code = "ERR_INVALID_FILE_URL_HOST";
      throw hostErr;
    }
    var n;
    for (n = 0; n < pathname.length; n++) {
      if (pathname.charAt(n) === "%") {
        var third = pathname.charCodeAt(n + 2) | 0x20;
        if (pathname.charAt(n + 1) === "2" && third === 102) {
          var pathErr = new TypeError("File URL path must not include encoded / characters");
          pathErr.code = "ERR_INVALID_FILE_URL_PATH";
          throw pathErr;
        }
      }
    }
    return decodeURIComponent(pathname);
  }

  function fileURLToWinPath(hostname, pathname) {
    var n;
    for (n = 0; n < pathname.length; n++) {
      if (pathname.charAt(n) === "%") {
        var third = pathname.charCodeAt(n + 2) | 0x20;
        if ((pathname.charAt(n + 1) === "2" && third === 102) ||
            (pathname.charAt(n + 1) === "5" && third === 99)) {
          var pathErr = new TypeError("File URL path must not include encoded \\ or / characters");
          pathErr.code = "ERR_INVALID_FILE_URL_PATH";
          throw pathErr;
        }
      }
    }
    var decoded = decodeURIComponent(pathname.replace(/\//g, "\\"));
    if (hostname) {
      return "\\\\" + hostname + decoded;
    }
    var letter = decoded.charCodeAt(1) | 0x20;
    if (letter < 97 || letter > 122 || decoded.charAt(2) !== ":") {
      var absErr = new TypeError("File URL path must be absolute");
      absErr.code = "ERR_INVALID_FILE_URL_PATH";
      throw absErr;
    }
    return decoded.slice(1);
  }

  function fileURLToPath(input, options) {
    var windows = optionsWindows(options);
    var href = "";
    var protocol = "";
    var hostname = "";
    var pathname = "";
    if (typeof input === "string") {
      href = input;
    } else if (input != null && typeof input === "object") {
      if (input.href != null) {
        href = String(input.href);
      }
      if (input.protocol != null) {
        protocol = String(input.protocol);
      }
      if (input.hostname != null) {
        hostname = String(input.hostname);
      }
      if (input.pathname != null) {
        pathname = String(input.pathname);
      }
    } else {
      throw invalidArgType("path", "string or URL instance", input);
    }
    var parsed = parseFileURL(href || (protocol + "//" + hostname + pathname));
    if (protocol !== "file:") {
      protocol = parsed.protocol;
      hostname = parsed.hostname;
      pathname = parsed.pathname;
    } else if (!pathname) {
      hostname = parsed.hostname;
      pathname = parsed.pathname;
    }
    if (protocol !== "file:") {
      var schemeErr = new TypeError("The URL must be of scheme file");
      schemeErr.code = "ERR_INVALID_URL_SCHEME";
      throw schemeErr;
    }
    if (windows) {
      return fileURLToWinPath(hostname, pathname);
    }
    return fileURLToPosixPath(hostname, pathname);
  }

  function formatLegacy(u) {
    var proto = u.protocol == null ? "" : String(u.protocol);
    if (proto && proto.charCodeAt(proto.length - 1) !== 58) {
      proto += ":";
    }
    var pathname = u.pathname == null ? "" : String(u.pathname);
    var hash = u.hash == null ? "" : String(u.hash);
    if (hash && hash.charAt(0) !== "#") {
      hash = "#" + hash;
    }
    var search = "";
    if (u.search != null) {
      search = String(u.search);
      if (search && search.charAt(0) !== "?") {
        search = "?" + search;
      }
    } else if (u.query != null && typeof u.query !== "object") {
      search = String(u.query);
      if (search && search.charAt(0) !== "?") {
        search = "?" + search;
      }
    }
    var host = "";
    if (u.host != null) {
      host = String(u.host);
    } else if (u.hostname != null) {
      host = String(u.hostname);
      if (u.port != null && u.port !== "") {
        host += ":" + u.port;
      }
    }
    var auth = u.auth ? String(u.auth) + "@" : "";
    var slashes = u.slashes;
    if (slashes == null) {
      slashes = proto === "http:" || proto === "https:" || proto === "ftp:" ||
        proto === "gopher:" || proto === "file:" || proto === "ws:" || proto === "wss:";
    }
    if (!proto && !host && !pathname && !search && !hash) {
      return "";
    }
    var out = proto;
    if (slashes) {
      out += "//";
    }
    return out + auth + host + pathname + search + hash;
  }

  function format(urlObject) {
    var t = typeof urlObject;
    if (urlObject === undefined || urlObject === null ||
        t === "boolean" || t === "number" || t === "bigint" ||
        t === "function" || t === "symbol") {
      throw invalidArgType("urlObject", "Object or string", urlObject);
    }
    if (t === "string") {
      return urlObject;
    }
    return formatLegacy(urlObject);
  }

  return {
    URL: globalThis.URL,
    URLSearchParams: globalThis.URLSearchParams,
    pathToFileURL: pathToFileURL,
    fileURLToPath: fileURLToPath,
    format: format
  };
}
`
