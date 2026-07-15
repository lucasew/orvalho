// Cat facts SSR demo package (hand-written Workers-shaped JS; no Astro).
// When outbound fetch (#37) is available, the handler loads a fact from
// catfact.ninja. Until then, orvalho serve still works with an offline fallback.

id: "cat-ssr"
name: "Cat facts SSR"

entry: "worker.js"
runtime: "js"

port: 8787

// Outbound allowlist — enforced once host fetch lands (#37).
egress: [
	"catfact.ninja",
	"https://catfact.ninja",
]

bindings: {
	// Reserved for later env.assets (#35); HTML is self-contained for now.
	assets: {
		root: "assets"
		paths: ["assets/style.css"]
	}
}
