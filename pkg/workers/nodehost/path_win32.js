'use strict';

var win32 = {
  sep: '\\',
  delimiter: ';',
};

win32.win32 = win32;
module.exports = win32;
win32.posix = require("path");
