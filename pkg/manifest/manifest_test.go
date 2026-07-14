package manifest_test

import (
	"errors"
	"strings"
	"testing"

	"orvalho/pkg/manifest"
)

func TestParseMinimalValid(t *testing.T) {
	raw := []byte(`{
		"schema_version": 1,
		"id": "hello",
		"entry": "worker.js",
		"runtime": "js"
	}`)
	m, err := manifest.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m.ID != "hello" {
		t.Errorf("ID = %q", m.ID)
	}
	if m.Entry != "worker.js" {
		t.Errorf("Entry = %q", m.Entry)
	}
	if m.Runtime != manifest.RuntimeJS {
		t.Errorf("Runtime = %q", m.Runtime)
	}
	if m.SchemaVersion != manifest.SchemaVersionCurrent {
		t.Errorf("SchemaVersion = %d", m.SchemaVersion)
	}
	if m.PreferredPort() != 0 {
		t.Errorf("PreferredPort = %d, want 0", m.PreferredPort())
	}
}

func TestParseFullValid(t *testing.T) {
	raw := []byte(`{
		"schema_version": 1,
		"id": "cat-ssr",
		"name": "Cat SSR Demo",
		"entry": "dist/worker.js",
		"runtime": "js",
		"bindings": {
			"assets": {
				"root": "dist/client",
				"paths": ["dist/client/favicon.ico"]
			},
			"secrets": [
				{ "name": "CAT_API_KEY", "required": true }
			],
			"config": [
				{ "name": "SITE_TITLE" },
				{ "name": "THEME", "required": false }
			]
		},
		"egress": [
			"api.thecatapi.com",
			"*.cdn.example.com",
			"https://api.example.org"
		],
		"port": 8080,
		"publish": {
			"port": 80,
			"protocol": "http"
		}
	}`)
	m, err := manifest.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m.ID != "cat-ssr" {
		t.Errorf("ID = %q", m.ID)
	}
	if m.Name != "Cat SSR Demo" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Bindings == nil || m.Bindings.Assets == nil {
		t.Fatal("expected assets binding")
	}
	if m.Bindings.Assets.Root != "dist/client" {
		t.Errorf("assets.root = %q", m.Bindings.Assets.Root)
	}
	if len(m.Bindings.Secrets) != 1 || m.Bindings.Secrets[0].Name != "CAT_API_KEY" || !m.Bindings.Secrets[0].Required {
		t.Errorf("secrets = %+v", m.Bindings.Secrets)
	}
	if len(m.Bindings.Config) != 2 {
		t.Errorf("config len = %d", len(m.Bindings.Config))
	}
	if len(m.Egress) != 3 {
		t.Errorf("egress len = %d", len(m.Egress))
	}
	if m.Port != 8080 {
		t.Errorf("port = %d", m.Port)
	}
	if m.PreferredPort() != 80 {
		t.Errorf("PreferredPort = %d, want 80 (publish.port)", m.PreferredPort())
	}
	if m.Publish == nil || m.Publish.Protocol != manifest.ProtocolHTTP {
		t.Errorf("publish = %+v", m.Publish)
	}
}

