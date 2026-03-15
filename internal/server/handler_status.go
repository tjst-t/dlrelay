package server

import (
	"net/http"
	"strings"
)

func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	base := safeServerURL(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(strings.ReplaceAll(statusHTML, "{{SERVER_URL}}", base)))
}

const statusHTML = `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>DL Relay — Downloads</title>
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
  padding: 0 1rem;
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
.container { max-width: 960px; margin: 0 auto; }
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
.logo span { color: var(--muted); font-weight: 600; }
.refresh-info { color: var(--muted); font-size: 0.78rem; }
.stats { display: flex; gap: 0.75rem; margin-bottom: 1.5rem; flex-wrap: wrap; }
.stat {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 0.75rem 1.25rem;
  min-width: 110px;
}
.stat-value {
  font-family: "Syne", system-ui, sans-serif;
  font-size: 1.4rem;
  font-weight: 700;
}
.stat-label { font-size: 0.72rem; color: var(--muted); text-transform: uppercase; letter-spacing: 0.04em; }
table {
  width: 100%;
  border-collapse: collapse;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
  overflow: hidden;
}
th {
  text-align: left;
  padding: 0.65rem 1rem;
  background: var(--bg);
  color: var(--muted);
  font-size: 0.7rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  font-weight: 600;
  border-bottom: 1px solid var(--border);
}
td {
  padding: 0.7rem 1rem;
  border-bottom: 1px solid var(--border);
  font-size: 0.84rem;
  vertical-align: middle;
}
tr:last-child td { border-bottom: none; }
tr:hover td { background: rgba(232, 152, 48, 0.02); }
.badge {
  display: inline-block;
  padding: 0.12rem 0.5rem;
  border-radius: 999px;
  font-size: 0.68rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}
.badge-completed { background: var(--green-dim); color: var(--green); }
.badge-downloading { background: var(--accent-dim); color: var(--accent); }
.badge-queued { background: rgba(240, 176, 48, 0.12); color: var(--yellow); }
.badge-failed { background: var(--red-dim); color: var(--red); }
.badge-cancelled { background: rgba(96, 96, 120, 0.15); color: var(--muted); }
.progress-bar {
  width: 100%;
  height: 5px;
  background: var(--border);
  border-radius: 3px;
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  border-radius: 3px;
  transition: width 0.3s;
  background: linear-gradient(90deg, var(--accent), var(--accent-hover));
}
.progress-fill.done { background: var(--green); }
.progress-fill.fail { background: var(--red); }
.filename {
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}
.url {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--muted);
  font-size: 0.72rem;
}
.size { white-space: nowrap; color: var(--text-2); }
.error-text {
  color: var(--red);
  font-size: 0.7rem;
  margin-top: 0.2rem;
  max-width: 340px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.btn-cancel, .btn-preview {
  background: none;
  padding: 0.18rem 0.5rem;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.68rem;
  font-family: inherit;
  transition: all 0.15s;
}
.btn-cancel {
  border: 1px solid var(--red);
  color: var(--red);
}
.btn-cancel:hover { background: var(--red); color: var(--bg); }
.btn-preview {
  border: 1px solid var(--green);
  color: var(--green);
  margin-right: 0.3rem;
}
.btn-preview:hover { background: var(--green); color: var(--bg); }
.modal-overlay {
  display: none;
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.85);
  z-index: 9999;
  align-items: center;
  justify-content: center;
}
.modal-overlay.active { display: flex; }
.modal-content {
  position: relative;
  max-width: 90vw;
  max-height: 90vh;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 12px;
  overflow: hidden;
}
.modal-content video {
  display: block;
  max-width: 90vw;
  max-height: 85vh;
}
.modal-close {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  background: rgba(0,0,0,0.6);
  border: none;
  color: #fff;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  cursor: pointer;
  font-size: 1.1rem;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1;
}
.modal-close:hover { background: rgba(255,255,255,0.2); }
.modal-title {
  padding: 0.5rem 1rem;
  font-size: 0.8rem;
  color: var(--text-2);
  background: var(--surface-2);
  border-top: 1px solid var(--border);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.empty {
  text-align: center;
  padding: 4rem 1rem;
  color: var(--muted);
}
.empty-icon { font-size: 2rem; margin-bottom: 0.5rem; opacity: 0.3; }
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
.downloading-anim { animation: pulse 1.5s infinite; }
@media (max-width: 700px) {
  .stats { gap: 0.5rem; }
  .stat { min-width: 80px; padding: 0.6rem 0.9rem; }
  .stat-value { font-size: 1.1rem; }
  .filename { max-width: 160px; }
  .url { max-width: 140px; }
  td, th { padding: 0.5rem 0.6rem; }
}
</style>
</head>
<body>
<div class="container">
  <header class="header">
    <div class="logo"><a href="/"><em>DL</em> Relay</a> <span>/ Downloads</span></div>
    <div class="refresh-info">Auto-refresh: <span id="countdown">2</span>s</div>
  </header>

  <div class="stats" id="stats">
    <div class="stat"><div class="stat-value" id="stat-total">-</div><div class="stat-label">Total</div></div>
    <div class="stat"><div class="stat-value downloading-anim" id="stat-active" style="color:var(--accent)">-</div><div class="stat-label">Active</div></div>
    <div class="stat"><div class="stat-value" id="stat-completed" style="color:var(--green)">-</div><div class="stat-label">Completed</div></div>
    <div class="stat"><div class="stat-value" id="stat-failed" style="color:var(--red)">-</div><div class="stat-label">Failed</div></div>
  </div>

  <div id="content">
    <div class="empty"><div class="empty-icon">&#9744;</div>No downloads yet</div>
  </div>
</div>

<div class="modal-overlay" id="preview-modal" onclick="closePreview(event)">
  <div class="modal-content">
    <button class="modal-close" onclick="closePreview()">&times;</button>
    <video id="preview-video" controls></video>
    <div class="modal-title" id="preview-title"></div>
  </div>
</div>

<script>
var API = "{{SERVER_URL}}";
var timer = 2;

function formatBytes(b) {
  if (b <= 0) return "-";
  var u = ["B","KB","MB","GB"];
  var i = Math.min(Math.floor(Math.log(b) / Math.log(1024)), u.length - 1);
  return (b / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0) + " " + u[i];
}

function badgeClass(state) {
  return "badge badge-" + state;
}

function progressPct(dl) {
  if (dl.state === "completed") return 100;
  if (dl.total_bytes > 0) return Math.round(dl.bytes_received / dl.total_bytes * 100);
  return 0;
}

function fillClass(state) {
  if (state === "completed") return "progress-fill done";
  if (state === "failed") return "progress-fill fail";
  return "progress-fill";
}

function cancelDownload(id) {
  fetch(API + "/api/downloads/" + id, { method: "DELETE" });
  refresh();
}

function isVideoFile(filename) {
  if (!filename) return false;
  var ext = filename.split(".").pop().toLowerCase();
  return ["mp4","webm","ogg","m4v"].indexOf(ext) >= 0;
}

function openPreview(id, filename) {
  var modal = document.getElementById("preview-modal");
  var video = document.getElementById("preview-video");
  var title = document.getElementById("preview-title");
  video.src = API + "/api/downloads/" + id + "/file";
  title.textContent = filename;
  modal.classList.add("active");
  var p = video.play();
  if (p && p.catch) p.catch(function() {});
}

function closePreview(event) {
  if (event && event.target !== event.currentTarget) return;
  var modal = document.getElementById("preview-modal");
  var video = document.getElementById("preview-video");
  video.pause();
  video.src = "";
  modal.classList.remove("active");
}

function refresh() {
  fetch(API + "/api/downloads").then(function(res) {
    return res.json();
  }).then(function(list) {
    var counts = { total: list.length, downloading: 0, queued: 0, completed: 0, failed: 0, cancelled: 0 };
    list.forEach(function(d) { if (counts[d.state] !== undefined) counts[d.state]++; });

    document.getElementById("stat-total").textContent = counts.total;
    document.getElementById("stat-active").textContent = counts.downloading + counts.queued;
    document.getElementById("stat-completed").textContent = counts.completed;
    document.getElementById("stat-failed").textContent = counts.failed + counts.cancelled;

    if (list.length === 0) {
      document.getElementById("content").innerHTML = '<div class="empty"><div class="empty-icon">&#9744;</div>No downloads yet</div>';
      return;
    }

    var order = { downloading: 0, queued: 1, failed: 2, cancelled: 3, completed: 4 };
    list.sort(function(a, b) { return (order[a.state] || 9) - (order[b.state] || 9); });

    var html = '<table><thead><tr><th>File</th><th>Status</th><th>Progress</th><th>Size</th><th></th></tr></thead><tbody>';
    list.forEach(function(dl) {
      var pct = progressPct(dl);
      var canCancel = dl.state === "downloading" || dl.state === "queued";
      html += '<tr>';
      html += '<td><div class="filename" title="' + esc(dl.filename) + '">' + esc(dl.filename) + '</div><div class="url" title="' + esc(dl.url) + '">' + esc(dl.url) + '</div></td>';
      html += '<td><span class="' + badgeClass(dl.state) + '">' + dl.state + '</span>';
      if (dl.error) html += '<div class="error-text" title="' + esc(dl.error) + '">' + esc(dl.error) + '</div>';
      html += '</td>';
      html += '<td style="min-width:110px"><div class="progress-bar"><div class="' + fillClass(dl.state) + '" style="width:' + pct + '%"></div></div><div style="font-size:.7rem;color:var(--muted);margin-top:.2rem">' + pct + '%</div></td>';
      html += '<td class="size">' + formatBytes(dl.bytes_received) + (dl.total_bytes > 0 ? ' / ' + formatBytes(dl.total_bytes) : '') + '</td>';
      var actions = '';
      if (dl.state === "completed" && dl.has_file && isVideoFile(dl.filename)) {
        actions += '<button class="btn-preview" onclick="openPreview(\'' + dl.id + '\', \'' + esc(dl.filename).replace(/'/g, "\\'") + '\')">Preview</button>';
      }
      if (canCancel) {
        actions += '<button class="btn-cancel" onclick="cancelDownload(\'' + dl.id + '\')">Cancel</button>';
      }
      html += '<td>' + actions + '</td>';
      html += '</tr>';
    });
    html += '</tbody></table>';
    document.getElementById("content").innerHTML = html;
  }).catch(function(e) {
    console.error("refresh failed:", e);
  });
}

function esc(s) {
  if (!s) return "";
  var d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

document.addEventListener("keydown", function(e) {
  if (e.key === "Escape") closePreview();
});

refresh();
setInterval(function() {
  timer--;
  if (timer <= 0) { timer = 2; refresh(); }
  document.getElementById("countdown").textContent = timer;
}, 1000);
</script>
</body>
</html>`
