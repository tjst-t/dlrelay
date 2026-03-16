// DL Relay — Popup UI

function isGenericBase(base) {
  const b = base.toLowerCase();
  const generic = ["download", "videoplayback", "player", "media", "video", "audio", "stream", "index", "playlist", "master", "chunklist"];
  if (generic.includes(b) || b.length < 3) return true;
  if (/^(video|audio|stream|media|seg|chunk)[\W_]/.test(b)) return true;
  return false;
}

function sanitizeForFilename(s) {
  return s.replace(/[\/\\:*?"<>|]/g, "_").replace(/\s+/g, " ").trim().substring(0, 200);
}

const DEFAULTS = {
  enabled: true,
  serverUrl: "",
  apiKey: "",
  mode: "video",
  minFileSize: 100 * 1024,
  autoDownload: false,
};

// --- DOM refs ---
const serverUrlInput = document.getElementById("server-url");
const enabledCheckbox = document.getElementById("enabled");
const autoDownloadCheckbox = document.getElementById("auto-download");
const apiKeyInput = document.getElementById("api-key");
const minFileSizeInput = document.getElementById("min-file-size");
const toggleBtns = document.querySelectorAll(".toggle");
const activityList = document.getElementById("activity-list");
const clearBtn = document.getElementById("clear-btn");
const dashboardLink = document.getElementById("dashboard-link");
const dot = document.getElementById("dot");
const statusText = document.getElementById("status-text");
const settingsBtn = document.getElementById("settings-btn");
const viewMain = document.getElementById("view-main");
const viewSettings = document.getElementById("view-settings");
const mediaList = document.getElementById("media-list");
const emptyState = document.getElementById("empty-state");

let currentView = "main";
let currentTabId = null;

// --- View switching ---
settingsBtn.addEventListener("click", () => {
  if (currentView === "main") {
    currentView = "settings";
    viewMain.classList.add("hidden");
    viewSettings.classList.remove("hidden");
    settingsBtn.classList.add("active");
  } else {
    currentView = "main";
    viewSettings.classList.add("hidden");
    viewMain.classList.remove("hidden");
    settingsBtn.classList.remove("active");
  }
});

// --- Load settings ---
async function load() {
  const settings = await chrome.storage.local.get(DEFAULTS);
  serverUrlInput.value = settings.serverUrl;
  enabledCheckbox.checked = settings.enabled;
  autoDownloadCheckbox.checked = settings.autoDownload;
  minFileSizeInput.value = Math.round((settings.minFileSize || 0) / 1024);
  apiKeyInput.value = settings.apiKey || "";

  toggleBtns.forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.mode === settings.mode);
  });

  dashboardLink.href = settings.serverUrl ? settings.serverUrl + "/" : "#";

  await checkHealth(settings.serverUrl);
  await renderMedia();
  await renderActivity();
}

// --- Health check ---
async function checkHealth(serverUrl) {
  if (!serverUrl) {
    dot.className = "dot";
    statusText.textContent = "No server";
    return;
  }
  try {
    const resp = await fetch(serverUrl + "/api/health", {
      signal: AbortSignal.timeout(3000),
    });
    if (resp.ok) {
      dot.className = "dot online";
      statusText.textContent = "Connected";
    } else {
      dot.className = "dot offline";
      statusText.textContent = "Error " + resp.status;
    }
  } catch {
    dot.className = "dot offline";
    statusText.textContent = "Unreachable";
  }
}

// --- Helpers ---

function formatSize(bytes) {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + " MB";
  return (bytes / (1024 * 1024 * 1024)).toFixed(2) + " GB";
}

function formatDuration(seconds) {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function esc(s) {
  if (!s) return "";
  const d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

// --- Media list (detected videos) ---

async function renderMedia() {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ type: "dlrelay-popup-get-media" }, (response) => {
      // Capture active tab ID for later use in downloads
      chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
        if (tabs[0]) currentTabId = tabs[0].id;
      });
      if (!response || !response.items || response.items.length === 0) {
        emptyState.style.display = "";
        resolve();
        return;
      }

      emptyState.style.display = "none";

      // Clear existing items except empty state
      const existing = mediaList.querySelectorAll(".media-card");
      existing.forEach((el) => el.remove());

      // If site-extracted (yt-dlp) items exist, hide webRequest/blob/DOM noise
      let items = response.items;
      const siteItems = items.filter(i => i.source === "site");
      if (siteItems.length > 0) {
        items = siteItems;
      }

      for (const item of items) {
        const el = createMediaItemElement(item);
        mediaList.appendChild(el);
      }
      resolve();
    });
  });
}