func TestParseValidFixtures(t *testing.T) {
	fixtures := []struct {
		name string
		raw  string
	}{
		{
			name: "port only",
			raw: `{
				"schema_version": 1,
				"id": "svc-a",
				"entry": "index.js",
				"runtime": "js",
				"port": 1
			}`,
		},
		{
			name: "publish only",
			raw: `{
				"schema_version": 1,
				"id": "svc-b",
				"entry": "main.mjs",
				"runtime": "js",
				"publish": { "port": 65535 }
			}`,
		},
		{
			name: "assets paths only",
			raw: `{
				"schema_version": 1,
				"id": "assets-only",
				"entry": "a.js",
				"runtime": "js",
				"bindings": { "assets": { "paths": ["static/x.txt"] } }
			}`,
		},
		{
			name: "empty egress omitted",
			raw: `{
				"schema_version": 1,
				"id": "offline",
				"entry": "w.js",
				"runtime": "js"
			}`,
		},
		{
			name: "https origin allowlist",
			raw: `{
				"schema_version": 1,
				"id": "fetchy",
				"entry": "w.js",
				"runtime": "js",
				"egress": ["https://api.github.com", "http://localhost.localdomain"]
			}`,
		},
	}
	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := manifest.Parse([]byte(tc.raw)); err != nil {
				t.Fatalf("Parse: %v", err)
			}
		})
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantSubstr string
	}{
		{
			name:       "empty",
			raw:        "",
			wantSubstr: "empty",
		},
		{
			name:       "not json",
			raw:        "nope",
			wantSubstr: "parse",
		},
		{
			name:       "unknown field",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","extra":true}`,
			wantSubstr: "parse",
		},
		{
			name:       "trailing data",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js"}{}`,
			wantSubstr: "trailing",
		},
		{
			name:       "missing schema_version",
			raw:        `{"id":"x","entry":"a.js","runtime":"js"}`,
			wantSubstr: "schema_version",
		},
		{
			name:       "bad schema_version",
			raw:        `{"schema_version":99,"id":"x","entry":"a.js","runtime":"js"}`,
			wantSubstr: "schema_version",
		},
		{
			name:       "missing id",
			raw:        `{"schema_version":1,"entry":"a.js","runtime":"js"}`,
			wantSubstr: "id",
		},
		{
			name:       "id uppercase",
			raw:        `{"schema_version":1,"id":"Hello","entry":"a.js","runtime":"js"}`,
			wantSubstr: "id",
		},
		{
			name:       "id starts with digit",
			raw:        `{"schema_version":1,"id":"1abc","entry":"a.js","runtime":"js"}`,
			wantSubstr: "id",
		},
		{
			name:       "id trailing hyphen",
			raw:        `{"schema_version":1,"id":"abc-","entry":"a.js","runtime":"js"}`,
			wantSubstr: "id",
		},
		{
			name:       "id double hyphen",
			raw:        `{"schema_version":1,"id":"ab--c","entry":"a.js","runtime":"js"}`,
			wantSubstr: "id",
		},
		{
			name:       "missing entry",
			raw:        `{"schema_version":1,"id":"x","runtime":"js"}`,
			wantSubstr: "entry",
		},
		{
			name:       "entry absolute",
			raw:        `{"schema_version":1,"id":"x","entry":"/abs.js","runtime":"js"}`,
			wantSubstr: "entry",
		},
		{
			name:       "entry parent",
			raw:        `{"schema_version":1,"id":"x","entry":"../escape.js","runtime":"js"}`,
			wantSubstr: "entry",
		},
		{
			name:       "entry backslash",
			raw:        `{"schema_version":1,"id":"x","entry":"dir\\w.js","runtime":"js"}`,
			wantSubstr: "entry",
		},
		{
			name:       "missing runtime",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js"}`,
			wantSubstr: "runtime",
		},
		{
			name:       "runtime wasm",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"wasm"}`,
			wantSubstr: "runtime",
		},
		{
			name:       "port zero explicit ok via omit; port out of range",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","port":70000}`,
			wantSubstr: "port",
		},
		{
			name:       "port negative",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","port":-1}`,
			wantSubstr: "port",
		},
		{
			name:       "publish protocol bad",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","publish":{"protocol":"https"}}`,
			wantSubstr: "publish.protocol",
		},
		{
			name:       "assets empty",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","bindings":{"assets":{}}}`,
			wantSubstr: "bindings.assets",
		},
		{
			name:       "assets root parent",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","bindings":{"assets":{"root":"foo/.."}}}`,
			wantSubstr: "bindings.assets.root",
		},
		{
			name:       "secret missing name",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","bindings":{"secrets":[{}]}}`,
			wantSubstr: "bindings.secrets",
		},
		{
			name:       "secret bad name",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","bindings":{"secrets":[{"name":"1bad"}]}}`,
			wantSubstr: "bindings.secrets",
		},
		{
			name:       "duplicate secret",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","bindings":{"secrets":[{"name":"K"},{"name":"K"}]}}`,
			wantSubstr: "duplicate",
		},
		{
			name:       "secret and config clash",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","bindings":{"secrets":[{"name":"K"}],"config":[{"name":"K"}]}}`,
			wantSubstr: "both secrets and config",
		},
		{
			name:       "egress bare star",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","egress":["*"]}`,
			wantSubstr: "open proxy",
		},
		{
			name:       "egress empty entry",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","egress":[""]}`,
			wantSubstr: "egress[0]",
		},
		{
			name:       "egress path",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","egress":["example.com/api"]}`,
			wantSubstr: "path",
		},
		{
			name:       "egress ip",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","egress":["1.2.3.4"]}`,
			wantSubstr: "IP",
		},
		{
			name:       "egress bad scheme",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","egress":["ftp://example.com"]}`,
			wantSubstr: "scheme",
		},
		{
			name:       "egress multi wildcard",
			raw:        `{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","egress":["*.*.example.com"]}`,
			wantSubstr: "wildcard",
		},
		{
			name:       "name control char",
			raw:        `{"schema_version":1,"id":"x","name":"bad\nname","entry":"a.js","runtime":"js"}`,
			wantSubstr: "name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := manifest.Parse([]byte(tc.raw))
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestParsePublishPortInvalid(t *testing.T) {
	raw := []byte(`{"schema_version":1,"id":"x","entry":"a.js","runtime":"js","publish":{"port":70000}}`)
	_, err := manifest.Parse(raw)
	if err == nil || !strings.Contains(err.Error(), "publish.port") {
		t.Fatalf("got %v", err)
	}
}

func TestValidationErrorMulti(t *testing.T) {
	// Missing several required fields at once.
	raw := []byte(`{"schema_version":1}`)
	_, err := manifest.Parse(raw)
	if err == nil {
		t.Fatal("expected error")
	}
	var ve *manifest.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %T %v", err, err)
	}
	if len(ve.Errors) < 2 {
		t.Fatalf("want multiple field errors, got %v", ve.Errors)
	}
	msg := err.Error()
	for _, sub := range []string{"id", "entry", "runtime"} {
		if !strings.Contains(msg, sub) {
			t.Errorf("error %q missing %q", msg, sub)
		}
	}
}

