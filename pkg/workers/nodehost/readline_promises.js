'use strict';

function createInterface() {
  return {
    question: function () {
      return Promise.resolve('');
    },
    close: function () {},
    on: function () { return this; },
    once: function () { return this; },
    off: function () { return this; }
  };
}

function Interface() {}
function Readline() {}

module.exports = {
  createInterface: createInterface,
  Interface: Interface,
  Readline: Readline
};
module.exports.default = module.exports;
