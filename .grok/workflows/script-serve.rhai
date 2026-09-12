let meta = #{
    name: "script-serve",
    description: "Keep script-run isolate up, rod GET /, fix host bugs",
    when_to_use: "Rod pesquisarr under orvalho script run and fix isolate failures",
    phases: [
        #{ title: "Server", detail: "one process on :4321" },
        #{ title: "Test", detail: "rod GET / only" },
        #{ title: "Implement", detail: "orvalho host only if the test failed" },
    ],
};

let report_schema = #{
    "type": "object",
    "properties": #{
        "ok": #{ "type": "boolean" },
        "detail": #{ "type": "string" },
    },
};

let worktree = if args == () { () } else { args.worktree };
let app = if args == () { () } else { args.app };
if worktree == () { pause("verification", "Pass args.worktree — orvalho worktree path."); }
if app == () { pause("verification", "Pass args.app — guest app path (pesquisarr)."); }

phase("Server");
let server_prompt = "";
server_prompt += "You only manage the orvalho isolate. Worktree: ";
server_prompt += worktree;
server_prompt += " Guest app: ";
server_prompt += app;
server_prompt += ". Binary is /tmp/orvalho. If 127.0.0.1:4321 is not listening, from the guest app run: ";
server_prompt += "ASTRO_ADAPTER=node SENTRY_DISABLED=1 /tmp/orvalho --pprof-dir /tmp/orvalho-dev/pprof -v script run ";
server_prompt += "node_modules/.orvalho/astro@6.4.8/node_modules/astro/bin/astro.mjs -- dev >> /tmp/orvalho-dev/server.log 2>&1. ";
server_prompt += "Do not GET / yourself. Do not edit guest app files. Rebuild /tmp/orvalho from the worktree if the binary is missing. ";
server_prompt += "Return {ok, detail} with the isolate pid and listen address.";
let server = agent(server_prompt, #{
    label: "server",
    capability_mode: "execute",
    output_schema: report_schema,
});
if server == () || !server.success {
    complete(#{ ok: false, step: "server", detail: "server agent failed" });
}

phase("Test");
let test_prompt = "";
test_prompt += "Rod http://127.0.0.1:4321/ the simplest way: search_tool then use_tool for rod. ";
test_prompt += "Navigate to that URL. Wait until the document has a search form or 180s. ";
test_prompt += "Do not request /node_modules/.vite/deps/lucide-svelte.js without a ?v= query. ";
test_prompt += "Do not edit files. Return {ok: true} only if GET / rendered a useful page (status 200 and visible search UI). ";
test_prompt += "Otherwise {ok: false, detail: what you saw}.";
let test = agent(test_prompt, #{
    label: "tester",
    capability_mode: "execute",
    output_schema: report_schema,
});
if test == () || !test.success {
    complete(#{ ok: false, step: "test", detail: "tester agent failed" });
}
if test.output == () || test.output.ok == true {
    complete(#{ ok: true, detail: "GET / ok" });
}

phase("Implement");
let impl_prompt = "";
impl_prompt += "The rod tester failed: ";
if test.output != () && test.output.detail != () {
    impl_prompt += json_encode(test.output.detail);
} else {
    impl_prompt += "unknown";
}
impl_prompt += ". Worktree: ";
impl_prompt += worktree;
impl_prompt += ". Read /tmp/orvalho-dev/server.log. Fix only orvalho host Bindings or the isolate loop in that worktree. ";
impl_prompt += "Do not change the guest app. Do not add guest workarounds. Official tests: ";
impl_prompt += "go test ./pkg/workers/ ./cmd/orvalho/ ./pkg/imports/ ./pkg/workers/bundle/ -count=1 -timeout 300s -skip TestLoadVite. ";
impl_prompt += "Return {ok, detail}.";
let impl = agent(impl_prompt, #{
    label: "implementer",
    capability_mode: "all",
    output_schema: report_schema,
});
if impl == () || !impl.success {
    complete(#{ ok: false, step: "implement", detail: "implementer agent failed" });
}
complete(#{ ok: false, step: "implement", detail: "tester failed; implementer ran" });
