// Package instance schema (zip root orvalho.cue).
// User instance unifies with this prelude (+ common).

schema_version: 1

id: #ID

name?: string

entry: #RelPath

runtime: #Runtime

bindings?: {
	assets?: {
		root?:  #RelPath
		paths?: [...#RelPath]
	}
	secrets?: [...#NameBinding]
	config?:  [...#NameBinding]
}

egress?: [...#Egress]

port?: #Port

publish?: {
	port?:     #Port
	protocol?: "http" | *"http"
}
