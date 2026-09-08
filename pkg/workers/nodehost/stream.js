'use strict';

var EE = require('events');
var util = require('util');

function Stream() {
  EE.call(this);
}
util.inherits(Stream, EE);

Stream.prototype.pipe = function (dest, opts) {
  var src = this;
  function ondata(chunk) {
    if (dest.write && dest.write(chunk) === false && src.pause) src.pause();
  }
  function ondrain() {
    if (src.resume) src.resume();
  }
  function onend() {
    if (dest.end) dest.end();
  }
  src.on('data', ondata);
  dest.on && dest.on('drain', ondrain);
  src.on('end', onend);
  return dest;
};

function Readable(opts) {
  Stream.call(this);
  this.readable = true;
  this._readableState = { ended: false, flowing: false };
}
util.inherits(Readable, Stream);
Readable.prototype.push = function (chunk) {
  if (chunk === null) {
    this._readableState.ended = true;
    this.emit('end');
    return false;
  }
  this.emit('data', chunk);
  return true;
};
Readable.prototype.read = function () { return null; };
Readable.prototype.resume = function () { return this; };
Readable.prototype.pause = function () { return this; };
Readable.prototype.destroy = function (err) {
  if (err) this.emit('error', err);
  this.emit('close');
  return this;
};
Readable.prototype.wrap = function () { return this; };

function Writable(opts) {
  Stream.call(this);
  this.writable = true;
}
util.inherits(Writable, Stream);
Writable.prototype.write = function (chunk, enc, cb) {
  if (typeof enc === 'function') cb = enc;
  if (cb) cb();
  return true;
};
Writable.prototype.end = function (chunk, enc, cb) {
  if (typeof chunk === 'function') cb = chunk;
  else if (chunk != null) this.write(chunk, enc);
  this.emit('finish');
  if (typeof cb === 'function') cb();
  return this;
};
Writable.prototype.destroy = function (err) {
  if (err) this.emit('error', err);
  this.emit('close');
  return this;
};
Writable.prototype.cork = function () {};
Writable.prototype.uncork = function () {};

function Duplex(opts) {
  Readable.call(this, opts);
  Writable.call(this, opts);
}
util.inherits(Duplex, Readable);
Duplex.prototype.write = Writable.prototype.write;
Duplex.prototype.end = Writable.prototype.end;
Duplex.prototype.cork = Writable.prototype.cork;
Duplex.prototype.uncork = Writable.prototype.uncork;

function Transform(opts) {
  Duplex.call(this, opts);
}
util.inherits(Transform, Duplex);
Transform.prototype._transform = function (chunk, enc, cb) { cb(null, chunk); };

function PassThrough(opts) {
  Transform.call(this, opts);
}
util.inherits(PassThrough, Transform);

function pipeline() {
  var args = [];
  for (var i = 0; i < arguments.length; i++) args.push(arguments[i]);
  var cb = typeof args[args.length - 1] === 'function' ? args.pop() : null;
  for (var j = 0; j < args.length - 1; j++) {
    if (args[j] && args[j].pipe) args[j].pipe(args[j + 1]);
  }
  if (cb) {
    var last = args[args.length - 1];
    if (last && last.on) last.on('finish', function () { cb(); });
  }
  return args[args.length - 1];
}

function finished(stream, opts, cb) {
  if (typeof opts === 'function') cb = opts;
  if (cb) {
    var done = false;
    var once = function (err) {
      if (done) return;
      done = true;
      if (err !== undefined && err !== null) cb(err);
      else cb();
    };
    stream.on('finish', function () { once(); });
    stream.on('end', function () { once(); });
    stream.on('error', once);
  }
  return stream;
}

Stream.Readable = Readable;
Stream.Writable = Writable;
Stream.Duplex = Duplex;
Stream.Transform = Transform;
Stream.PassThrough = PassThrough;
Stream.Stream = Stream;
Stream.pipeline = pipeline;
Stream.finished = finished;
module.exports = Stream;
module.exports.default = Stream;
