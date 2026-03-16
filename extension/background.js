// DL Relay — Background Service Worker
// Detects video URLs via webRequest + DOM + site-specific extractors,
// manages per-tab media lists, and relays downloads to the server.

// --- Constants ---

const DEFAULTS = {
  enabled: true,
  serverUrl: "",
  apiKey: "",
  mode: "video", // "video" | "all"
  minFileSize: 100 * 1024, // 100KB minimum (filter ads)
  autoDownload: false,
};

const VIDEO_MIMES = [
  "video/mp4", "video/webm", "video/ogg", "video/x-matroska",
  "video/x-flv", "video/3gpp", "video/quicktime",
  "video/mp2t", "video/mpeg",
];

const STREAM_MIMES = [
  "application/x-mpegurl", "application/vnd.apple.mpegurl",
  "application/dash+xml",
];

const AUDIO_MIMES = [
  "audio/mp4", "audio/mpeg", "audio/webm", "audio/ogg", "audio/aac",
];

const ALL_MEDIA_MIMES = [...VIDEO_MIMES, ...STREAM_MIMES, ...AUDIO_MIMES];

const VIDEO_EXTS = [
  ".mp4", ".mkv", ".webm", ".avi", ".mov", ".flv",
  ".ts", ".m3u8", ".mpd", ".m4v", ".wmv", ".3gp",
  ".m4s", ".mp3", ".m4a", ".aac", ".ogg",
];

const IGNORED_PATTERNS = [
  /^data:/,
  /^blob:/,
  /^chrome-extension:/,
  /^moz-extension:/,
  /^about:/,
  // Small tracking pixels, analytics
  /google-analytics\.com/,
  /googletagmanager\.com/,
  /doubleclick\.net/,
  /googlevideo\.com\/videoplayback/,
  /\/api\/stats\//,
  /\/player_api/,
];

// Domains handled by site extractors + yt-dlp — skip webRequest/DOM/blob noise.
// These sites are detected via site-specific or generic extractors in content-main.js,
// which send the page URL to yt-dlp for server-side download.
const EXTRACTOR_DOMAINS = [
  // Video platforms (site domains + CDN domains that serve video streams)
  "youtube.com", "youtu.be", "googlevideo.com",
  "nicovideo.jp", "nico.ms", "dmc.nico",
  "vimeo.com", "vimeocdn.com",
  "dailymotion.com", "dai.ly", "dmcdn.net",
  "bilibili.com", "b23.tv", "bilivideo.com", "hdslb.com",
  "twitch.tv", "twitchcdn.net", "jtvnw.net",
  // Social media
  "twitter.com", "x.com",
  "instagram.com", "cdninstagram.com",
  "tiktok.com", "tiktokcdn.com",
  "reddit.com", "redd.it", "redditmedia.com", "v.redd.it",
  // Adult sites (yt-dlp supported)
  "pornhub.com", "phncdn.com",
  "xvideos.com", "xnxx.com",
  "spankbang.com", "sb-cd.com",
  "xhamster.com", "xhcdn.com",
  "redtube.com",
  "youporn.com",
  "tube8.com",
  // Streaming / media
  "crunchyroll.com",
  "soundcloud.com", "sndcdn.com",
  "bandcamp.com", "bcbits.com",
  "mixcloud.com",
  // Other
  "facebook.com", "fbcdn.net",
  "ted.com",
];

function isExtractorDomain(url) {
  try {
    const host = new URL(url).hostname;
    return EXTRACTOR_DOMAINS.some(d => host === d || host.endsWith("." + d));
  } catch { return false; }
}

// --- Per-tab media store ---
// Key: tabId, Value: Map<url, MediaItem>
const tabMedia = new Map();

// Per-tab request headers captured by webRequest
// Key: tabId, Value: Map<url, headers>
const tabRequestHeaders = new Map();

// --- Storage helpers ---

let cachedSettings = null;

async function getSettings() {
  if (cachedSettings) return cachedSettings;
  cachedSettings = await chrome.storage.local.get(DEFAULTS);
  return cachedSettings;
}

// Invalidate cache when settings change
chrome.storage.onChanged.addListener((changes, area) => {
  if (area === "local") {
    cachedSettings = null;
  }
});

