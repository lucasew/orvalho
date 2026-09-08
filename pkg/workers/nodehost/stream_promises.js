'use strict';

var stream = require('stream');
var util = require('util');

module.exports = {
  pipeline: util.promisify(stream.pipeline),
  finished: util.promisify(stream.finished),
};
module.exports.default = module.exports;
