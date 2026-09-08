'use strict';

// Native workerd binaries are a non-goal. require("workerd") must not
// throw during config load; Spawn will deny executing the path.
module.exports = {
  default: '/dev/null',
  compatibilityDate: '2026-06-17',
  version: '0.0.0-orvalho'
};