async function addActivity(entry) {
  const { recentActivity = [] } = await chrome.storage.local.get("recentActivity");
  recentActivity.unshift(entry);
  if (recentActivity.length > 50) recentActivity.length = 50;
  await chrome.storage.local.set({ recentActivity });
}

// --- Media item model ---

function createMediaItem(opts) {
  return {
    id: opts.id || generateId(),
    url: opts.url,
    pageUrl: opts.pageUrl || "",
    tabId: opts.tabId,
    type: opts.type || "http", // "http" | "hls" | "dash"
    mimeType: opts.mimeType || "",
    filename: opts.filename || extractFilenameFromUrl(opts.url),
    title: opts.title || "",
    thumbnail: opts.thumbnail || "",
    duration: opts.duration || 0, // seconds
    size: opts.size || 0,
    resolution: opts.resolution || "",
    source: opts.source || "webRequest", // "webRequest" | "dom" | "site" | "blob"
    headers: opts.headers || {},
    variants: opts.variants || [], // for HLS/DASH quality options
    timestamp: Date.now(),
  };
}

function generateId() {
  return Math.random().toString(36).substring(2, 10);
}

function extractFilenameFromUrl(url) {
  try {
    const pathname = new URL(url).pathname;
    const parts = pathname.split("/");
    const last = parts[parts.length - 1];
    return decodeURIComponent(last) || "download";
  } catch {
    return "download";
  }
}

function isGenericFilename(name) {
  const generic = ["download", "videoplayback", "player", "media", "video", "audio", "stream", "index", "playlist", "master", "chunklist"];
  const base = name.replace(/\.[^.]+$/, "").toLowerCase();
  if (generic.includes(base) || base.length < 3) return true;
  // HLS/DASH URL-derived names like "video_360p", "stream_720p", "seg-1"
  if (/^(video|audio|stream|media|seg|chunk)[\W_]/.test(base)) return true;
  return false;
}

