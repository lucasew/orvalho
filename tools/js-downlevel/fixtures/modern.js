// Modern guest JS fixture for the esbuild downlevel pipeline stub.
// Intentionally uses post-ES2015 syntax (optional chaining, nullish coalescing)
// so the downlevel step is observable. Platform APIs are host-provided; this
// file only exercises language syntax.

function greet(user) {
  const name = user?.profile?.name ?? "anonymous";
  const tags = user?.tags ?? [];
  return "hello, " + name + " [" + tags.join(",") + "]";
}

const sampleUser = {
  profile: { name: "dew" },
  tags: ["mesh", "actor"],
};

globalThis.__orvalhoFixture = {
  greet: greet,
  sample: greet(sampleUser),
  missing: greet(null),
};
