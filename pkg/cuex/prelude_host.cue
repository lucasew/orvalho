// Host runtime instance schema (orvalho.cue under --data-dir or --config).
// dataDir is never in CUE — always a CLI argument.

role?: "manager" | "worker"

identity?: {
	// Path to manager private key PEM (absolute or relative to process cwd).
	keyPath?: string
}

// Always present; listen defaults to localhost admin port.
http: {
	listen: string | *"127.0.0.1:7840"
}