function sanitizeFilename(title) {
  return title
    .replace(/[\/\\:*?"<>|]/g, "_")
    .replace(/\s+/g, " ")
    .trim()
    .substring(0, 200);
}

// --- Detection: webRequest ---

function isMediaUrl(url, mimeType) {
  if (IGNORED_PATTERNS.some(p => p.test(url))) return false;

  // Check MIME type
  if (mimeType) {
    const mime = mimeType.toLowerCase().split(";")[0].trim();
    if (ALL_MEDIA_MIMES.includes(mime)) return true;
    if (mime === "application/octet-stream") {
      return hasVideoExtension(url);
    }
  }

  return hasVideoExtension(url);
}

function hasVideoExtension(url) {
  try {
    const pathname = new URL(url).pathname.toLowerCase();
    return VIDEO_EXTS.some(ext => pathname.endsWith(ext));
  } catch {
    return false;
  }
}

function getMediaType(url, mimeType) {
  const lower = (mimeType || "").toLowerCase();
  if (lower.includes("mpegurl") || url.toLowerCase().includes(".m3u8")) return "hls";
  if (lower.includes("dash+xml") || url.toLowerCase().includes(".mpd")) return "dash";
  return "http";
}

// Capture request headers for all requests (needed for Cookie/Referer/Auth forwarding)
chrome.webRequest.onBeforeSendHeaders.addListener(
  (details) => {
    if (details.tabId < 0) return;
    if (!tabRequestHeaders.has(details.tabId)) {
      tabRequestHeaders.set(details.tabId, new Map());
    }
    const headerMap = {};
    for (const h of (details.requestHeaders || [])) {
      // Capture key headers
      const name = h.name.toLowerCase();
      if (["referer", "cookie", "authorization", "origin"].includes(name)) {
        headerMap[h.name] = h.value;
      }
    }
    if (Object.keys(headerMap).length > 0) {
      const map = tabRequestHeaders.get(details.tabId);
      // Limit per-tab header cache to prevent memory bloat on long-lived tabs
      if (map.size > 1000) {
        const first = map.keys().next().value;
        map.delete(first);
      }
      map.set(details.url, headerMap);
    }
  },
  { urls: ["<all_urls>"] },
  ["requestHeaders", "extraHeaders"]
);

// Monitor completed requests for video content
chrome.webRequest.onCompleted.addListener(
  async (details) => {
    if (details.tabId < 0) return;
    const settings = await getSettings();
    if (!settings.enabled) return;

    // Skip domains handled by site extractors + yt-dlp
    if (isExtractorDomain(details.url)) return;

    // Skip pages on extractor domains — site extractors handle detection there.
    // Check early to avoid unnecessary header parsing / media-type detection.
    let pageUrl = "";
    try {
      const tab = await chrome.tabs.get(details.tabId);
      pageUrl = tab.url || "";
    } catch { /* tab may be closed */ }
    if (isExtractorDomain(pageUrl)) return;

    // Skip other known noisy URLs (tracking, ads)
    const urlLower = details.url.toLowerCase();
    if (urlLower.includes("/pagead/") ||
        urlLower.includes("play.google.com")) {
      return;
    }

    const contentType = (details.responseHeaders || [])
      .find(h => h.name.toLowerCase() === "content-type");
    const mimeType = contentType ? contentType.value : "";

    const contentLength = (details.responseHeaders || [])
      .find(h => h.name.toLowerCase() === "content-length");
    const size = contentLength ? parseInt(contentLength.value, 10) : 0;

    if (!isMediaUrl(details.url, mimeType)) return;

    const type = getMediaType(details.url, mimeType);

    // Filter small files (ads, tracking pixels), but exempt HLS/DASH manifests
    // which are intentionally small metadata files (~1-5KB).
    if (type !== "hls" && type !== "dash" && size > 0 && size < settings.minFileSize) return;

    // Get captured headers for this request
    const capturedHeaders = tabRequestHeaders.get(details.tabId)?.get(details.url) || {};

    const item = createMediaItem({
      url: details.url,
      pageUrl,
      tabId: details.tabId,
      type,
      mimeType: mimeType.split(";")[0].trim(),
      size,
      source: "webRequest",
      headers: capturedHeaders,
    });

    addMediaToTab(details.tabId, item);

    // Auto-enrich HLS/DASH streams with quality variants
    if ((type === "hls" || type === "dash") && item.variants.length === 0) {
      enrichStreamMedia(details.tabId, item);
    }
  },
  { urls: ["<all_urls>"] },
  ["responseHeaders"]
);

// --- Detection: messages from content scripts ---

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  const tabId = sender.tab?.id;

  if (sender.tab) {
  if (message.type === "dlrelay-media-detected" && !isExtractorDomain(sender.tab.url || "")) {
    // From content.js DOM detection
    // Skip entirely if page is on an extractor domain (handled by site extractors)
    for (const media of (message.items || [])) {
      if (IGNORED_PATTERNS.some(p => p.test(media.url))) continue;
      if (isExtractorDomain(media.url)) continue;
      const item = createMediaItem({
        url: media.url,
        pageUrl: sender.tab.url || "",
        tabId,
        type: getMediaType(media.url, media.mimeType || ""),
        mimeType: media.mimeType || "",
        source: "dom",
      });
      addMediaToTab(tabId, item);
    }
  }

  if (message.type === "dlrelay-blob-detected" && !isExtractorDomain(sender.tab.url || "")) {
    // From content-main.js blob detection
    // Skip entirely if page is on an extractor domain (handled by site extractors)
    for (const media of (message.items || [])) {
      const url = media.sourceUrl || media.blobUrl;
      if (isExtractorDomain(url)) continue;
      if (IGNORED_PATTERNS.some(p => p.test(url))) continue;
      const item = createMediaItem({
        url,
        pageUrl: sender.tab.url || "",
        tabId,
        type: media.type || "http",
        source: "blob",
      });
      addMediaToTab(tabId, item);
    }
  }

  if (message.type === "dlrelay-site-detected") {
    // From site-specific extractors
    for (const media of (message.items || [])) {
      const item = createMediaItem({
        ...media,
        pageUrl: sender.tab.url || "",
        tabId,
        source: "site",
      });
      addMediaToTab(tabId, item);
    }
  }

  if (message.type === "dlrelay-page-metadata") {
      // Update all media items for this tab with page metadata
      const mediaMap = tabMedia.get(tabId);
      if (mediaMap) {
        for (const [, item] of mediaMap) {
          if (!item.title && message.title) item.title = message.title;
          if (!item.thumbnail && message.thumbnail) item.thumbnail = message.thumbnail;
          if (!item.duration && message.duration) item.duration = message.duration;
          // Update filename from title if it's a generic name
          if (message.title && isGenericFilename(item.filename)) {
            const ext = item.filename.split(".").pop() || "mp4";
            item.filename = sanitizeFilename(message.title) + "." + ext;
          }
        }
        saveTabMedia(tabId);
      }
    }

  if (message.type === "dlrelay-get-media") {
    const media = getTabMedia(tabId);
    sendResponse({ items: media });
    return true;
  }

  if (message.type === "dlrelay-download") {
    handleDownloadRequest(message.item, tabId).then(result => {
      sendResponse(result);
    }).catch(err => {
      sendResponse({ error: err.message });
    });
    return true;
  }
  } // end if (sender.tab)

  // --- Popup communication ---

  if (message.type === "dlrelay-popup-get-media") {
    chrome.tabs.query({ active: true, currentWindow: true }, async (tabs) => {
      if (tabs[0]) {
        const tabId = tabs[0].id;
        await restoreTabMedia(tabId);
        const media = getTabMedia(tabId);
        sendResponse({ items: media, tabUrl: tabs[0].url });
      } else {
        sendResponse({ items: [], tabUrl: "" });
      }
    });
    return true;
  }

  if (message.type === "dlrelay-popup-download") {
    const activeTabId = message.tabId;
    handleDownloadRequest(message.item, activeTabId).then(result => {
      sendResponse(result);
    }).catch(err => {
      sendResponse({ error: err.message });
    });
    return true;
  }

  if (message.type === "dlrelay-popup-parse-stream") {
    const { url, mediaType, headers } = message;
    const parseFn = mediaType === "hls" ? parseM3U8 : parseMPD;
    parseFn(url, headers).then(result => {
      sendResponse(result);
    }).catch(err => {
      sendResponse({ error: err.message });
    });
    return true;
  }
});

