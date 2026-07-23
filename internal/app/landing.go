package app

import (
	"bytes"
	"html/template"
	"net/http"
)

var landingPageTemplate = template.Must(template.New("landing").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="description" content="Seol publishes static HTML pages and returns shareable URLs.">
  <title>Seol — publish an HTML page</title>
  <style>
    :root { color-scheme: light; --paper:#faf7f2; --ink:#201d19; --muted:#6d655c; --line:#ddd5ca; --panel:#f1ece4; --accent:#b84c20; }
    * { box-sizing:border-box; }
    html { font-size:17px; }
    body { margin:0; background:var(--paper); color:var(--ink); font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; line-height:1.6; }
    main, footer { width:min(44rem, calc(100% - 2rem)); margin-inline:auto; }
    main { padding:5rem 0 3rem; }
    header { padding-bottom:3.5rem; border-bottom:1px solid var(--line); }
    h1, h2 { letter-spacing:-.035em; line-height:1.1; }
    h1 { margin:0 0 .75rem; font-size:clamp(2.8rem, 10vw, 5.2rem); }
    h2 { margin:0 0 1rem; font-size:1.5rem; }
    p { margin:.6rem 0; }
    .tagline { max-width:34rem; margin:0; font-size:clamp(1.3rem, 4vw, 1.8rem); }
    .intro { max-width:39rem; margin-top:1.25rem; color:var(--muted); }
    .instance { display:inline-block; margin-top:1rem; }
    section { padding:3rem 0; border-bottom:1px solid var(--line); }
    a { color:var(--accent); text-decoration-thickness:.08em; text-underline-offset:.18em; }
    a:hover { text-decoration-thickness:.14em; }
    a:focus-visible { outline:3px solid var(--accent); outline-offset:4px; border-radius:2px; }
    pre { margin:1rem 0; padding:1.1rem 1.2rem; overflow-x:auto; border:1px solid var(--line); border-radius:.45rem; background:var(--panel); font:0.88rem/1.65 ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace; }
    code { font-family:ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace; }
    :not(pre) > code { padding:.1em .3em; border-radius:.2rem; background:var(--panel); font-size:.9em; }
    .result { color:var(--accent); }
    .facts { display:grid; grid-template-columns:repeat(3, 1fr); gap:1.5rem; }
    .facts h3 { margin:0 0 .35rem; font-size:1rem; }
    .facts p { color:var(--muted); font-size:.94rem; }
    .note { color:var(--muted); font-size:.94rem; }
    footer { padding:1.5rem 0 3rem; color:var(--muted); font-size:.9rem; }
    @media (max-width:38rem) { main { padding-top:3rem; } .facts { grid-template-columns:1fr; gap:.8rem; } section { padding:2.4rem 0; } }
  </style>
</head>
<body>
<main>
  <header>
    <h1>Seol</h1>
    <p class="tagline">Publish an HTML page. Get a shareable URL.</p>
    <p class="intro">A small self-hosted service for static reports, dashboards, demos, documentation, and pages made by AI agents. Upload a file, directory, or ZIP from the command line.</p>
    <a class="instance" href="{{.PublicBaseURL}}">{{.PublicBaseURL}}</a>
  </header>

  <section aria-labelledby="quick-start">
    <h2 id="quick-start">Install and publish</h2>
    <p>On Linux amd64:</p>
    <pre><code>mkdir -p ~/.local/bin
curl -fL https://github.com/semyonfox/seol/releases/latest/download/seol-linux-amd64 \
  -o ~/.local/bin/seol
chmod +x ~/.local/bin/seol</code></pre>
    <p class="note">Other platforms are on <a href="https://github.com/semyonfox/seol/releases/latest">GitHub Releases</a>. With Go installed, use <code>go install github.com/semyonfox/seol/cmd/seol@latest</code>.</p>
    <pre><code>seol configure --server {{.PublicBaseURL}} --token TOKEN
seol publish ./report
<span class="result">Published: {{.PublicBaseURL}}/p/…/</span></code></pre>
    <p>Standalone HTML files work directly. Directories and ZIP archives need an <code>index.html</code> at their root.</p>
    <p class="note">Uploads are limited to 10 MiB compressed, 50 MiB extracted, and 100 archive entries.</p>
  </section>

  <section aria-labelledby="how-it-works">
    <h2 id="how-it-works">How it works</h2>
    <div class="facts">
      <div><h3>Static only</h3><p>HTML, CSS, JavaScript, images, fonts, JSON, SVG, and WASM are served as files. Uploaded server-side code never runs.</p></div>
      <div><h3>Random URLs</h3><p>Each page gets a cryptographically random public link. Publishing uses one configured server token; viewing needs none.</p></div>
      <div><h3>Temporary</h3><p>Pages live for one day by default and at most seven days after their latest update. Expired content is removed automatically.</p></div>
    </div>
  </section>

  <section aria-labelledby="commands">
    <h2 id="commands">The commands</h2>
    <pre><code>seol publish [--title TITLE] [--expires 7d] [--quiet|--json] PATH
seol list
seol stats
seol info PAGE_ID
seol replace PAGE_ID PATH
seol expiry PAGE_ID 3d
seol delete PAGE_ID</code></pre>
    <p class="note"><code>--quiet</code> prints only the URL for scripts and agents. <code>--json</code> returns machine-readable output.</p>
  </section>

  <section aria-labelledby="agents">
    <h2 id="agents">For coding agents</h2>
    <p>Ask Codex to use <code>$skill-installer</code> to install the Seol skill from <a href="https://github.com/semyonfox/seol/tree/main/skills/seol">the repository</a>. Or give any agent this instruction:</p>
    <pre><code>Create a static website in a temporary directory with index.html at its root.
Publish it by running:

    seol publish --quiet DIRECTORY

Return the printed URL. Never publish credentials or private data.</code></pre>
  </section>

  <section aria-labelledby="self-host">
    <h2 id="self-host">Run your own</h2>
    <p>Seol is one Go binary with SQLite metadata and filesystem storage. The included Compose setup can run it with an optional Cloudflare Tunnel sidecar.</p>
    <pre><code>git clone https://github.com/semyonfox/seol.git
cd seol
cp .env.example .env
# Set SEOL_TOKEN in .env, then:
docker compose up --build -d</code></pre>
    <p class="note">See the <a href="https://github.com/semyonfox/seol#quick-start">README</a> for configuration and tunnel setup.</p>
  </section>
</main>
<footer><a href="https://github.com/semyonfox/seol">Source on GitHub</a> · MIT licensed · no accounts, dashboard, or uploaded server-side code.</footer>
</body>
</html>`))

func (s *Server) landingPage(w http.ResponseWriter, _ *http.Request) {
	var page bytes.Buffer
	if err := landingPageTemplate.Execute(&page, struct{ PublicBaseURL string }{s.cfg.PublicBaseURL}); err != nil {
		http.Error(w, "Could not render landing page.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = page.WriteTo(w)
}
