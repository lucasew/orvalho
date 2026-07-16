id: "with-assets"

agents: {
	main: {
		entrypoint: "index.js"
		bindings: {
			ASSETS: {
				type: "assets"
				root: "assets"
				paths: ["assets/logo.txt", "assets/style.css"]
			}
		}
	}
}