// --- Tab media management ---

function addMediaToTab(tabId, item) {
  // Reject URLs without a proper scheme (e.g. protocol-relative "//...")
  if (!item.url || (!item.url.startsWith("http://") && !item.url.startsWith("https://"))) {
    return;
  }

  if (!tabMedia.has(tabId)) {
    tabMedia.set(tabId, new Map());
  }
  const mediaMap = tabMedia.get(tabId);

  // Deduplicate by URL (keep latest with more info)
  const existing = mediaMap.get(item.url);
  if (existing) {
    // Merge: keep the one with more information
    if (item.size > existing.size) existing.size = item.size;
    if (item.mimeType && !existing.mimeType) existing.mimeType = item.mimeType;
    if (item.resolution && !existing.resolution) existing.resolution = item.resolution;
    if (Object.keys(item.headers).length > Object.keys(existing.headers).length) {
      existing.headers = { ...existing.headers, ...item.headers };
    }
    if (item.variants.length > existing.variants.length) {
      existing.variants = item.variants;
    }
    return;
  }

  mediaMap.set(item.url, item);
  updateBadge(tabId);
  saveTabMedia(tabId);
}

function getTabMedia(tabId) {
  const mediaMap = tabMedia.get(tabId);
  if (!mediaMap) return [];
  return Array.from(mediaMap.values()).sort((a, b) => b.timestamp - a.timestamp);
}

async function saveTabMedia(tabId) {
  const media = getTabMedia(tabId);
  try {
    await chrome.storage.session.set({ [`tab_${tabId}`]: media });
  } catch {
    // chrome.storage.session may not be available in all browsers
  }
}

async function restoreTabMedia(tabId) {
  try {
    const data = await chrome.storage.session.get(`tab_${tabId}`);
    const items = data[`tab_${tabId}`];
    if (items && items.length > 0) {
      const mediaMap = new Map();
      for (const item of items) {
        mediaMap.set(item.url, item);
      }
      tabMedia.set(tabId, mediaMap);
    }
  } catch { /* ignore */ }
}

