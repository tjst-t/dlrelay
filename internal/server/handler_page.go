package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

func serverURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
			scheme = fwd
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + r.Host
}

// safeServerURL returns the server URL safely escaped for embedding in JS/HTML.
// Uses json.Marshal to escape all special characters (quotes, backslashes, angle brackets, etc.).
func safeServerURL(r *http.Request) string {
	raw := serverURL(r)
	b, _ := json.Marshal(raw)
	// json.Marshal returns `"value"` — strip outer quotes
	return string(b[1 : len(b)-1])
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	base := safeServerURL(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(strings.ReplaceAll(pageHTML, "{{SERVER_URL}}", base)))
}


const pageHTML = `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>DL Relay</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Syne:wght@600;700;800&family=Noto+Sans+JP:wght@400;500;600&display=swap" rel="stylesheet">
<style>
:root {
  --bg: #0a0a10;
  --surface: #141420;
  --surface-2: #1c1c2a;
  --border: #28283a;
  --text: #eaeaf0;
  --text-2: #9898b0;
  --muted: #606078;
  --accent: #e89830;
  --accent-dim: rgba(232, 152, 48, 0.10);
  --accent-hover: #f0b050;
  --green: #30d880;
  --green-dim: rgba(48, 216, 128, 0.15);
  --red: #e85050;
  --blue: #5088f0;
}
*,*::before,*::after { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: "Noto Sans JP", "Hiragino Kaku Gothic ProN", "Yu Gothic UI", system-ui, sans-serif;
  background: var(--bg);
  color: var(--text);
  min-height: 100vh;
  line-height: 1.6;
  overflow-x: hidden;
}
body::after {
  content: "";
  position: fixed;
  inset: 0;
  opacity: 0.018;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.8' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
  pointer-events: none;
  z-index: 10000;
}
.wrap { max-width: 700px; margin: 0 auto; padding: 0 1.5rem; }
a { color: var(--accent); text-decoration: none; }
a:hover { color: var(--accent-hover); }
code {
  font-family: "SF Mono", "Cascadia Code", "Fira Code", "Consolas", monospace;
  background: var(--surface);
  padding: 0.1rem 0.35rem;
  border-radius: 4px;
  font-size: 0.82em;
  border: 1px solid var(--border);
}
.header {
  padding: 1.25rem 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--border);
  margin-bottom: 3rem;
}
.logo {
  font-family: "Syne", system-ui, sans-serif;
  font-weight: 800;
  font-size: 1.15rem;
  letter-spacing: -0.02em;
}
.logo em { font-style: normal; color: var(--accent); }
.logo a { color: inherit; text-decoration: none; }
.logo a:hover { color: inherit; }
.header-right { display: flex; align-items: center; gap: 1.25rem; }
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.78rem;
  color: var(--muted);
}
.status-dot {
  width: 7px; height: 7px;
  border-radius: 50%;
  background: var(--muted);
  transition: all 0.4s;
}
.status-dot.online {
  background: var(--green);
  box-shadow: 0 0 6px var(--green);
  animation: pulse-dot 2s ease-in-out infinite;
}
.header-link {
  font-size: 0.85rem;
  color: var(--text-2);
  text-decoration: none;
  transition: color 0.15s;
}
.header-link:hover { color: var(--accent); }
.header-link-active { color: var(--accent); }
.hero {
  text-align: center;
  padding: 1.5rem 0 2.5rem;
  position: relative;
  animation: fadeUp 0.5s ease-out;
}
.hero::before {
  content: "";
  position: absolute;
  width: 500px; height: 400px;
  background: radial-gradient(ellipse, var(--accent-dim) 0%, transparent 70%);
  top: -100px; left: 50%;
  transform: translateX(-50%);
  pointer-events: none;
  z-index: -1;
}
.hero h1 {
  font-family: "Syne", system-ui, sans-serif;
  font-weight: 800;
  font-size: 2.1rem;
  letter-spacing: -0.03em;
  line-height: 1.35;
  margin-bottom: 0.9rem;
}
.hero-sub {
  color: var(--text-2);
  font-size: 0.92rem;
  max-width: 460px;
  margin: 0 auto 1.5rem;
}
.server-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 999px;
  padding: 0.4rem 1rem;
  font-family: "SF Mono", "Cascadia Code", "Fira Code", monospace;
  font-size: 0.82rem;
  color: var(--text-2);
}
.server-chip .dot {
  width: 6px; height: 6px;
  border-radius: 50%;
  background: var(--green);
  box-shadow: 0 0 6px var(--green);
}
.flow-section {
  padding: 0.5rem 0 2rem;
  animation: fadeUp 0.5s ease-out 0.08s both;
}
.flow {
  display: flex;
  align-items: center;
  justify-content: center;
}
.flow-node {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  min-width: 96px;
}
.flow-icon {
  width: 52px; height: 52px;
  border-radius: 14px;
  background: var(--surface);
  border: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-2);
  transition: all 0.3s;
}
.flow-node.active .flow-icon {
  border-color: var(--accent);
  color: var(--accent);
  box-shadow: 0 0 24px var(--accent-dim), 0 0 0 1px var(--accent);
}
.flow-label {
  font-size: 0.75rem;
  color: var(--muted);
  font-weight: 500;
}
.flow-node.active .flow-label { color: var(--accent); }
.flow-connector {
  flex: 0 0 72px;
  height: 2px;
  background: var(--border);
  position: relative;
  overflow: hidden;
  margin: 0 -2px;
  margin-bottom: 1.5rem;
}
.flow-signal {
  position: absolute;
  width: 14px; height: 2px;
  background: var(--accent);
  top: 0; left: -14px;
  box-shadow: 0 0 8px var(--accent);
  animation: signal 2.5s ease-in-out infinite;
}
.flow-connector:last-of-type .flow-signal {
  animation-delay: 1.2s;
}
.section {
  padding: 1.75rem 0;
  border-top: 1px solid var(--border);
}
.section-title {
  font-family: "Syne", system-ui, sans-serif;
  font-weight: 700;
  font-size: 1.05rem;
  letter-spacing: -0.01em;
  margin-bottom: 1.5rem;
}
.section-setup { animation: fadeUp 0.5s ease-out 0.14s both; }
.section-api { animation: fadeUp 0.5s ease-out 0.20s both; }
.step {
  display: flex;
  gap: 1rem;
  margin-bottom: 1.75rem;
}
.step:last-child { margin-bottom: 0; }
.step-num {
  flex: 0 0 30px;
  width: 30px; height: 30px;
  border-radius: 50%;
  background: var(--accent-dim);
  border: 1px solid rgba(232, 152, 48, 0.2);
  color: var(--accent);
  font-family: "Syne", system-ui, sans-serif;
  font-weight: 700;
  font-size: 0.82rem;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 0.1rem;
}
.step-body { flex: 1; min-width: 0; }
.step-body h3 {
  font-size: 0.92rem;
  font-weight: 600;
  margin-bottom: 0.25rem;
}
.step-body p {
  color: var(--text-2);
  font-size: 0.84rem;
  margin-bottom: 0.6rem;
}
.step-body ol {
  list-style: none;
  counter-reset: sub;
  padding: 0;
}
.step-body ol li {
  counter-increment: sub;
  padding: 0.25rem 0 0.25rem 1.4rem;
  position: relative;
  font-size: 0.84rem;
  color: var(--text-2);
  line-height: 1.55;
}
.step-body ol li::before {
  content: counter(sub) ".";
  position: absolute;
  left: 0;
  color: var(--muted);
  font-weight: 600;
  font-size: 0.8rem;
}
.btn-dl {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.6rem 1.4rem;
  background: var(--accent);
  color: var(--bg);
  text-decoration: none;
  border-radius: 8px;
  font-weight: 600;
  font-size: 0.88rem;
  transition: all 0.2s;
  margin-top: 0.25rem;
}
.btn-dl:hover {
  background: var(--accent-hover);
  color: var(--bg);
  transform: translateY(-1px);
  box-shadow: 0 4px 16px var(--accent-dim);
}
.btn-dl svg { width: 16px; height: 16px; }
.browser-tabs { display: flex; gap: 0; margin: 0.6rem 0; }
.browser-tab {
  padding: 0.3rem 0.8rem;
  background: var(--surface);
  color: var(--muted);
  border: 1px solid var(--border);
  cursor: pointer;
  font-size: 0.78rem;
  font-family: inherit;
  transition: all 0.15s;
}
.browser-tab:first-child { border-radius: 6px 0 0 6px; }
.browser-tab:last-child { border-radius: 0 6px 6px 0; border-left: none; }
.browser-tab.active {
  background: var(--accent-dim);
  color: var(--accent);
  border-color: rgba(232, 152, 48, 0.25);
}
.browser-content { display: none; }
.browser-content.active { display: block; }
.api-table {
  border: 1px solid var(--border);
  border-radius: 10px;
  overflow: hidden;
  background: var(--surface);
}
.api-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.55rem 1rem;
  border-bottom: 1px solid var(--border);
  font-size: 0.82rem;
  transition: background 0.1s;
}
.api-row:last-child { border-bottom: none; }
.api-row:hover { background: var(--surface-2); }
.method {
  font-family: "SF Mono", "Cascadia Code", "Fira Code", monospace;
  font-size: 0.68rem;
  font-weight: 700;
  padding: 0.12rem 0.4rem;
  border-radius: 4px;
  text-transform: uppercase;
  flex: 0 0 48px;
  text-align: center;
  letter-spacing: 0.02em;
}
.method-get { background: rgba(48, 216, 128, 0.10); color: var(--green); }
.method-post { background: rgba(80, 136, 240, 0.10); color: var(--blue); }
.method-del { background: rgba(232, 80, 80, 0.10); color: var(--red); }
.api-path {
  font-family: "SF Mono", "Cascadia Code", "Fira Code", monospace;
  font-size: 0.8rem;
  color: var(--text);
  flex: 1;
}
.api-path a { color: var(--text); text-decoration: none; }
.api-path a:hover { color: var(--accent); }
.api-desc { color: var(--muted); font-size: 0.76rem; flex: 0 0 130px; text-align: right; }
.footer {
  padding: 2rem 0;
  text-align: center;
  color: var(--muted);
  font-size: 0.75rem;
  border-top: 1px solid var(--border);
  margin-top: 0.5rem;
  animation: fadeUp 0.5s ease-out 0.28s both;
}
@keyframes fadeUp {
  from { opacity: 0; transform: translateY(14px); }
  to { opacity: 1; transform: translateY(0); }
}
@keyframes signal {
  0% { left: -14px; opacity: 0; }
  12% { opacity: 1; }
  88% { opacity: 1; }
  100% { left: calc(100% + 14px); opacity: 0; }
}
@keyframes pulse-dot {
  0%, 100% { box-shadow: 0 0 4px var(--green); }
  50% { box-shadow: 0 0 10px var(--green), 0 0 18px rgba(48, 216, 128, 0.2); }
}
@media (max-width: 600px) {
  .header { flex-wrap: wrap; gap: 0.5rem; margin-bottom: 2rem; }
  .header-right { gap: 0.75rem; }
  .hero h1 { font-size: 1.5rem; }
  .flow-connector { flex: 0 0 36px; }
  .flow-node { min-width: 76px; }
  .flow-icon { width: 44px; height: 44px; border-radius: 11px; }
  .flow-icon svg { width: 20px; height: 20px; }
  .step { gap: 0.75rem; }
  .api-desc { display: none; }
  .api-row { padding: 0.45rem 0.7rem; }
}
</style>
</head>
<body>
<div class="wrap">
  <header class="header">
    <div class="logo"><a href="/"><em>DL</em> Relay</a></div>
    <nav class="header-right">
      <a class="header-link" href="{{SERVER_URL}}/">Downloads</a>
      <a class="header-link header-link-active" href="{{SERVER_URL}}/setup">Extension</a>
      <a class="header-link" href="{{SERVER_URL}}/bookmarklet">Bookmarklet</a>
      <div class="status-badge">
        <span class="status-dot" id="status-dot"></span>
        <span id="status-text">...</span>
      </div>
    </nav>
  </header>

  <section class="hero">
    <h1>ブラウザの動画を<br>サーバーに直接保存</h1>
    <p class="hero-sub">拡張機能が動画を自動検出し、このサーバーにダウンロードを指示します。yt-dlp / HLS / HTTP に対応。</p>
    <div class="server-chip">
      <span class="dot"></span>
      <span>{{SERVER_URL}}</span>
    </div>
  </section>

  <section class="flow-section">
    <div class="flow">
      <div class="flow-node">
        <div class="flow-icon">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="14" rx="2"/><path d="M8 21h8M12 17v4"/></svg>
        </div>
        <span class="flow-label">ブラウザ</span>
      </div>
      <div class="flow-connector"><div class="flow-signal"></div></div>
      <div class="flow-node">
        <div class="flow-icon">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></svg>
        </div>
        <span class="flow-label">拡張機能</span>
      </div>
      <div class="flow-connector"><div class="flow-signal"></div></div>
      <div class="flow-node active">
        <div class="flow-icon">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
        </div>
        <span class="flow-label">Relay Server</span>
      </div>
    </div>
  </section>

  <section class="section section-setup">
    <h2 class="section-title">セットアップ</h2>
    <div class="step">
      <div class="step-num">1</div>
      <div class="step-body">
        <h3>拡張機能をダウンロード</h3>
        <p>サーバー URL は設定済みの状態で含まれています</p>
        <a class="btn-dl" href="{{SERVER_URL}}/api/extension.zip">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
          Extension (.zip)
        </a>
      </div>
    </div>
    <div class="step">
      <div class="step-num">2</div>
      <div class="step-body">
        <h3>ブラウザにインストール</h3>
        <div class="browser-tabs">
          <button class="browser-tab active" onclick="switchBrowser('chrome',this)">Chrome / Edge</button>
          <button class="browser-tab" onclick="switchBrowser('firefox',this)">Firefox</button>
        </div>
        <div id="browser-chrome" class="browser-content active">
          <ol>
            <li>ダウンロードした zip を展開する</li>
            <li><code>chrome://extensions</code> を開き「デベロッパー モード」をON</li>
            <li>「パッケージ化されていない拡張機能を読み込む」で展開フォルダを選択</li>
          </ol>
        </div>
        <div id="browser-firefox" class="browser-content">
          <ol>
            <li><code>about:debugging#/runtime/this-firefox</code> を開く</li>
            <li>「一時的なアドオンを読み込む」で zip 内の <code>manifest.json</code> を選択</li>
          </ol>
        </div>
      </div>
    </div>
    <div class="step">
      <div class="step-num">3</div>
      <div class="step-body">
        <h3>動画をダウンロード</h3>
        <p>動画ページでツールバーのアイコンをクリックし、検出された動画を選んでダウンロード</p>
      </div>
    </div>
  </section>

  <section class="section section-api">
    <h2 class="section-title">API</h2>
    <div class="api-table">
      <div class="api-row">
        <span class="method method-get">GET</span>
        <span class="api-path"><a href="{{SERVER_URL}}/api/health">/api/health</a></span>
        <span class="api-desc">ヘルスチェック</span>
      </div>
      <div class="api-row">
        <span class="method method-post">POST</span>
        <span class="api-path">/api/downloads</span>
        <span class="api-desc">ダウンロード開始</span>
      </div>
      <div class="api-row">
        <span class="method method-get">GET</span>
        <span class="api-path"><a href="{{SERVER_URL}}/api/downloads">/api/downloads</a></span>
        <span class="api-desc">ダウンロード一覧</span>
      </div>
      <div class="api-row">
        <span class="method method-get">GET</span>
        <span class="api-path">/api/downloads/{id}</span>
        <span class="api-desc">ダウンロード状況</span>
      </div>
      <div class="api-row">
        <span class="method method-del">DEL</span>
        <span class="api-path">/api/downloads/{id}</span>
        <span class="api-desc">キャンセル</span>
      </div>
      <div class="api-row">
        <span class="method method-post">POST</span>
        <span class="api-path">/api/convert</span>
        <span class="api-desc">FFmpeg 変換</span>
      </div>
      <div class="api-row">
        <span class="method method-post">POST</span>
        <span class="api-path">/api/probe</span>
        <span class="api-desc">メディア情報取得</span>
      </div>
      <div class="api-row">
        <span class="method method-get">GET</span>
        <span class="api-path"><a href="{{SERVER_URL}}/api/codecs">/api/codecs</a></span>
        <span class="api-desc">コーデック一覧</span>
      </div>
      <div class="api-row">
        <span class="method method-get">GET</span>
        <span class="api-path"><a href="{{SERVER_URL}}/api/formats">/api/formats</a></span>
        <span class="api-desc">フォーマット一覧</span>
      </div>
    </div>
  </section>

  <footer class="footer">dlrelay v2.0.0</footer>
</div>
<script>
function switchBrowser(id, el) {
  var contents = document.querySelectorAll(".browser-content");
  var tabs = document.querySelectorAll(".browser-tab");
  for (var i = 0; i < contents.length; i++) contents[i].classList.remove("active");
  for (var i = 0; i < tabs.length; i++) tabs[i].classList.remove("active");
  document.getElementById("browser-" + id).classList.add("active");
  el.classList.add("active");
}
(function() {
  var dot = document.getElementById("status-dot");
  var txt = document.getElementById("status-text");
  fetch("{{SERVER_URL}}/api/health").then(function(r) {
    if (r.ok) { dot.classList.add("online"); txt.textContent = "Online"; }
    else { txt.textContent = "Error"; }
  }).catch(function() { txt.textContent = "Offline"; });
})();
</script>
</body>
</html>`

