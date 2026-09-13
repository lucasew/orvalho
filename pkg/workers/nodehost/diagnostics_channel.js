'use strict';

function channel() {
  return {
    hasSubscribers: false,
    subscribe: function () {},
    unsubscribe: function () {},
    publish: function () {}
  };
}

module.exports = {
  channel: channel,
  subscribe: function () {},
  unsubscribe: function () {},
  hasSubscribers: function () { return false; },
  tracingChannel: function () {
    return { start: {}, end: {}, asyncStart: {}, asyncEnd: {}, error: {} };
  }
};
module.exports.default = module.exports;