// --- Badge ---

function updateBadge(tabId) {
  const count = tabMedia.get(tabId)?.size || 0;
  const text = count > 0 ? String(count) : "";
  chrome.action.setBadgeText({ text, tabId });
  chrome.action.setBadgeBackgroundColor({ color: "#38bdf8", tabId });
}

// Update badge on tab switch
chrome.tabs.onActivated.addListener(async (activeInfo) => {
  await restoreTabMedia(activeInfo.tabId);
  updateBadge(activeInfo.tabId);
});

// Clean up on tab close
chrome.tabs.onRemoved.addListener((tabId) => {
  tabMedia.delete(tabId);
  tabRequestHeaders.delete(tabId);
  chrome.storage.session.remove(`tab_${tabId}`).catch(() => {});
});

// Reset on navigation
chrome.tabs.onUpdated.addListener((tabId, changeInfo) => {
  if (changeInfo.status === "loading") {
    tabMedia.delete(tabId);
    tabRequestHeaders.delete(tabId);
    updateBadge(tabId);
  }
});

// --- Download handling ---

async function handleDownloadRequest(item, tabId) {
  const settings = await getSettings();
  if (!settings.serverUrl) {
    throw new Error("No server URL configured");
  }

  // Collect headers: captured headers + cookies
  const headers = { ...item.headers };

  // Get cookies for the URL
  try {
    const cookies = await chrome.cookies.getAll({ url: item.url });
    if (cookies.length > 0) {
      const cookieStr = cookies.map(c => `${c.name}=${c.value}`).join("; ");
      // Don't overwrite if already captured from webRequest
      if (!headers["Cookie"]) {
        headers["Cookie"] = cookieStr;
      }
    }
  } catch { /* cookies API may fail */ }

  // Set Referer from page URL if not already set
  if (!headers["Referer"] && item.pageUrl) {
    headers["Referer"] = item.pageUrl;
  }

  const payload = {
    url: item.url,
    filename: item.filename,
    headers,
    page_url: item.pageUrl || "",
  };

  // yt-dlp: server-side download via page URL extraction.
  // Pass Cookie, Referer, and User-Agent so yt-dlp can authenticate and
  // bypass bot detection (e.g. CloudFlare checks cf_clearance + UA match).
  if (item.type === "ytdlp") {
    payload.method = "ytdlp";
    payload.headers = {};
    if (headers["Cookie"]) payload.headers["Cookie"] = headers["Cookie"];
    if (headers["Referer"]) payload.headers["Referer"] = headers["Referer"];
    // User-Agent must match what the browser sent (cf_clearance is UA-bound)
    payload.headers["User-Agent"] = navigator.userAgent;

    // Attach a fallback URL from detected streams (HLS/HTTP) on the same tab.
    // If yt-dlp doesn't support this site, the server retries with this URL.
    if (tabId != null) {
      const others = getTabMedia(tabId).filter(m => m.type !== "ytdlp" && m.url !== item.url);
      // Prefer HLS > DASH > HTTP, then largest size
      const ranked = others.sort((a, b) => {
        const typeOrder = { hls: 0, dash: 1, http: 2 };
        const ta = typeOrder[a.type] ?? 9;
        const tb = typeOrder[b.type] ?? 9;
        if (ta !== tb) return ta - tb;
        return (b.size || 0) - (a.size || 0);
      });
      if (ranked.length > 0) {
        payload.fallback_url = ranked[0].url;
      }
    }
  }

  // Add audio_url for DASH with separate audio
  if (item.audioUrl) {
    payload.audio_url = item.audioUrl;
  }

  const fetchHeaders = { "Content-Type": "application/json" };
  if (settings.apiKey) {
    fetchHeaders["X-API-Key"] = settings.apiKey;
  }
  const resp = await fetch(settings.serverUrl + "/api/downloads", {
    method: "POST",
    headers: fetchHeaders,
    body: JSON.stringify(payload),
  });

  if (!resp.ok) {
    throw new Error(`HTTP ${resp.status}`);
  }

  const result = await resp.json();

  await addActivity({
    id: result.id,
    filename: item.filename,
    url: item.url,
    state: result.state,
    timestamp: Date.now(),
  });

  chrome.notifications.create({
    type: "basic",
    iconUrl: "icons/icon128.png",
    title: "DL Relay",
    message: `Relaying: ${item.filename}`,
  });

  return result;
}

