'use strict';

var posix = {
  sep: '/',
  delimiter: ':',
};

posix.posix = posix;
module.exports = posix;
posix.win32 = require("path/win32");
