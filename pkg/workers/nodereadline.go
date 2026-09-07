package workers

import "github.com/dop251/goja"

// nodeReadlineBinding materializes require("readline") / require("node:readline").
// I/O goes through the caller-injected streams only; the isolate never opens a TTY.
type nodeReadlineBinding struct{}

var _ Binding = nodeReadlineBinding{}

func (nodeReadlineBinding) Materialize(iso *Isolate) (*goja.Object, error) {
	if iso == nil || iso.vm == nil {
		return nil, ErrBindNilIsolate
	}
	for _, key := range []string{"readline", "node:readline"} {
		if v, ok := iso.moduleCache[key]; ok {
			if o, ok := v.(*goja.Object); ok {
				return o, nil
			}
		}
	}
	v, err := runNamedScript(iso.vm, "node:readline", "("+nodeReadlineSource+")()")
	if err != nil {
		return nil, err
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return iso.vm.NewObject(), nil
	}
	return obj, nil
}

// nodeReadlineSource is ES2015. Line splitting and events stay in JS so
// guest listeners see the same objects they passed in.
const nodeReadlineSource = `
function () {
  function invalidArgType(name, expected, actual) {
    var rec = actual === null ? "null" : typeof actual;
    var err = new TypeError(
      "The \"" + name + "\" argument must be of type " + expected + ". Received " + rec
    );
    err.code = "ERR_INVALID_ARG_TYPE";
    return err;
  }

  function Emitter() {
    this._ev = Object.create(null);
  }
  Emitter.prototype.on = function (ev, fn) {
    if (typeof fn !== "function") {
      return this;
    }
    var list = this._ev[ev];
    if (!list) {
      list = this._ev[ev] = [];
    }
    list.push(fn);
    return this;
  };
  Emitter.prototype.addListener = Emitter.prototype.on;
  Emitter.prototype.once = function (ev, fn) {
    var self = this;
    function wrap() {
      self.removeListener(ev, wrap);
      return fn.apply(this, arguments);
    }
    wrap.listener = fn;
    return this.on(ev, wrap);
  };
  Emitter.prototype.removeListener = function (ev, fn) {
    var list = this._ev[ev];
    if (!list) {
      return this;
    }
    var next = [];
    for (var i = 0; i < list.length; i++) {
      if (list[i] !== fn && list[i].listener !== fn) {
        next.push(list[i]);
      }
    }
    this._ev[ev] = next;
    return this;
  };
  Emitter.prototype.off = Emitter.prototype.removeListener;
  Emitter.prototype.removeAllListeners = function (ev) {
    if (arguments.length === 0) {
      this._ev = Object.create(null);
    } else {
      delete this._ev[ev];
    }
    return this;
  };
  Emitter.prototype.emit = function (ev) {
    var list = this._ev[ev];
    if (!list || list.length === 0) {
      return false;
    }
    var args = [];
    for (var i = 1; i < arguments.length; i++) {
      args.push(arguments[i]);
    }
    list = list.slice();
    for (var j = 0; j < list.length; j++) {
      list[j].apply(this, args);
    }
    return true;
  };
  Emitter.prototype.listenerCount = function (ev) {
    var list = this._ev[ev];
    return list ? list.length : 0;
  };

  function writeCSI(stream, code, cb) {
    if (stream && typeof stream.write === "function") {
      stream.write(code);
    }
    if (typeof cb === "function") {
      cb();
    }
    return true;
  }

  function clearLine(stream, dir, cb) {
    var code = "\x1b[2K";
    if (dir > 0) {
      code = "\x1b[0K";
    } else if (dir < 0) {
      code = "\x1b[1K";
    }
    return writeCSI(stream, code, cb);
  }

  function clearScreenDown(stream, cb) {
    return writeCSI(stream, "\x1b[0J", cb);
  }

  function cursorTo(stream, x, y, cb) {
    if (typeof y === "function") {
      cb = y;
      y = undefined;
    }
    var code;
    if (typeof y === "number") {
      code = "\x1b[" + (y + 1) + ";" + (x + 1) + "H";
    } else {
      code = "\x1b[" + (x + 1) + "G";
    }
    return writeCSI(stream, code, cb);
  }

  function moveCursor(stream, dx, dy, cb) {
    if (typeof dy === "function") {
      cb = dy;
      dy = 0;
    }
    var out = "";
    if (dx < 0) {
      out += "\x1b[" + (-dx) + "D";
    } else if (dx > 0) {
      out += "\x1b[" + dx + "C";
    }
    if (dy < 0) {
      out += "\x1b[" + (-dy) + "A";
    } else if (dy > 0) {
      out += "\x1b[" + dy + "B";
    }
    return writeCSI(stream, out, cb);
  }

  function emitKeypressEvents(stream) {
    if (!stream || typeof stream.on !== "function") {
      return;
    }
    if (stream._orvalhoKeypress) {
      return;
    }
    stream._orvalhoKeypress = true;
    stream.on("data", function (chunk) {
      var s = chunk == null ? "" : String(chunk);
      if (s.length === 0) {
        return;
      }
      if (typeof stream.emit === "function") {
        stream.emit("keypress", s, decodeKey(s));
      }
    });
  }

  function decodeKey(s) {
    var key = { sequence: s, name: undefined, ctrl: false, meta: false, shift: false };
    if (s === "\r" || s === "\n") {
      key.name = "return";
    } else if (s === "\t") {
      key.name = "tab";
    } else if (s === "\b" || s === "\x7f") {
      key.name = "backspace";
    } else if (s === "\x1b") {
      key.name = "escape";
    } else if (s === "\x03") {
      key.name = "c";
      key.ctrl = true;
    } else if (s.charCodeAt(0) < 32) {
      key.ctrl = true;
      key.name = String.fromCharCode(s.charCodeAt(0) + 96);
    } else if (s === "\x1b[A") {
      key.name = "up";
    } else if (s === "\x1b[B") {
      key.name = "down";
    } else if (s === "\x1b[C") {
      key.name = "right";
    } else if (s === "\x1b[D") {
      key.name = "left";
    } else {
      key.name = s;
    }
    return key;
  }

  function findSep(buf, crlfInfinity, ending) {
    for (var i = 0; i < buf.length; i++) {
      var c = buf.charAt(i);
      if (c === "\r") {
        if (i + 1 < buf.length && buf.charAt(i + 1) === "\n") {
          return { at: i, n: 2 };
        }
        if (crlfInfinity && !ending && i === buf.length - 1) {
          return null;
        }
        return { at: i, n: 1 };
      }
      if (c === "\n" || c === "\u2028" || c === "\u2029") {
        return { at: i, n: 1 };
      }
    }
    return null;
  }

  function Interface(input, output, completer, terminal) {
    Emitter.call(this);
    var options = input;
    if (options == null || typeof options !== "object" || typeof options.on === "function") {
      options = {
        input: input,
        output: output,
        completer: completer,
        terminal: terminal
      };
    }
    this.input = options.input;
    this.output = options.output;
    this.completer = options.completer;
    this.terminal = !!options.terminal;
    this.crlfDelay = options.crlfDelay;
    this._prompt = options.prompt == null ? "> " : String(options.prompt);
    this.line = "";
    this.cursor = 0;
    this.closed = false;
    this._buf = "";
    var self = this;
    this._onData = function (chunk) {
      self._feed(chunk, false);
    };
    this._onEnd = function () {
      self._feed("", true);
      self.close();
    };
    if (this.input && typeof this.input.on === "function") {
      this.input.on("data", this._onData);
      this.input.on("end", this._onEnd);
    }
  }
  Interface.prototype = Object.create(Emitter.prototype);
  Interface.prototype.constructor = Interface;

  Interface.prototype._feed = function (chunk, ending) {
    if (this.closed) {
      return;
    }
    if (chunk != null && chunk !== "") {
      this._buf += String(chunk);
    }
    var crlfInfinity = this.crlfDelay === Infinity;
    var sep;
    while ((sep = findSep(this._buf, crlfInfinity, ending))) {
      var line = this._buf.slice(0, sep.at);
      this._buf = this._buf.slice(sep.at + sep.n);
      this.line = "";
      this.emit("line", line);
    }
    if (ending && this._buf.length > 0) {
      var last = this._buf;
      this._buf = "";
      this.line = "";
      this.emit("line", last);
    }
  };

  Interface.prototype.setPrompt = function (prompt) {
    this._prompt = prompt == null ? "" : String(prompt);
  };

  Interface.prototype.getPrompt = function () {
    return this._prompt;
  };

  Interface.prototype.prompt = function () {
    if (this.output && typeof this.output.write === "function") {
      this.output.write(this._prompt);
    }
  };

  Interface.prototype.write = function (data) {
    if (data == null) {
      return;
    }
    this._feed(data, false);
  };

  Interface.prototype.question = function (query, options, cb) {
    if (typeof options === "function") {
      cb = options;
      options = undefined;
    }
    if (this.output && typeof this.output.write === "function") {
      this.output.write(query == null ? "" : String(query));
    }
    if (typeof cb === "function") {
      this.once("line", cb);
      return;
    }
    var self = this;
    return new Promise(function (resolve) {
      self.once("line", resolve);
    });
  };

  Interface.prototype.pause = function () {
    if (this.input && typeof this.input.pause === "function") {
      this.input.pause();
    }
    return this;
  };

  Interface.prototype.resume = function () {
    if (this.input && typeof this.input.resume === "function") {
      this.input.resume();
    }
    return this;
  };

  Interface.prototype.close = function () {
    if (this.closed) {
      return;
    }
    this.closed = true;
    if (this.input && typeof this.input.removeListener === "function") {
      this.input.removeListener("data", this._onData);
      this.input.removeListener("end", this._onEnd);
    }
    this.emit("close");
  };

  Interface.prototype.getCursorPos = function () {
    return { rows: 0, cols: this.cursor };
  };

  function createInterface(input, output, completer, terminal) {
    return new Interface(input, output, completer, terminal);
  }

  var api = {
    Interface: Interface,
    createInterface: createInterface,
    emitKeypressEvents: emitKeypressEvents,
    clearLine: clearLine,
    clearScreenDown: clearScreenDown,
    cursorTo: cursorTo,
    moveCursor: moveCursor
  };
  api.default = api;
  return api;
}
`
