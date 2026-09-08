'use strict';

// Native workerd binaries are a non-goal. require("workerd") must not
// throw during config load; Spawn will deny executing the path.
// CJS workerd package is the binary path string. spawn(require("workerd")).
module.exports = '/dev/null';
