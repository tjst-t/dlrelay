package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tjst-t/dlrelay/internal/version"
)

// safeAPIKey returns the API key safely escaped for embedding in JS strings.
func safeAPIKey(key string) string {
	b, _ := json.Marshal(key)
	return string(b[1 : len(b)-1])
}

func (s *Server) handleBookmarkletPage(w http.ResponseWriter, r *http.Request) {
	base := safeServerURL(r)
	key := safeAPIKey(s.apiKey)
	html := strings.ReplaceAll(bookmarkletHTML, "{{SERVER_URL}}", base)
	html = strings.ReplaceAll(html, "{{API_KEY}}", key)
	html = strings.ReplaceAll(html, "{{VERSION}}", version.Version)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

const bookmarkletHTML = `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>DL Relay — Bookmarklet</title>
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
  --red-dim: rgba(232, 80, 80, 0.12);
  --blue: #5088f0;
  --yellow: #f0b030;
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
.wrap { max-width: 960px; margin: 0 auto; padding: 0 1.5rem; }
a { color: var(--accent); text-decoration: none; }
a:hover { color: var(--accent-hover); }
code { font-family: "SF Mono", "Cascadia Code", "Fira Code", monospace; }
.header {
  padding: 1.25rem 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--border);
  margin-bottom: 1.5rem;
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
  padding: 1rem 0 2.5rem;
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
  margin: 0 auto;
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
.section-bm { animation: fadeUp 0.5s ease-out 0.08s both; }
.section-setup { animation: fadeUp 0.5s ease-out 0.14s both; }
.section-features { animation: fadeUp 0.5s ease-out 0.20s both; }
.bm-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 1.5rem;
  position: relative;
  overflow: hidden;
}
.bm-card::before {
  content: "";
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 2px;
  background: linear-gradient(90deg, transparent, var(--accent), transparent);
}
.bm-methods {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
  margin-top: 1.25rem;
}
.bm-method {
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 1rem;
  transition: all 0.2s;
}
.bm-method:hover {
  border-color: rgba(232, 152, 48, 0.3);
}
.bm-method-label {
  font-size: 0.72rem;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  margin-bottom: 0.6rem;
  font-weight: 600;
}
.bm-link {
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
}
.bm-link:hover {
  background: var(--accent-hover);
  color: var(--bg);
  transform: translateY(-1px);
  box-shadow: 0 4px 16px var(--accent-dim);
}
.bm-link svg { width: 16px; height: 16px; }
.copy-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  background: var(--surface-2);
  border: 1px solid var(--border);
  color: var(--text-2);
  padding: 0.5rem 1rem;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.82rem;
  font-family: inherit;
  transition: all 0.2s;
}
.copy-btn:hover { border-color: var(--accent); color: var(--accent); }
.copy-btn.copied { border-color: var(--green); color: var(--green); background: var(--green-dim); }
.copy-btn svg { width: 14px; height: 14px; }
.code-block {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 0.75rem 1rem;
  overflow-x: auto;
  font-family: "SF Mono", "Cascadia Code", "Fira Code", monospace;
  font-size: 0.72rem;
  line-height: 1.4;
  word-break: break-all;
  white-space: pre-wrap;
  margin-top: 0.75rem;
  max-height: 100px;
  overflow-y: auto;
  color: var(--muted);
}
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
}
.feature-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem;
}
.feature-item {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 1rem;
  transition: background 0.15s;
}
.feature-item:hover { background: var(--surface-2); }
.feature-icon {
  width: 32px; height: 32px;
  border-radius: 8px;
  background: var(--accent-dim);
  border: 1px solid rgba(232, 152, 48, 0.15);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--accent);
  margin-bottom: 0.6rem;
}
.feature-icon svg { width: 16px; height: 16px; }
.feature-name {
  font-size: 0.84rem;
  font-weight: 600;
  margin-bottom: 0.2rem;
}
.feature-desc {
  font-size: 0.76rem;
  color: var(--muted);
}
.footer {
  padding: 2rem 0;
  text-align: center;
  color: var(--muted);
  font-size: 0.75rem;
  border-top: 1px solid var(--border);
  margin-top: 1.5rem;
}
@keyframes fadeUp {
  from { opacity: 0; transform: translateY(14px); }
  to { opacity: 1; transform: translateY(0); }
}
@media (max-width: 600px) {
  .header { flex-wrap: wrap; gap: 0.5rem; }
  .header-right { gap: 0.75rem; }
  .hero h1 { font-size: 1.5rem; }
  .bm-methods { grid-template-columns: 1fr; }
  .feature-grid { grid-template-columns: 1fr; }
  .step { gap: 0.75rem; }
}
</style>
</head>
<body>
<div class="wrap">
  <header class="header">
    <div class="logo"><a href="{{SERVER_URL}}/"><em>DL</em> Relay</a></div>
    <nav class="header-right">
      <a class="header-link" href="{{SERVER_URL}}/">Downloads</a>
      <a class="header-link" href="{{SERVER_URL}}/setup">Extension</a>
      <a class="header-link header-link-active" href="{{SERVER_URL}}/bookmarklet">Bookmarklet</a>
    </nav>
  </header>

  <section class="hero">
    <h1>モバイルでも<br>動画をダウンロード</h1>
    <p class="hero-sub">ブックマークレットを使えば、拡張機能が使えないモバイルブラウザからでも動画をサーバーに保存できます。</p>
  </section>

  <section class="section section-bm">
    <h2 class="section-title">ブックマークレット</h2>
    <div class="bm-card">
      <p style="color:var(--text-2);font-size:0.84rem;margin-bottom:0.5rem">ブックマークに登録して、動画のあるページで実行するとダウンロードを開始します。</p>
      <div class="bm-methods">
        <div class="bm-method">
          <div class="bm-method-label">方法1: ドラッグ</div>
          <p style="color:var(--text-2);font-size:0.8rem;margin-bottom:0.75rem">ブックマークバーにドラッグ（<code style="border:none;background:none;padding:0;font-size:inherit">javascript:</code> が保持される）</p>
          <a class="bm-link" href="javascript:void(0)" id="bookmarklet-link" onclick="event.preventDefault();alert('このリンクをブックマークバーにドラッグ&ドロップしてください。\nまたは右の Copy ボタンでコードをコピーして、ブックマークのURLに貼り付けてください。')">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 21l-7-5-7 5V5a2 2 0 012-2h10a2 2 0 012 2z"/></svg>
            DL Relay
          </a>
        </div>
        <div class="bm-method">
          <div class="bm-method-label">方法2: コピー</div>
          <p style="color:var(--text-2);font-size:0.8rem;margin-bottom:0.75rem">コードをコピーしてブックマークURLに貼り付け</p>
          <button class="copy-btn" id="copy-btn" onclick="copyCode()">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>
            Copy to clipboard
          </button>
        </div>
      </div>
      <div class="code-block" id="code-display"></div>
    </div>
  </section>

  <section class="section section-setup">
    <h2 class="section-title">設定手順</h2>
    <div class="step">
      <div class="step-num">1</div>
      <div class="step-body">
        <h3>ブックマークを追加</h3>
        <p>まず適当なページをブックマークに追加する（またはブックマークマネージャで新規作成）</p>
      </div>
    </div>
    <div class="step">
      <div class="step-num">2</div>
      <div class="step-body">
        <h3>URLを置き換え</h3>
        <p>ブックマークを編集し、URL欄に上でコピーしたコードを貼り付ける</p>
        <div style="background:var(--surface);border:1px solid rgba(232,152,48,0.25);border-radius:6px;padding:0.7rem 0.9rem;margin-top:0.6rem;font-size:0.8rem;color:var(--accent)">
          <strong style="display:block;margin-bottom:0.3rem">注意: <code style="border:none;background:none;padding:0;font-size:inherit">javascript:</code> が消える場合</strong>
          <span style="color:var(--text-2)">Chromium系ブラウザ（Vivaldi, Chrome, Edge）はセキュリティ上の理由でペースト時に <code style="border:none;background:none;padding:0;font-size:inherit">javascript:</code> を削除します。貼り付けた後、先頭に <code style="border:none;background:none;padding:0;font-size:inherit">javascript:</code> を手入力で追加してください。</span>
        </div>
      </div>
    </div>
    <div class="step">
      <div class="step-num">3</div>
      <div class="step-body">
        <h3>動画をダウンロード</h3>
        <p>動画のあるページでブックマークをクリック/タップして実行</p>
      </div>
    </div>
  </section>

  <section class="section section-features">
    <h2 class="section-title">対応機能</h2>
    <div class="feature-grid">
      <div class="feature-item">
        <div class="feature-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
        </div>
        <div class="feature-name">yt-dlp</div>
        <div class="feature-desc">YouTube, X, Instagram, TikTok, ニコニコ動画など多数のサイトに対応</div>
      </div>
      <div class="feature-item">
        <div class="feature-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="23 7 16 12 23 17 23 7"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg>
        </div>
        <div class="feature-name">DOM 検出</div>
        <div class="feature-desc">&lt;video&gt; / &lt;source&gt; タグから動画URLを自動検出</div>
      </div>
      <div class="feature-item">
        <div class="feature-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
        </div>
        <div class="feature-name">HLS / DASH</div>
        <div class="feature-desc">.m3u8 / .mpd ストリーミングURLを検出してダウンロード</div>
      </div>
      <div class="feature-item">
        <div class="feature-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>
        </div>
        <div class="feature-name">複数選択</div>
        <div class="feature-desc">検出した動画を一覧から選んでダウンロード</div>
      </div>
    </div>
  </section>

  <footer class="footer">dlrelay {{VERSION}}</footer>
</div>

<script>
var SERVER = "{{SERVER_URL}}";
var APIKEY = "{{API_KEY}}";

var bmCode = (function() {
  var src = '(function(){' +
    'if(document.getElementById("dl-bm")){document.getElementById("dl-bm").remove();return;}' +
    'var S="' + SERVER.replace(/\\/g,'\\\\').replace(/"/g,'\\"') + '",' +
    'K="' + APIKEY.replace(/\\/g,'\\\\').replace(/"/g,'\\"') + '";' +

    // Prevent running on the DL Relay server itself
    'if(location.href.indexOf(S)===0){' +
      'alert("\\u26a0 DL Relay\\n\\n\\u3053\\u306e\\u30d6\\u30c3\\u30af\\u30de\\u30fc\\u30af\\u30ec\\u30c3\\u30c8\\u306f\\u52d5\\u753b\\u306e\\u3042\\u308b\\u30da\\u30fc\\u30b8\\u3067\\u5b9f\\u884c\\u3057\\u3066\\u304f\\u3060\\u3055\\u3044\\u3002\\n(YouTube, X, Instagram \\u306a\\u3069)");' +
      'return;' +
    '}' +

    'var vids=[];' +
    'document.querySelectorAll("video,audio").forEach(function(el){' +
      'if(el.currentSrc&&el.currentSrc.indexOf("blob:")!==0&&el.currentSrc.indexOf("data:")!==0)' +
        'vids.push({u:el.currentSrc,t:"media"});' +
    '});' +
    'document.querySelectorAll("source").forEach(function(el){' +
      'var s=el.src||el.getAttribute("src");' +
      'if(s&&s.indexOf("blob:")!==0&&s.indexOf("data:")!==0)' +
        'vids.push({u:s,t:"source"});' +
    '});' +

    'var seen={},uv=[];' +
    'vids.forEach(function(v){if(!seen[v.u]){seen[v.u]=1;uv.push(v);}});' +

    'var d=document.createElement("div");d.id="dl-bm";' +
    'Object.assign(d.style,{position:"fixed",top:"0",left:"0",right:"0",bottom:"0",zIndex:"2147483647",background:"rgba(10,10,16,.92)",display:"flex",flexDirection:"column",fontFamily:"-apple-system,sans-serif",color:"#eaeaf0",fontSize:"14px",overflow:"auto"});' +

    'var hd=document.createElement("div");' +
    'Object.assign(hd.style,{display:"flex",justifyContent:"space-between",alignItems:"center",padding:"12px 16px",borderBottom:"1px solid #28283a",flexShrink:"0"});' +
    'var logo=document.createElement("span");logo.textContent="DL Relay";' +
    'Object.assign(logo.style,{fontWeight:"800",fontSize:"16px",color:"#e89830"});' +
    'hd.appendChild(logo);' +
    'var cb=document.createElement("button");cb.textContent="\\u2715";' +
    'Object.assign(cb.style,{background:"none",border:"none",color:"#606078",fontSize:"20px",cursor:"pointer",padding:"4px 8px"});' +
    'cb.onclick=function(){d.remove();};' +
    'hd.appendChild(cb);d.appendChild(hd);' +

    'var ct=document.createElement("div");' +
    'Object.assign(ct.style,{padding:"16px",overflowY:"auto",flex:"1"});' +
    'd.appendChild(ct);' +

    'function mkBtn(label,onClick){' +
      'var b=document.createElement("button");b.textContent=label;' +
      'Object.assign(b.style,{display:"block",width:"100%",padding:"12px",marginBottom:"8px",background:"#e89830",color:"#0a0a10",border:"none",borderRadius:"8px",fontWeight:"600",fontSize:"14px",cursor:"pointer",fontFamily:"inherit",textAlign:"left",wordBreak:"break-all"});' +
      'b.onclick=onClick;return b;' +
    '}' +

    'function send(url,method,btn){' +
      'btn.disabled=true;btn.style.opacity="0.6";btn.textContent="Sending...";' +
      'var h={"Content-Type":"application/json"};' +
      'if(K)h["X-API-Key"]=K;' +
      'var fn=method?"":url.split("/").pop().split("?")[0];' +
      'if(!fn||fn==="watch")fn=document.title||"download";' +
      'var body={url:url,filename:fn};' +
      'if(method)body.method=method;' +
      'fetch(S+"/api/downloads",{method:"POST",headers:h,body:JSON.stringify(body)})' +
      '.then(function(r){if(!r.ok)throw new Error(r.status);return r.json();})' +
      '.then(function(j){btn.textContent="\\u2713 "+j.filename;btn.style.background="#30d880";})' +
      '.catch(function(e){btn.textContent="\\u2717 Error: "+e.message;btn.style.background="#e85050";});' +
    '}' +

    'ct.appendChild(mkBtn("yt-dlp: "+document.title,function(){send(location.href,"ytdlp",this);}));' +

    'if(uv.length>0){' +
      'var sep=document.createElement("div");' +
      'Object.assign(sep.style,{color:"#606078",fontSize:"12px",margin:"12px 0 8px",borderTop:"1px solid #28283a",paddingTop:"12px"});' +
      'sep.textContent="Detected media ("+uv.length+")";' +
      'ct.appendChild(sep);' +
      'uv.forEach(function(v){' +
        'var label=v.u.length>80?v.u.substring(0,80)+"...":v.u;' +
        'var isHls=v.u.indexOf(".m3u8")!==-1;' +
        'var isDash=v.u.indexOf(".mpd")!==-1;' +
        'if(isHls)label="[HLS] "+label;' +
        'if(isDash)label="[DASH] "+label;' +
        'ct.appendChild(mkBtn(label,function(){send(v.u,null,this);}));' +
      '});' +
    '}' +

    'if(uv.length===0){' +
      'var nv=document.createElement("div");' +
      'Object.assign(nv.style,{color:"#606078",fontSize:"12px",marginTop:"12px"});' +
      'nv.textContent="No <video>/<source> elements detected. Use yt-dlp button above.";' +
      'ct.appendChild(nv);' +
    '}' +

    'document.body.appendChild(d);' +
  '})()';

  return 'javascript:' + encodeURIComponent(src);
})();

document.getElementById('bookmarklet-link').href = bmCode;
document.getElementById('code-display').textContent = bmCode;

function copyCode() {
  navigator.clipboard.writeText(bmCode).then(function() {
    var btn = document.getElementById('copy-btn');
    btn.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg> Copied!';
    btn.classList.add('copied');
    setTimeout(function() {
      btn.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg> Copy to clipboard';
      btn.classList.remove('copied');
    }, 2000);
  });
}
</script>
</body>
</html>`
