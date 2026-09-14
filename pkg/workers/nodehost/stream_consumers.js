'use strict';

function readAll(stream) {
  return new Promise(function (resolve, reject) {
    if (stream && typeof stream.then === 'function') {
      stream.then(readAll).then(resolve, reject);
      return;
    }
    if (!stream || typeof stream.on !== 'function') {
      resolve(Buffer.from([]));
      return;
    }
    var chunks = [];
    stream.on('data', function (c) { chunks.push(c); });
    stream.on('end', function () {
      var buf = Buffer.concat ? Buffer.concat(chunks.map(function (c) {
        return Buffer.isBuffer(c) ? c : Buffer.from(c);
      })) : Buffer.from([]);
      resolve(buf);
    });
    stream.on('error', reject);
  });
}

exports.buffer = readAll;
exports.arrayBuffer = function (stream) {
  return readAll(stream).then(function (b) {
    return b.buffer ? b.buffer : new Uint8Array(b).buffer;
  });
};
exports.text = function (stream) {
  return readAll(stream).then(function (b) { return b.toString(); });
};
exports.json = function (stream) {
  return exports.text(stream).then(function (s) { return JSON.parse(s); });
};
exports.blob = function (stream) {
  return readAll(stream);
};
module.exports = exports;
