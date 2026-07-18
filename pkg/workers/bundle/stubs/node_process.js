function getBuiltinModule(name) {
  if (name === "node:process" || name === "process") return process;
  return {};
}
var process = {
  env: globalThis.__orvalhoProcessEnv || {},
  version: "v20.19.0",
  versions: { node: "20.19.0" },
  platform: "linux",
  arch: "x64",
  pid: 1,
  ppid: 0,
  title: "orvalho",
  argv: ["orvalho"],
  cwd: function () {
    return "/";
  },
  nextTick: function (fn) {
    var args = Array.prototype.slice.call(arguments, 1);
    Promise.resolve().then(function () {
      fn.apply(null, args);
    });
  },
  emitWarning: function () {},
  on: function () {
    return process;
  },
  once: function () {
    return process;
  },
  off: function () {
    return process;
  },
  listeners: function () {
    return [];
  },
  getBuiltinModule: getBuiltinModule,
  features: {},
  exit: function () {},
};
export default process;
export var env = process.env;
