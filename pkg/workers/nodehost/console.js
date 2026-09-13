'use strict';

module.exports = globalThis.console;
if (module.exports && !module.exports.default) {
  module.exports.default = module.exports;
}
