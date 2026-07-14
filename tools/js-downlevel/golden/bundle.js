(() => {
  // tools/js-downlevel/fixtures/modern.js
  function greet(user) {
    var _a, _b, _c;
    const name = (_b = (_a = user == null ? void 0 : user.profile) == null ? void 0 : _a.name) != null ? _b : "anonymous";
    const tags = (_c = user == null ? void 0 : user.tags) != null ? _c : [];
    return "hello, " + name + " [" + tags.join(",") + "]";
  }
  var sampleUser = {
    profile: { name: "dew" },
    tags: ["mesh", "actor"]
  };
  globalThis.__orvalhoFixture = {
    greet,
    sample: greet(sampleUser),
    missing: greet(null)
  };
})();
