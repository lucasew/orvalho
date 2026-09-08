package workers

import (
	"embed"
	"strings"

	"github.com/lucasew/orvalho/pkg/imports"
)

//go:embed nodehost/*.js
var nodehostFS embed.FS

// NodeScriptImports is the require chain for Script run: claimed Node
// builtins plus a stub for Node's test/common helper.
func NodeScriptImports() []imports.Handler[any] {
	return []imports.Handler[any]{
		imports.Alias[any]{From: "assert", To: "node:assert"},
		imports.Alias[any]{From: "async_hooks", To: "node:async_hooks"},
		imports.Alias[any]{From: "buffer", To: "node:buffer"},
		imports.Alias[any]{From: "child_process", To: "node:child_process"},
		imports.Alias[any]{From: "console", To: "node:console"},
		imports.Alias[any]{From: "crypto", To: "node:crypto"},
		imports.Alias[any]{From: "dns", To: "node:dns"},
		imports.Alias[any]{From: "diagnostics_channel", To: "node:diagnostics_channel"},
		imports.Alias[any]{From: "domain", To: "node:domain"},
		imports.Alias[any]{From: "dns/promises", To: "node:dns/promises"},
		imports.Alias[any]{From: "events", To: "node:events"},
		imports.Alias[any]{From: "fs", To: "node:fs"},
		imports.Alias[any]{From: "fs/promises", To: "node:fs/promises"},
		imports.Alias[any]{From: "http", To: "node:http"},
		imports.Alias[any]{From: "http2", To: "node:http2"},
		imports.Alias[any]{From: "https", To: "node:https"},
		imports.Alias[any]{From: "module", To: "node:module"},
		imports.Alias[any]{From: "net", To: "node:net"},
		imports.Alias[any]{From: "os", To: "node:os"},
		imports.Alias[any]{From: "path", To: "node:path"},
		imports.Alias[any]{From: "path/posix", To: "node:path"},
		imports.Alias[any]{From: "node:path/posix", To: "node:path"},
		imports.Alias[any]{From: "path/win32", To: "node:path/win32"},
		imports.Alias[any]{From: "perf_hooks", To: "node:perf_hooks"},
		imports.Alias[any]{From: "process", To: "node:process"},
		imports.Alias[any]{From: "punycode", To: "node:punycode"},
		imports.Alias[any]{From: "querystring", To: "node:querystring"},
		imports.Alias[any]{From: "readline", To: "node:readline"},
		imports.Alias[any]{From: "readline/promises", To: "node:readline/promises"},
		imports.Alias[any]{From: "stream", To: "node:stream"},
		imports.Alias[any]{From: "stream/promises", To: "node:stream/promises"},
		imports.Alias[any]{From: "stream/consumers", To: "node:stream/consumers"},
		imports.Alias[any]{From: "stream/web", To: "node:stream/web"},
		imports.Alias[any]{From: "string_decoder", To: "node:string_decoder"},
		imports.Alias[any]{From: "timers", To: "node:timers"},
		imports.Alias[any]{From: "timers/promises", To: "node:timers/promises"},
		imports.Alias[any]{From: "tls", To: "node:tls"},
		imports.Alias[any]{From: "tty", To: "node:tty"},
		imports.Alias[any]{From: "url", To: "node:url"},
		imports.Alias[any]{From: "util", To: "node:util"},
		imports.Alias[any]{From: "util/types", To: "node:util/types"},
		imports.Alias[any]{From: "v8", To: "node:v8"},
		imports.Alias[any]{From: "worker_threads", To: "node:worker_threads"},
		imports.Alias[any]{From: "zlib", To: "node:zlib"},
		imports.Map[any]{
			"node:assert":              nodehostScript("nodehost/assert.js"),
			"node:async_hooks":         nodehostScript("nodehost/async_hooks.js"),
			"node:buffer":              nodehostScript("nodehost/buffer.js"),
			"node:child_process":       nodeChildBinding{},
			"node:console":             nodehostScript("nodehost/console.js"),
			"node:crypto":              nodeCryptoBinding{},
			"node:diagnostics_channel": nodehostScript("nodehost/diagnostics_channel.js"),
			"node:dns":                 nodeDNSBinding{},
			"node:dns/promises":        nodeDNSPromisesBinding{},
			"node:domain":              nodehostScript("nodehost/domain.js"),
			"node:events":              nodehostScript("nodehost/events.js"),
			"node:fs":                  nodeFSBinding{},
			"node:fs/promises":         nodeFSPromisesBinding{},
			"node:http":                nodeHTTPBinding{},
			"node:http2":               nodeHTTP2Binding{},
			"node:https":               nodeHTTPSBinding{},
			"node:module":              nodeModuleBinding{},
			"node:net":                 nodeNetBinding{},
			"node:os":                  nodeOSBinding{},
			"node:path":                nodehostScript("nodehost/path.js"),
			"node:path/win32":          nodehostScript("nodehost/path_win32.js"),
			"node:perf_hooks":          nodePerfHooksBinding{},
			"node:process":             nodeProcessBinding{},
			"node:punycode":            nodehostScript("nodehost/punycode.js"),
			"node:querystring":         nodehostScript("nodehost/querystring.js"),
			"node:readline":            nodeReadlineBinding{},
			"node:readline/promises":   nodehostScript("nodehost/readline_promises.js"),
			"node:stream":              nodehostScript("nodehost/stream.js"),
			"node:stream/promises":     nodehostScript("nodehost/stream_promises.js"),
			"node:stream/consumers":    nodehostScript("nodehost/stream_consumers.js"),
			"node:stream/web":          nodehostScript("nodehost/stream_web.js"),
			"node:string_decoder":      nodehostScript("nodehost/string_decoder.js"),
			"node:timers":              nodehostScript("nodehost/timers.js"),
			"node:timers/promises":     nodehostScript("nodehost/timers_promises.js"),
			"node:tls":                 nodeTLSBinding{},
			"node:tty":                 nodeTTYBinding{},
			"node:url":                 nodeURLBinding{},
			"node:util":                nodehostScript("nodehost/util.js"),
			"node:util/types":          nodehostScript("nodehost/types.js"),
			"node:v8":                  nodeV8Binding{},
			"node:worker_threads":      nodeWorkerThreadsBinding{},
			"node:zlib":                nodeZlibBinding{},
		},
		imports.Func[any](func(spec string, next imports.Resolver[any]) (any, error) {
			if spec == "workerd" || strings.HasPrefix(spec, "@cloudflare/workerd-") {
				return nodehostScript("nodehost/workerd_stub.js"), nil
			}
			if spec == "common" || strings.HasSuffix(spec, "/common") || strings.HasSuffix(spec, "/common/index.js") {
				return nodehostScript("nodehost/common.js"), nil
			}
			return next(spec)
		}),
	}
}

func nodehostScript(name string) imports.Script {
	data, err := nodehostFS.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return imports.Script{Source: string(data), File: name}
}
