// Bridge package: Astro/CF pesquisarr dist under orvalho serve.
// Rebuild: copy from ../pesquisarr/dist/{server,client} (see README).

id: "pesquisarr"
name: "Pesquisarr (CF adapter dist)"

agents: {
	main: {
		entrypoint: "entry.mjs"
		bindings: {
			ASSETS: {
				type: "assets"
				root: "assets"
			}
		}
	}
}

port: 8788

// Deny outbound by default in demo; open as needed for live features.
egress: []
