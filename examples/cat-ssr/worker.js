// Cat facts SSR — Workers-shaped default export.
//
// Prefer live upstream when host provides fetch (issue #37).
// Otherwise return a polished offline page so `orvalho serve` is usable today.
//
// JSON is parsed via text() + JSON.parse (Response.json not required).

var CAT_FACT_URL = "https://catfact.ninja/fact";
var FALLBACK_FACT =
  "Cats sleep 12–16 hours a day. (offline scaffold — outbound host fetch not wired yet)";

function escapeHTML(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function pageHTML(fact, source, detail) {
  var live = source !== "fallback";
  var badgeClass = live ? "live" : "fallback";
  var badgeLabel = live ? "live · " + source : "fallback";
  var detailLine = detail
    ? "<footer>" + escapeHTML(detail) + "</footer>"
    : "";

  // Inline critical CSS so the page looks fine before env.assets (#35).
  var css =
    ":root{color-scheme:light dark;--bg:#0f1419;--fg:#e7ecf1;--muted:#8b9aab;--card:#1a2332;--accent:#f4a261;--ok:#2a9d8f;--warn:#e9c46a;font-family:Segoe UI,system-ui,sans-serif}" +
    "body{margin:0;min-height:100vh;background:radial-gradient(1200px 600px at 10% -10%,#1b3a4b,var(--bg));color:var(--fg);display:grid;place-items:center;padding:2rem}" +
    "main{width:min(36rem,100%);background:var(--card);border:1px solid #2c3e50;border-radius:1rem;padding:1.75rem 1.5rem;box-shadow:0 20px 50px rgba(0,0,0,.35)}" +
    "h1{margin:0 0 .25rem;font-size:1.35rem;font-weight:650}" +
    ".meta{color:var(--muted);font-size:.85rem;margin-bottom:1.25rem}" +
    "blockquote{margin:0;padding:1rem 1.1rem;border-left:4px solid var(--accent);background:rgba(244,162,97,.08);border-radius:0 .5rem .5rem 0;font-size:1.15rem;line-height:1.45}" +
    ".badge{display:inline-block;margin-top:1.25rem;padding:.2rem .55rem;border-radius:999px;font-size:.75rem;font-weight:600;letter-spacing:.04em;text-transform:uppercase}" +
    ".badge.live{background:rgba(42,157,143,.2);color:var(--ok)}" +
    ".badge.fallback{background:rgba(233,196,106,.15);color:var(--warn)}" +
    "footer{margin-top:1.5rem;color:var(--muted);font-size:.8rem}";

  return (
    "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\">" +
    "<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
    "<title>Cat fact · Orvalho</title>" +
    "<style>" +
    css +
    "</style></head><body><main>" +
    "<h1>🐱 Cat fact</h1>" +
    "<p class=\"meta\">Orvalho reference workload (hand-written worker)</p>" +
    "<blockquote>" +
    escapeHTML(fact) +
    "</blockquote>" +
    "<span class=\"badge " +
    badgeClass +
    "\">" +
    escapeHTML(badgeLabel) +
    "</span>" +
    detailLine +
    "</main></body></html>"
  );
}

async function loadFact() {
  if (typeof fetch !== "function") {
    return {
      fact: FALLBACK_FACT,
      source: "fallback",
      detail: "global fetch is undefined — install outbound fetch (#37) for live facts",
    };
  }
  try {
    var res = await fetch(CAT_FACT_URL);
    // Host Response may expose status + text(); avoid res.json() dependency.
    var status = res.status;
    var body = await res.text();
    if (status < 200 || status > 299) {
      throw new Error("upstream HTTP " + status);
    }
    var data = JSON.parse(body);
    if (!data || !data.fact) {
      throw new Error("unexpected JSON shape");
    }
    return { fact: data.fact, source: "catfact.ninja", detail: "" };
  } catch (e) {
    return {
      fact: FALLBACK_FACT,
      source: "fallback",
      detail: "live fetch failed: " + String(e && e.message ? e.message : e),
    };
  }
}

export default {
  async fetch(request, env, ctx) {
    var loaded = await loadFact();
    var html = pageHTML(loaded.fact, loaded.source, loaded.detail);
    return new Response(html, {
      status: 200,
      headers: {
        "Content-Type": "text/html; charset=utf-8",
        "Cache-Control": "no-store",
      },
    });
  },
};
