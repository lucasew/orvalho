// Cat facts SSR demo package (hand-written Workers-shaped JS; no Astro).
// Host injects runtime.env; this package projects nothing extra onto agent.env.
// ASSETS binding serves files under assets/ via CF-like env.ASSETS.fetch.

id: "cat-ssr"
name: "Cat facts SSR"

agents: {
	main: {
		entrypoint: "worker.js"
		bindings: {
			ASSETS: {
				type: "assets"
				root: "assets"
			}
		}
	}
}

port: 8787

egress: [
	"catfact.ninja",
	"https://catfact.ninja",
]
