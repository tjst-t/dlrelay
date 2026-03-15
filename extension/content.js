// DL Relay — Content Script (ISOLATED world)
// Detects video/audio elements in the DOM and reports them to the background script.

(function () {
  "use strict";

  const MEDIA_SELECTORS = "video, audio, video source, audio source";

  const DETECTED_URLS = new Set();

  function collectMediaUrls() {
    const items = [];
    const elements = document.querySelectorAll(MEDIA_SELECTORS);

    for (const el of elements) {
      let url = "";

      if (el.tagName === "VIDEO" || el.tagName === "AUDIO") {
        url = el.currentSrc || el.src || "";
      } else if (el.tagName === "SOURCE") {
        url = el.src || "";
      }

      if (!url) continue;
      if (url.startsWith("blob:") || url.startsWith("data:")) continue;
      // Reject protocol-relative or other non-http URLs
      if (!url.startsWith("http://") && !url.startsWith("https://")) continue;
      if (DETECTED_URLS.has(url)) continue;

      DETECTED_URLS.add(url);
      items.push({
        url,
        mimeType: el.type || "",
      });
    }

    return items;
  }

  function reportMedia() {
    const items = collectMediaUrls();
    if (items.length > 0) {
      chrome.runtime.sendMessage({
        type: "dlrelay-media-detected",
        items,
      });
    }
  }

  // Send page metadata (title, thumbnail, duration) to background
  function reportPageMetadata() {
    const title = document.title || "";
    const thumbnail =
      document.querySelector('meta[property="og:image"]')?.content ||
      document.querySelector('meta[name="twitter:image"]')?.content ||
      document.querySelector('link[rel="image_src"]')?.href ||
      "";

    // Try to get video duration
    let duration = 0;
    const durationMeta = document.querySelector('meta[itemprop="duration"]')?.content;
    if (durationMeta) {
      // Parse ISO 8601 duration (PT2H9M33S)
      const match = durationMeta.match(/PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?/);
      if (match) {
        duration = (parseInt(match[1] || 0) * 3600) + (parseInt(match[2] || 0) * 60) + parseInt(match[3] || 0);
      }
    }
    if (!duration) {
      const video = document.querySelector("video");
      if (video && video.duration && isFinite(video.duration)) {
        duration = Math.round(video.duration);
      }
    }

    if (title || thumbnail || duration) {
      chrome.runtime.sendMessage({
        type: "dlrelay-page-metadata",
        title,
        thumbnail,
        duration,
      });
    }
  }

  // Report metadata after page loads
  if (document.readyState === "complete") {
    setTimeout(reportPageMetadata, 1000);
  } else {
    window.addEventListener("load", () => setTimeout(reportPageMetadata, 1000));
  }
  // Also report when video metadata loads
  document.addEventListener("loadedmetadata", (e) => {
    if (e.target.tagName === "VIDEO") {
      setTimeout(reportPageMetadata, 500);
    }
  }, true);

  // Initial scan
  reportMedia();

  // Watch for dynamically added media elements
  const observer = new MutationObserver((mutations) => {
    let hasNewMedia = false;
    for (const mutation of mutations) {
      for (const node of mutation.addedNodes) {
        if (node.nodeType !== Node.ELEMENT_NODE) continue;
        if (node.matches && node.matches(MEDIA_SELECTORS)) {
          hasNewMedia = true;
          break;
        }
        if (node.querySelector && node.querySelector(MEDIA_SELECTORS)) {
          hasNewMedia = true;
          break;
        }
      }
      if (hasNewMedia) break;
    }
    if (hasNewMedia) {
      reportMedia();
    }
  });

  observer.observe(document.documentElement, {
    childList: true,
    subtree: true,
  });

  // Listen for messages from MAIN world content script
  window.addEventListener("message", (event) => {
    if (event.source !== window) return;
    if (event.data && event.data.type === "dlrelay-blob-from-main") {
      chrome.runtime.sendMessage({
        type: "dlrelay-blob-detected",
        items: event.data.items || [],
      });
    }
    if (event.data && event.data.type === "dlrelay-site-from-main") {
      chrome.runtime.sendMessage({
        type: "dlrelay-site-detected",
        items: event.data.items || [],
      });
    }
  });
})();