func TestPreferredPortFallback(t *testing.T) {
	m := &manifest.Manifest{Port: 9}
	if m.PreferredPort() != 9 {
		t.Fatalf("got %d", m.PreferredPort())
	}
	m.Publish = &manifest.PublishHints{Port: 11}
	if m.PreferredPort() != 11 {
		t.Fatalf("got %d", m.PreferredPort())
	}
	var nilM *manifest.Manifest
	if nilM.PreferredPort() != 0 {
		t.Fatalf("nil PreferredPort")
	}
}

func TestMustParsePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	manifest.MustParse([]byte(`{}`))
}

func TestFilenameConstant(t *testing.T) {
	if manifest.Filename != "orvalho.json" {
		t.Fatalf("Filename = %q", manifest.Filename)
	}
}

func TestString(t *testing.T) {
	m := manifest.MustParse([]byte(`{"schema_version":1,"id":"x","entry":"a.js","runtime":"js"}`))
	s := m.String()
	if !strings.Contains(s, "x") || !strings.Contains(s, "a.js") {
		t.Fatalf("String = %q", s)
	}
}

func TestWhitespaceNormalized(t *testing.T) {
	raw := []byte(`{
		"schema_version": 1,
		"id": "  hello  ",
		"entry": "  worker.js  ",
		"runtime": "  js  ",
		"egress": ["  api.example.com  "]
	}`)
	m, err := manifest.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "hello" || m.Entry != "worker.js" || m.Runtime != "js" {
		t.Fatalf("not trimmed: %+v", m)
	}
	if m.Egress[0] != "api.example.com" {
		t.Fatalf("egress not trimmed: %q", m.Egress[0])
	}
}
