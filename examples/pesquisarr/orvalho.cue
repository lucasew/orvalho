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

// Search engines + scraped result hosts (open enough for the demo).
// Without these, SSR search never calls outbound fetch (egress deny).
egress: [
	"www.google.com",
	"google.com",
	"www.google.com.br",
	"html.duckduckgo.com",
	"duckduckgo.com",
	"www.duckduckgo.com",
	"yandex.com",
	"www.yandex.com",
	"yandex.com.br",
	// Common torrent index hosts returned by search scrapers
	"*.thepiratebay.org",
	"thepiratebay.org",
	"*.1337x.to",
	"1337x.to",
	"*.nyaa.si",
	"nyaa.si",
	"*.torrentgalaxy.to",
	"torrentgalaxy.to",
	"*.limetorrents.lol",
	"limetorrents.lol",
	"*.magnetdl.com",
	"magnetdl.com",
	"*.torlock.com",
	"torlock.com",
	"*.zooqle.com",
	"zooqle.com",
	"*.rarbg.to",
	"rarbg.to",
	"*.kickasstorrents.to",
	"kickasstorrents.to",
]
