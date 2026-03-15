// DL Relay — Content Script (MAIN world)
// Hooks MediaSource API to capture blob: URL source information,
// and runs site-specific video URL extractors.

(function () {
  "use strict";

  // ========================================================
  // Part 1: MediaSource / blob: URL hook (Issue #11)
  // ========================================================

  // Track MediaSource objects and their associated fetch URLs
  const mediaSourceMap = new Map(); // MediaSource -> { blobUrl, sourceUrls }
  const blobToSourceUrls = new Map();   // blobUrl -> Set<sourceUrl>
  const fetchUrlBuffer = [];            // Recent fetch URLs (FIFO, max 200)

  // Hook URL.createObjectURL to track blob: URLs created from MediaSource
  const origCreateObjectURL = URL.createObjectURL;
  URL.createObjectURL = function (obj) {
    const url = origCreateObjectURL.call(this, obj);
    if (obj instanceof MediaSource) {
      mediaSourceMap.set(obj, { blobUrl: url, sourceUrls: new Set() });
      blobToSourceUrls.set(url, new Set());
    }
    return url;
  };

  // Hook SourceBuffer.appendBuffer to track which URLs feed into a MediaSource
  const origAppendBuffer = window.SourceBuffer?.prototype?.appendBuffer;
  if (origAppendBuffer) {
    SourceBuffer.prototype.appendBuffer = function (data) {
      // Find the MediaSource that owns this SourceBuffer and attribute recent fetch URLs
      const recentUrls = fetchUrlBuffer.slice(-5);
      for (const [ms, info] of mediaSourceMap) {
        try {
          const buffers = ms.sourceBuffers;
          for (let i = 0; i < buffers.length; i++) {
            if (buffers[i] === this) {
              const urlSet = blobToSourceUrls.get(info.blobUrl);
              if (urlSet) {
                for (const u of recentUrls) urlSet.add(u);
              }
              break;
            }
          }
        } catch { /* MediaSource may be closed */ }
      }
      return origAppendBuffer.call(this, data);
    };
  }

  // Hook fetch to track media segment URLs
  const origFetch = window.fetch;
  window.fetch = function (...args) {
    const url = typeof args[0] === "string" ? args[0] : args[0]?.url;
    if (url && !url.startsWith("data:") && !url.startsWith("blob:")) {
      fetchUrlBuffer.push(url);
      if (fetchUrlBuffer.length > 200) fetchUrlBuffer.shift();
    }
    return origFetch.apply(this, args);
  };

  // Hook XMLHttpRequest to track media segment URLs
  const origXHROpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function (method, url, ...rest) {
    if (typeof url === "string" && !url.startsWith("data:") && !url.startsWith("blob:")) {
      fetchUrlBuffer.push(url);
      if (fetchUrlBuffer.length > 200) fetchUrlBuffer.shift();
    }
    return origXHROpen.call(this, method, url, ...rest);
  };

  // Periodically check for blob: URLs in video elements and report source URLs
  const reportedBlobs = new Set();
  function checkBlobVideos() {
    const videos = document.querySelectorAll("video");
    for (const video of videos) {
      const src = video.currentSrc || video.src || "";
      if (!src.startsWith("blob:")) continue;
      if (reportedBlobs.has(src)) continue;

      const sourceUrls = blobToSourceUrls.get(src);
      if (sourceUrls && sourceUrls.size > 0) {
        reportedBlobs.add(src);

        // Filter out obviously non-media source URLs that were captured
        // by the fetch/XHR hook near an appendBuffer call.
        const nonMediaRe = /\.(svg|css|js|png|jpe?g|gif|ico|woff2?|ttf|eot|json|html?|xml|txt|map|vtt|webp)(\?|$)/i;
        const filtered = [...sourceUrls].filter(u => !nonMediaRe.test(u));
        if (filtered.length === 0) return;

        // Classify each URL
        const classified = filtered.map(u => {
          const lower = u.toLowerCase();
          let type = "http";
          if (lower.includes(".m3u8") || lower.includes("m3u8")) type = "hls";
          else if (lower.includes(".mpd") || lower.includes("dash")) type = "dash";
          return { sourceUrl: u, type };
        });

        // Pick the best single URL to represent this blob: video.
        // Prefer manifest URLs (HLS/DASH) over segment URLs.
        const best = classified.find(c => c.type === "hls")
          || classified.find(c => c.type === "dash")
          || classified[0];

        window.postMessage({
          type: "dlrelay-blob-from-main",
          items: [{
            blobUrl: src,
            sourceUrl: best.sourceUrl,
            type: best.type,
          }],
        }, window.location.origin);
      }
    }
  }

  // Check periodically for new blob videos and clean up closed MediaSources
  setInterval(() => {
    checkBlobVideos();
    // Clean up closed MediaSource entries to prevent memory leak
    for (const [ms, info] of mediaSourceMap) {
      try {
        if (ms.readyState === "closed") {
          mediaSourceMap.delete(ms);
          blobToSourceUrls.delete(info.blobUrl);
        }
      } catch {
        mediaSourceMap.delete(ms);
      }
    }
  }, 3000);
  // Also check when video elements get their src set
  const origVideoSrcDesc = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, "src");
  if (origVideoSrcDesc?.set) {
    Object.defineProperty(HTMLMediaElement.prototype, "src", {
      set(val) {
        origVideoSrcDesc.set.call(this, val);
        if (typeof val === "string" && val.startsWith("blob:")) {
          setTimeout(checkBlobVideos, 500);
        }
      },
      get() {
        return origVideoSrcDesc.get?.call(this) || "";
      },
      configurable: true,
    });
  }

  // ========================================================
  // Part 2: Site-specific extractors (top frame only)
  // ========================================================
  //
  // All supported sites use yt-dlp on the server for the actual download.
  // Extractors only need to provide metadata (title, thumbnail, duration)
  // for the popup UI. The page URL is sent to the server as the download target.
  //
  // Skip in iframes — extractors should only run on the top-level page.
  // Part 1 (blob hooks) still runs in all frames to capture iframe players.
  if (window !== window.top) return;

  const SITE_EXTRACTORS = [];

  function sanitizeForFilename(s) {
    return s.replace(/[\/\\:*?"<>|]/g, "_").replace(/\s+/g, " ").trim().substring(0, 200);
  }

  // Helper: build a yt-dlp item from page metadata
  function makeYtdlpItem(opts) {
    return {
      url: opts.url || location.href,
      type: "ytdlp",
      mimeType: "video/mp4",
      filename: sanitizeForFilename(opts.title || document.title || "video") + ".mp4",
      title: opts.title || "",
      thumbnail: opts.thumbnail || "",
      duration: opts.duration || 0,
    };
  }

  // Helper: get metadata from og/meta tags
  function getPageMetadata() {
    const title = document.querySelector('meta[property="og:title"]')?.content || document.title || "";
    const thumbnail = document.querySelector('meta[property="og:image"]')?.content || "";
    let duration = 0;
    const durationMeta = document.querySelector('meta[itemprop="duration"]')?.content;
    if (durationMeta) {
      const m = durationMeta.match(/PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?/);
      if (m) duration = (parseInt(m[1] || 0) * 3600) + (parseInt(m[2] || 0) * 60) + parseInt(m[3] || 0);
    }
    if (!duration) {
      const video = document.querySelector("video");
      if (video && video.duration && isFinite(video.duration)) duration = Math.round(video.duration);
    }
    return { title, thumbnail, duration };
  }

  // --- Twitter/X ---
  SITE_EXTRACTORS.push({
    name: "twitter",
    matches: ["twitter.com", "x.com"],
    extractFromPage() {
      // Only detect on tweet/status pages with video
      if (!location.pathname.match(/\/status\/\d+/)) return [];
      const video = document.querySelector("video");
      if (!video) return [];
      const meta = getPageMetadata();
      return [makeYtdlpItem(meta)];
    },
  });

  // --- Instagram ---
  SITE_EXTRACTORS.push({
    name: "instagram",
    matches: ["instagram.com"],
    extractFromPage() {
      // Detect on reel/post pages with video
      if (!location.pathname.match(/\/(reel|p)\//)) return [];
      const video = document.querySelector("video");
      if (!video) return [];
      const meta = getPageMetadata();
      return [makeYtdlpItem(meta)];
    },
  });

  // --- TikTok ---
  SITE_EXTRACTORS.push({
    name: "tiktok",
    matches: ["tiktok.com"],
    extractFromPage() {
      if (!location.pathname.match(/\/video\/|@.*\/video/)) return [];
      const video = document.querySelector("video");
      if (!video) return [];
      const meta = getPageMetadata();
      return [makeYtdlpItem(meta)];
    },
  });

  // --- YouTube ---
  let lastExtractedYouTubeVideoId = "";

  function getCurrentYouTubeVideoId() {
    try {
      const params = new URLSearchParams(location.search);
      return params.get("v") || "";
    } catch { return ""; }
  }

  SITE_EXTRACTORS.push({
    name: "youtube",
    matches: ["youtube.com", "youtu.be"],
    extractFromPage() {
      const currentId = getCurrentYouTubeVideoId();
      if (!currentId) return [];
      if (currentId === lastExtractedYouTubeVideoId) return [];

      // Try YouTube-specific data sources for richer metadata
      const sources = [
        window.ytInitialPlayerResponse,
        window.ytplayer?.config?.args?.raw_player_response,
      ];
      try {
        const flexy = document.querySelector("ytd-watch-flexy");
        if (flexy) {
          const d = flexy.playerData || flexy.__data?.playerResponse;
          if (d) sources.push(d);
        }
      } catch { /* ignore */ }
      try {
        const p = document.querySelector("ytd-player");
        if (p) { const r = p.player_?.getPlayerResponse?.(); if (r) sources.push(r); }
      } catch { /* ignore */ }

      for (const source of sources) {
        if (!source) continue;
        const videoId = source?.videoDetails?.videoId || "";
        if (videoId !== currentId) continue;
        const d = source.videoDetails || {};
        lastExtractedYouTubeVideoId = currentId;
        return [makeYtdlpItem({
          title: d.title || "",
          thumbnail: d.thumbnail?.thumbnails?.slice(-1)?.[0]?.url || "",
          duration: parseInt(d.lengthSeconds || "0", 10),
        })];
      }

      // Fallback: script tag parsing
      try {
        for (const script of document.querySelectorAll("script")) {
          const match = (script.textContent || "").match(/ytInitialPlayerResponse\s*=\s*(\{.+?\});/s);
          if (!match) continue;
          const data = JSON.parse(match[1]);
          if (data?.videoDetails?.videoId !== currentId) continue;
          const d = data.videoDetails;
          lastExtractedYouTubeVideoId = currentId;
          return [makeYtdlpItem({
            title: d.title || "",
            thumbnail: d.thumbnail?.thumbnails?.slice(-1)?.[0]?.url || "",
            duration: parseInt(d.lengthSeconds || "0", 10),
          })];
        }
      } catch { /* ignore */ }

      return [];
    },
  });

  // --- Reddit ---
  SITE_EXTRACTORS.push({
    name: "reddit",
    matches: ["reddit.com", "redd.it"],
    extractFromPage() {
      if (!location.pathname.match(/\/comments\//)) return [];
      const video = document.querySelector("video, shreddit-player");
      if (!video) return [];
      const meta = getPageMetadata();
      return [makeYtdlpItem(meta)];
    },
  });

  // --- Niconico ---
  SITE_EXTRACTORS.push({
    name: "niconico",
    matches: ["nicovideo.jp", "nico.ms"],
    extractFromPage() {
      if (!location.pathname.match(/\/watch\//)) return [];
      const meta = getPageMetadata();
      // Niconico-specific: richer metadata from data attribute
      try {
        const el = document.getElementById("js-initial-watch-data");
        if (el) {
          const data = JSON.parse(el.getAttribute("data-api-data") || "{}");
          if (data?.video?.title) meta.title = data.video.title;
          if (data?.video?.thumbnail?.url) meta.thumbnail = data.video.thumbnail.url;
          if (data?.video?.duration) meta.duration = data.video.duration;
        }
      } catch { /* ignore */ }
      return [makeYtdlpItem(meta)];
    },
  });

  // --- Twitch ---
  SITE_EXTRACTORS.push({
    name: "twitch",
    matches: ["twitch.tv"],
    extractFromPage() {
      // Detect on clip/video pages
      if (!location.pathname.match(/\/(clip|videos)\//)) {
        // Also detect live streams (channel pages with video)
        const video = document.querySelector("video");
        if (!video) return [];
      }
      const meta = getPageMetadata();
      return [makeYtdlpItem(meta)];
    },
  });

  // --- Vimeo ---
  SITE_EXTRACTORS.push({
    name: "vimeo",
    matches: ["vimeo.com"],
    extractFromPage() {
      if (!location.pathname.match(/\/\d+/)) return [];
      const meta = getPageMetadata();
      return [makeYtdlpItem(meta)];
    },
  });

  // --- Dailymotion ---
  SITE_EXTRACTORS.push({
    name: "dailymotion",
    matches: ["dailymotion.com", "dai.ly"],
    extractFromPage() {
      if (!location.pathname.match(/\/video\//)) return [];
      const meta = getPageMetadata();
      return [makeYtdlpItem(meta)];
    },
  });

  // --- Bilibili ---
  SITE_EXTRACTORS.push({
    name: "bilibili",
    matches: ["bilibili.com", "b23.tv"],
    extractFromPage() {
      if (!location.pathname.match(/\/video\//)) return [];
      const meta = getPageMetadata();
      return [makeYtdlpItem(meta)];
    },
  });

  // --- Generic extractor (fallback for any page with <video>) ---
  // Covers all 1700+ yt-dlp supported sites without individual extractors.
  // Only needs to detect that the page IS a video page — the page URL is sent
  // to yt-dlp which handles the actual stream extraction server-side.
  let lastGenericSentUrl = "";

  SITE_EXTRACTORS.push({
    name: "generic",
    matches: [], // Never matched by domain — used as fallback only
    extractFromPage() {
      // Check for any <video> element — src may be empty, blob:, or direct URL.
      // Many sites use MediaSource/blob: for playback, so we can't require a src.
      const videos = document.querySelectorAll("video");
      if (videos.length === 0) return [];
      // Require a page title to distinguish video pages from pages with
      // incidental video elements (e.g. background decoration).
      const meta = getPageMetadata();
      if (!meta.title) return [];
      // Always verify at least one <video> looks like a real player.
      // Some sites set og:type="video" but deliver content
      // via iframes — their <video> elements are all empty placeholders.
      // og:type is used as a secondary signal to relax the dimension threshold.
      const ogType = document.querySelector('meta[property="og:type"]')?.content || "";
      const ogVideo = document.querySelector('meta[property="og:video"], meta[property="og:video:url"]')?.content || "";
      const isVideoPage = ogType.startsWith("video") || !!ogVideo;
      let hasPlayer = false;
      for (const v of videos) {
        const src = v.currentSrc || v.src || "";
        // Video with a real source (http, blob) is a player
        if (src && src !== "about:blank") { hasPlayer = true; break; }
        // Video with significant visible dimensions is likely a player.
        // On og:type="video" pages, use a lower threshold (player may be
        // initializing), otherwise require a clearly visible player.
        const minW = isVideoPage ? 100 : 300;
        const minH = isVideoPage ? 50 : 150;
        if (v.offsetWidth >= minW && v.offsetHeight >= minH) { hasPlayer = true; break; }
      }
      if (!hasPlayer) return [];
      return [makeYtdlpItem(meta)];
    },
  });

  // --- Site extractor runner ---

  function getCurrentSiteExtractor() {
    const hostname = location.hostname;
    for (const extractor of SITE_EXTRACTORS) {
      if (extractor.matches.length > 0 && extractor.matches.some(m => hostname.includes(m))) {
        return extractor;
      }
    }
    // Fallback: return the generic extractor for any page
    return SITE_EXTRACTORS.find(e => e.name === "generic") || null;
  }

  // Run page extraction for the current site
  function runSitePageExtraction() {
    const extractor = getCurrentSiteExtractor();
    if (!extractor) return;

    // Generic extractor dedup: skip if we already sent for this URL.
    // Only activate the guard once document.readyState leaves "loading",
    // because content.js (ISOLATED world, document_idle) may not have
    // registered its postMessage listener during the early "loading" phase.
    if (extractor.name === "generic" && location.href === lastGenericSentUrl
        && document.readyState !== "loading") {
      return;
    }

    const items = extractor.extractFromPage();
    if (items.length > 0) {
      window.postMessage({
        type: "dlrelay-site-from-main",
        items,
      }, window.location.origin);
      if (extractor.name === "generic") lastGenericSentUrl = location.href;
    }
  }

  const siteExtractor = getCurrentSiteExtractor();
  if (siteExtractor) {
    // Debounced re-extraction for SPA navigation / dynamic content.
    // Uses its own timer, separate from the initial extraction.
    let reExtractTimer = null;
    function scheduleReExtraction(delayMs) {
      if (reExtractTimer) clearTimeout(reExtractTimer);
      reExtractTimer = setTimeout(runSitePageExtraction, delayMs);
    }

    // Extraction triggers — multiple to handle various page load timings.
    // content.js (ISOLATED world) runs at document_idle, so its postMessage
    // listener may not exist when early timers fire. DOMContentLoaded and load
    // events ensure extraction runs after content.js is ready.
    // Extractors are idempotent (lastGenericSentUrl / lastExtractedYouTubeVideoId
    // prevent duplicate posts once content.js is ready), so extra calls are harmless.
    setTimeout(runSitePageExtraction, 2000);
    document.addEventListener("DOMContentLoaded", () => {
      setTimeout(runSitePageExtraction, 1000);
    });
    window.addEventListener("load", () => {
      setTimeout(runSitePageExtraction, 1000);
    });
    // Late retry for very slow pages (e.g. ad-heavy sites where video loads late)
    setTimeout(runSitePageExtraction, 8000);

    // Unified SPA navigation handler
    let lastUrl = location.href;
    function onNavigate() {
      const newUrl = location.href;
      if (newUrl === lastUrl) return;
      lastUrl = newUrl;
      lastExtractedYouTubeVideoId = "";
      lastGenericSentUrl = "";
      scheduleReExtraction(3000);
    }

    // Hook pushState/replaceState for SPA navigation detection
    const origPushState = history.pushState;
    const origReplaceState = history.replaceState;
    history.pushState = function (...args) {
      origPushState.apply(this, args);
      onNavigate();
    };
    history.replaceState = function (...args) {
      origReplaceState.apply(this, args);
      onNavigate();
    };
    window.addEventListener("popstate", onNavigate);

    // YouTube-specific: listen for YouTube's own SPA navigation event
    window.addEventListener("yt-navigate-finish", () => {
      lastExtractedYouTubeVideoId = "";
      lastGenericSentUrl = "";
      scheduleReExtraction(2000);
    });

    // URL polling fallback for sites that don't use pushState
    setInterval(() => { onNavigate(); }, 2000);

    // Re-run on DOM changes (debounced, cannot cancel the initial timer)
    const siteObserver = new MutationObserver(() => {
      scheduleReExtraction(2000);
    });
    siteObserver.observe(document.documentElement, {
      childList: true,
      subtree: true,
    });
  }
})();
