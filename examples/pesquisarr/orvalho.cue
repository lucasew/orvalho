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

// Explicit open egress for search scrapers (any http(s) host).
// Empty egress = deny all; "*" must be declared here.
egress: ["*"]
