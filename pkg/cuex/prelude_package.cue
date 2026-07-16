// Package instance schema (zip root orvalho.cue).
// User instance unifies with this prelude (+ common).
//
// Policy: SPEC.md — runtime.env is outside-world map[string]string;
// agents map workers; agent.env is CF-style string bag; bindings are
// typed host drivers (not one binding per env var).

schema_version: 1

id: #ID

name?: string

// Outside world → package. Host unifies concrete values at serve/install.
runtime: {
	env: {[string]: string}
} | *{
	env: {}
}

// Workers in this package. Serve requires exactly one agent for now.
agents: [string]: #Agent

#Agent: {
	entrypoint: #RelPath

	// String properties on guest env (CF vars/secrets shape). Map, not list.
	env: {[string]: string} | *{}

	// Named host drivers (assets, later devices, …).
	bindings?: [string]: #Binding
}

// Open binding object: type selects host driver; remaining fields are driver-specific.
#Binding: {
	type: string & =~"^[a-z]([a-z0-9_-]*[a-z0-9])?$"
	...
}

egress?: [...#Egress]

port?: #Port

publish?: {
	port?:     #Port
	protocol?: "http" | *"http"
}