function createMediaItemElement(item) {
  const div = document.createElement("div");
  div.className = "media-card";
  div.dataset.url = item.url;

  // Thumbnail with duration overlay
  const thumbUrl = item.thumbnail || "";
  const durationStr = item.duration > 0 ? formatDuration(item.duration) : "";

  // Title: use item.title, fall back to filename
  const title = item.title || item.filename || "Unknown";

  // Domain from URL
  let domain = "";
  try { domain = new URL(item.pageUrl || item.url).hostname; } catch {}

  // Format info badges
  const formatBadges = [];
  const mime = (item.mimeType || "").split("/")[1]?.split(";")[0]?.toUpperCase();
  if (mime) formatBadges.push(mime);
  if (item.resolution) formatBadges.push(item.resolution);
  if (item.size > 0) formatBadges.push(formatSize(item.size));

  // Build quality selector HTML
  let qualityHtml = "";
  if (item.variants && item.variants.length > 0) {
    const options = item.variants.map((v, i) => {
      const label = v.label || v.resolution || `${Math.round(v.bandwidth / 1000)}kbps`;
      return `<option value="${i}" ${i === 0 ? "selected" : ""}>${esc(label)}</option>`;
    }).join("");
    qualityHtml = `<select class="quality-select">${options}</select>`;
  }

  div.innerHTML = `
    <div class="media-thumb-wrap">
      ${thumbUrl ? `<img class="media-thumb" src="${esc(thumbUrl)}" alt="">` : `<div class="media-thumb media-thumb-placeholder"></div>`}
      ${durationStr ? `<span class="media-thumb-duration">${durationStr}</span>` : ""}
    </div>
    <div class="media-detail">
      <div class="media-title" title="${esc(title)}">${esc(title)}</div>
      <div class="media-meta">
        ${domain ? `<span class="media-domain">${esc(domain)}</span>` : ""}
        ${formatBadges.map(b => `<span class="media-badge">${esc(b)}</span>`).join("")}
      </div>
      <div class="media-actions">
        ${qualityHtml}
        <button class="btn-download">Download</button>
      </div>
    </div>
  `;

  // Download handler
  div.querySelector(".btn-download").addEventListener("click", async (e) => {
    const btn = e.target;
    btn.disabled = true;
    btn.textContent = "Sending...";

    let downloadItem = { ...item };

    if (item.variants && item.variants.length > 0) {
      const select = div.querySelector(".quality-select");
      if (select) {
        const idx = parseInt(select.value, 10);
        const variant = item.variants[idx];
        if (variant) {
          downloadItem.url = variant.url;
          if (variant.audioUrl) downloadItem.audioUrl = variant.audioUrl;
          // Use page title for filename if the current name is URL-derived generic
          let base = downloadItem.filename.replace(/\.[^.]+$/, "");
          if (item.title && isGenericBase(base)) {
            base = sanitizeForFilename(item.title);
          }
          const ext = (downloadItem.filename.split(".").pop() || "mp4").replace(/^m3u8$/, "mp4");
          const safeLabel = (variant.label || "selected").replace(/[\/\\:*?"<>|]/g, "_");
          downloadItem.filename = `${base}_${safeLabel}.${ext}`;
        }
      }
    }

    try {
      chrome.runtime.sendMessage(
        { type: "dlrelay-popup-download", item: downloadItem, tabId: currentTabId },
        (result) => {
          if (result && result.error) {
            btn.textContent = "Error";
            btn.classList.add("error");
          } else {
            btn.textContent = "Sent!";
            btn.classList.add("success");
          }
          setTimeout(() => {
            btn.disabled = false;
            btn.textContent = "Download";
            btn.classList.remove("error", "success");
          }, 2000);
        }
      );
    } catch {
      btn.textContent = "Error";
      btn.classList.add("error");
      setTimeout(() => {
        btn.disabled = false;
        btn.textContent = "Download";
        btn.classList.remove("error");
      }, 2000);
    }
  });

  return div;
}

// --- Activity list ---
async function renderActivity() {
  const { recentActivity = [] } = await chrome.storage.local.get("recentActivity");

  if (recentActivity.length === 0) {
    activityList.innerHTML = '<div class="empty">No relayed downloads yet</div>';
    return;
  }

  const items = recentActivity.slice(0, 15);
  activityList.innerHTML = items
    .map((a) => {
      const time = new Date(a.timestamp).toLocaleTimeString();
      const badgeClass = "badge badge-" + (a.state || "queued");
      return `<div class="activity-item">
        <div class="activity-filename" title="${esc(a.filename)}">${esc(a.filename)}</div>
        <div class="activity-meta">
          <span class="${badgeClass}">${a.state || "queued"}</span>
          <span>${time}</span>
        </div>
      </div>`;
    })
    .join("");
}

// --- Save on change ---
let saveTimer;
serverUrlInput.addEventListener("input", () => {
  clearTimeout(saveTimer);
  saveTimer = setTimeout(async () => {
    const serverUrl = serverUrlInput.value.replace(/\/+$/, "");
    await chrome.storage.local.set({ serverUrl });
    dashboardLink.href = serverUrl ? serverUrl + "/" : "#";
    await checkHealth(serverUrl);
  }, 500);
});

enabledCheckbox.addEventListener("change", () => {
  chrome.storage.local.set({ enabled: enabledCheckbox.checked });
});

autoDownloadCheckbox.addEventListener("change", () => {
  chrome.storage.local.set({ autoDownload: autoDownloadCheckbox.checked });
});

apiKeyInput.addEventListener("input", () => {
  clearTimeout(saveTimer);
  saveTimer = setTimeout(async () => {
    await chrome.storage.local.set({ apiKey: apiKeyInput.value });
  }, 500);
});

minFileSizeInput.addEventListener("change", () => {
  const kb = parseInt(minFileSizeInput.value, 10) || 0;
  chrome.storage.local.set({ minFileSize: kb * 1024 });
});

toggleBtns.forEach((btn) => {
  btn.addEventListener("click", () => {
    toggleBtns.forEach((b) => b.classList.remove("active"));
    btn.classList.add("active");
    chrome.storage.local.set({ mode: btn.dataset.mode });
  });
});

clearBtn.addEventListener("click", async () => {
  await chrome.storage.local.set({ recentActivity: [] });
  await renderActivity();
  chrome.action.setBadgeText({ text: "" });
});

dashboardLink.addEventListener("click", (e) => {
  e.preventDefault();
  const url = dashboardLink.href;
  if (url && url !== "#") {
    chrome.tabs.create({ url });
  }
});

// --- Init ---
load();
