'use strict';

function Domain() {}
Domain.prototype.run = function (fn) {
  return fn.apply(this, Array.prototype.slice.call(arguments, 1));
};
Domain.prototype.bind = function (fn) {
  return typeof fn === 'function' ? fn : function () {};
};
Domain.prototype.intercept = function (fn) {
  return typeof fn === 'function' ? fn : function () {};
};
Domain.prototype.add = function () {};
Domain.prototype.remove = function () {};
Domain.prototype.enter = function () {};
Domain.prototype.exit = function () {};

function create() {
  return new Domain();
}

module.exports = {
  Domain: Domain,
  create: create,
  createDomain: create,
  active: undefined,
};
module.exports.default = module.exports;
