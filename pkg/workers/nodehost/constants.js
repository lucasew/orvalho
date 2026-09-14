'use strict';

var os = require('os');
var fs = require('fs');
var osC = os && os.constants ? os.constants : {};
var fsC = fs && fs.constants ? fs.constants : {};
var cryptoC;
try {
  var crypto = require('crypto');
  if (crypto && crypto.constants) cryptoC = crypto.constants;
} catch (e) {}

module.exports = Object.assign({}, osC, fsC, cryptoC || {});
module.exports.os = osC;
module.exports.fs = fsC;
if (cryptoC) module.exports.crypto = cryptoC;
module.exports.default = module.exports;