// --- Legacy: intercept browser downloads ---

chrome.downloads.onCreated.addListener(async (item) => {
  const settings = await getSettings();
  if (!settings.enabled || !settings.serverUrl) return;
  if (settings.mode !== "all" && !isMediaUrl(item.url, item.mime)) return;

  chrome.downloads.cancel(item.id);
  chrome.downloads.erase({ id: item.id });

  const filename = extractFilenameFromUrl(item.filename || item.url);
  const headers = {};
  if (item.referrer) headers["Referer"] = item.referrer;

  try {
    const result = await handleDownloadRequest({
      url: item.url,
      filename,
      headers,
      pageUrl: item.referrer || "",
    });
  } catch (err) {
    chrome.notifications.create({
      type: "basic",
      iconUrl: "icons/icon128.png",
      title: "DL Relay — Error",
      message: `Failed to relay ${filename}: ${err.message}`,
    });
  }
});

// --- HLS parsing (for quality selection) ---

async function parseM3U8(url, headers) {
  try {
    const resp = await fetch(url, {
      headers: headers || {},
    });
    if (!resp.ok) return null;
    const text = await resp.text();
    return parseM3U8Text(text, url);
  } catch {
    return null;
  }
}

function parseM3U8Text(text, baseUrl) {
  const lines = text.split("\n").map(l => l.trim());
  const variants = [];
  let isMediaPlaylist = false;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];

    if (line.startsWith("#EXT-X-TARGETDURATION") || line.startsWith("#EXTINF:")) {
      isMediaPlaylist = true;
    }

    if (line.startsWith("#EXT-X-STREAM-INF:")) {
      const attrs = parseM3U8Attributes(line.substring("#EXT-X-STREAM-INF:".length));
      const nextLine = lines[i + 1];
      if (nextLine && !nextLine.startsWith("#")) {
        const variantUrl = resolveUrl(baseUrl, nextLine);
        const resolution = attrs.RESOLUTION || "";
        const bandwidth = parseInt(attrs.BANDWIDTH || "0", 10);
        const codecs = attrs.CODECS || "";

        let label = "";
        if (resolution) {
          const height = resolution.split("x")[1];
          label = `${height}p`;
        } else if (bandwidth > 0) {
          label = `${Math.round(bandwidth / 1000)}kbps`;
        }

        variants.push({
          url: variantUrl,
          resolution,
          bandwidth,
          codecs,
          label,
        });
      }
    }
  }

  if (variants.length > 0) {
    // Sort by bandwidth descending (best quality first)
    variants.sort((a, b) => b.bandwidth - a.bandwidth);
    return { type: "master", variants };
  }

  if (isMediaPlaylist) {
    return { type: "media", url: baseUrl };
  }

  return null;
}

function parseM3U8Attributes(str) {
  const attrs = {};
  const re = /([A-Z0-9-]+)=(?:"([^"]*)"|([^,]*))/g;
  let match;
  while ((match = re.exec(str)) !== null) {
    attrs[match[1]] = match[2] || match[3];
  }
  return attrs;
}

// --- DASH (MPD) parsing ---

async function parseMPD(url, headers) {
  try {
    const resp = await fetch(url, { headers: headers || {} });
    if (!resp.ok) return null;
    const text = await resp.text();
    return parseMPDText(text, url);
  } catch {
    return null;
  }
}

