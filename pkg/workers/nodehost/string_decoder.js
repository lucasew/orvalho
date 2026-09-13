'use strict';

function StringDecoder(encoding) {
  this.encoding = encoding || 'utf8';
}
StringDecoder.prototype.write = function (buf) {
  if (buf == null) {
    return '';
  }
  if (typeof buf === 'string') {
    return buf;
  }
  if (typeof Buffer === 'function' && Buffer.isBuffer && Buffer.isBuffer(buf)) {
    return buf.toString(this.encoding);
  }
  if (typeof Buffer === 'function') {
    return Buffer.from(buf).toString(this.encoding);
  }
  return String(buf);
};
StringDecoder.prototype.end = function (buf) {
  return buf == null ? '' : this.write(buf);
};

module.exports = {
  StringDecoder: StringDecoder,
};
module.exports.default = module.exports;