function parseMPDText(text, baseUrl) {
  const parser = new DOMParser();
  const doc = parser.parseFromString(text, "application/xml");

  const videoReps = [];
  const audioReps = [];

  const adaptationSets = doc.querySelectorAll("AdaptationSet");
  for (const as of adaptationSets) {
    const mimeType = as.getAttribute("mimeType") || "";
    const contentType = as.getAttribute("contentType") || "";
    const isVideo = mimeType.startsWith("video") || contentType === "video";
    const isAudio = mimeType.startsWith("audio") || contentType === "audio";

    const reps = as.querySelectorAll("Representation");
    for (const rep of reps) {
      const repMime = rep.getAttribute("mimeType") || mimeType;
      const bandwidth = parseInt(rep.getAttribute("bandwidth") || "0", 10);
      const width = parseInt(rep.getAttribute("width") || "0", 10);
      const height = parseInt(rep.getAttribute("height") || "0", 10);
      const codecs = rep.getAttribute("codecs") || as.getAttribute("codecs") || "";

      // Get URL from BaseURL or SegmentTemplate
      let repUrl = "";
      const baseUrlEl = rep.querySelector("BaseURL") || as.querySelector("BaseURL");
      if (baseUrlEl) {
        repUrl = resolveUrl(baseUrl, baseUrlEl.textContent.trim());
      }

      const segTemplate = rep.querySelector("SegmentTemplate") || as.querySelector("SegmentTemplate");
      if (segTemplate && !repUrl) {
        const media = segTemplate.getAttribute("media") || "";
        const init = segTemplate.getAttribute("initialization") || "";
        repUrl = resolveUrl(baseUrl, init || media);
      }

      const entry = {
        url: repUrl || baseUrl,
        bandwidth,
        width,
        height,
        codecs,
        mimeType: repMime,
        label: "",
      };

      if (isVideo || repMime.startsWith("video")) {
        entry.label = height > 0 ? `${height}p` : `${Math.round(bandwidth / 1000)}kbps`;
        videoReps.push(entry);
      } else if (isAudio || repMime.startsWith("audio")) {
        entry.label = `${Math.round(bandwidth / 1000)}kbps`;
        audioReps.push(entry);
      }
    }
  }

  videoReps.sort((a, b) => b.bandwidth - a.bandwidth);
  audioReps.sort((a, b) => b.bandwidth - a.bandwidth);

  // Create combined variants
  const variants = [];
  if (videoReps.length > 0) {
    const bestAudio = audioReps[0];
    for (const video of videoReps) {
      const label = bestAudio
        ? `${video.label} + ${bestAudio.label} audio`
        : video.label;
      variants.push({
        url: video.url,
        audioUrl: bestAudio ? bestAudio.url : "",
        resolution: video.width > 0 ? `${video.width}x${video.height}` : "",
        bandwidth: video.bandwidth,
        codecs: video.codecs,
        label,
      });
    }
  }

  return {
    type: "dash",
    variants,
    videoReps,
    audioReps,
  };
}

function resolveUrl(base, ref) {
  try {
    return new URL(ref, base).href;
  } catch {
    return ref;
  }
}

// When an HLS or DASH stream is detected, parse variants.
// If the manifest is a master playlist, attach variants to the item and
// remove any standalone items whose URL matches a variant (dedup).
async function enrichStreamMedia(tabId, item) {
  if (item.type === "hls") {
    const parsed = await parseM3U8(item.url, item.headers);
    if (parsed && parsed.type === "master" && parsed.variants.length > 0) {
      item.variants = parsed.variants;
      addMediaToTab(tabId, item);
      // Remove standalone items that are variant playlists of this master
      const mediaMap = tabMedia.get(tabId);
      if (mediaMap) {
        for (const v of parsed.variants) {
          if (v.url !== item.url && mediaMap.has(v.url)) {
            mediaMap.delete(v.url);
          }
        }
        saveTabMedia(tabId);
        updateBadge(tabId);
      }
    } else if (parsed && parsed.type === "media") {
      // This is a variant/media playlist, not a master.
      // Check if it's already covered by an existing master playlist.
      const mediaMap = tabMedia.get(tabId);
      if (mediaMap) {
        for (const [, existing] of mediaMap) {
          if (existing.type === "hls" && existing.variants.length > 0) {
            if (existing.variants.some(v => v.url === item.url)) {
              // Already a variant of an existing master — remove standalone
              mediaMap.delete(item.url);
              saveTabMedia(tabId);
              updateBadge(tabId);
              return;
            }
          }
        }
      }
    }
  } else if (item.type === "dash") {
    const parsed = await parseMPD(item.url, item.headers);
    if (parsed && parsed.variants.length > 0) {
      item.variants = parsed.variants;
      addMediaToTab(tabId, item);
    }
  }
}

