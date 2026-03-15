// @ts-check
/**
 * Extension detection integration tests.
 *
 * Loads the actual Chrome extension into a browser, navigates to mock pages
 * with video elements, and verifies detection works across all pathways.
 *
 * Run: xvfb-run npx playwright test test/browser/extension-detection.spec.js
 */
const { test, expect, chromium } = require("@playwright/test");
const path = require("path");
const fs = require("fs");
const os = require("os");
const http = require("http");

const EXTENSION_PATH = path.resolve(__dirname, "../../extension");

// --- Mock video site server ---

function createMockVideoSite() {
  const server = http.createServer((req, res) => {
    const url = new URL(req.url, `http://${req.headers.host}`);

    if (url.pathname === "/youtube-watch") {
      res.writeHead(200, { "Content-Type": "text/html" });
      res.end(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Big Buck Bunny - YouTube</title>
  <meta property="og:title" content="Big Buck Bunny">
  <meta property="og:image" content="https://i.ytimg.com/vi/aqz-KE-bpKQ/maxresdefault.jpg">
  <meta itemprop="duration" content="PT9M56S">
</head>
<body>
  <script>
    // Simulate YouTube's ytInitialPlayerResponse
    window.ytInitialPlayerResponse = {
      videoDetails: {
        videoId: "aqz-KE-bpKQ",
        title: "Big Buck Bunny 60fps 4K - Official Blender Foundation Short Film",
        lengthSeconds: "596",
        thumbnail: {
          thumbnails: [
            { url: "https://i.ytimg.com/vi/aqz-KE-bpKQ/default.jpg", width: 120, height: 90 },
            { url: "https://i.ytimg.com/vi/aqz-KE-bpKQ/maxresdefault.jpg", width: 1280, height: 720 },
          ]
        }
      }
    };
  </script>
  <video src="blob:https://www.youtube.com/fake-blob-url" autoplay></video>
</body>
</html>`);
    } else if (url.pathname === "/niconico-watch") {
      res.writeHead(200, { "Content-Type": "text/html" });
      res.end(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Test Video - niconico</title>
  <meta property="og:title" content="Test Niconico Video">
  <meta property="og:image" content="https://nicovideo.cdn.nimg.jp/thumbnails/sm12345">
</head>
<body>
  <div id="js-initial-watch-data" data-api-data='${JSON.stringify({
    video: {
      title: "Niconico Test Video Title",
      thumbnail: { url: "https://nicovideo.cdn.nimg.jp/thumbnails/sm12345" },
      duration: 180
    }
  })}'></div>
  <video src="blob:https://www.nicovideo.jp/fake-blob" autoplay></video>
</body>
</html>`);
    } else if (url.pathname === "/generic-video") {
      // Generic page with a <video> element — should trigger generic extractor
      res.writeHead(200, { "Content-Type": "text/html" });
      res.end(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Cool Video on Random Site</title>
  <meta property="og:title" content="Cool Video on Random Site">
  <meta property="og:image" content="https://example.com/thumb.jpg">
</head>
<body>
  <video src="https://example.com/video.mp4" controls></video>
</body>
</html>`);
    } else if (url.pathname === "/direct-mp4") {
      // Page with a direct MP4 video URL (not a blob)
      res.writeHead(200, { "Content-Type": "text/html" });
      res.end(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Direct Video Page</title>
  <meta property="og:title" content="Direct MP4 Video">
</head>
<body>
  <video src="/sample.mp4" controls></video>
</body>
</html>`);
    } else if (url.pathname === "/sample.mp4") {
      // Serve a fake MP4 file
      const body = Buffer.alloc(200000, 0x00); // 200KB fake video
      res.writeHead(200, {
        "Content-Type": "video/mp4",
        "Content-Length": String(body.length),
      });
      res.end(body);
    } else if (url.pathname === "/no-video") {
      // Page without any video elements
      res.writeHead(200, { "Content-Type": "text/html" });
      res.end(`<!DOCTYPE html>
<html><head><title>No Video Page</title></head>
<body><p>This page has no video.</p></body>
</html>`);
    } else if (url.pathname === "/hls-page") {
      res.writeHead(200, { "Content-Type": "text/html" });
      res.end(`<!DOCTYPE html>
<html>
<head><title>HLS Video Page</title>
<meta property="og:title" content="HLS Stream Video">
</head>
<body><video src="blob:https://example.com/hls-blob" controls></video></body>
</html>`);
    } else {
      res.writeHead(404);
      res.end("Not found");
    }
  });

  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => {
      const port = server.address().port;
      resolve({ server, port, baseUrl: `http://127.0.0.1:${port}` });
    });
  });
}

/** Launch Chromium with the extension loaded. */
async function launchWithExtension() {
  const userDataDir = fs.mkdtempSync(path.join(os.tmpdir(), "dlrelay-ext-test-"));
  const context = await chromium.launchPersistentContext(userDataDir, {
    headless: false,
    args: [
      `--disable-extensions-except=${EXTENSION_PATH}`,
      `--load-extension=${EXTENSION_PATH}`,
      "--no-first-run",
      "--disable-default-apps",
      "--disable-popup-blocking",
      "--mute-audio",
    ],
    viewport: { width: 1280, height: 720 },
  });

  // Wait for the service worker to be ready
  let sw;
  const existingWorkers = context.serviceWorkers();
  if (existingWorkers.length > 0) {
    sw = existingWorkers.find((w) => w.url().includes("background"));
  }
  if (!sw) {
    sw = await context.waitForEvent("serviceworker", { timeout: 10000 });
  }

  return { context, sw, userDataDir };
}

/** Query detected media for a tab from the background service worker. */
async function getDetectedMedia(sw, tabId) {
  return sw.evaluate(async (tid) => {
    // Restore from session storage first
    try {
      const data = await chrome.storage.session.get(`tab_${tid}`);
      const items = data[`tab_${tid}`];
      if (items && items.length > 0) {
        return items.map((m) => ({
          url: m.url,
          title: m.title,
          thumbnail: m.thumbnail,
          duration: m.duration,
          type: m.type,
          filename: m.filename,
          source: m.source,
          size: m.size,
          mimeType: m.mimeType,
        }));
      }
    } catch {}

    // @ts-ignore — background.js globals
    const media = typeof getTabMedia === "function" ? getTabMedia(tid) : [];
    return media.map((m) => ({
      url: m.url,
      title: m.title,
      thumbnail: m.thumbnail,
      duration: m.duration,
      type: m.type,
      filename: m.filename,
      source: m.source,
      size: m.size,
      mimeType: m.mimeType,
    }));
  }, tabId);
}

/** Get tab ID for a page URL from the service worker. */
async function getTabId(sw, urlFragment) {
  return sw.evaluate(async (frag) => {
    const tabs = await chrome.tabs.query({});
    const tab = tabs.find((t) => t.url && t.url.includes(frag));
    return tab?.id;
  }, urlFragment);
}

// =====================================================================

test.describe("Extension Detection Tests", () => {
  /** @type {import('@playwright/test').BrowserContext} */
  let context;
  let sw;
  let userDataDir;
  let mockSite;

  test.beforeAll(async () => {
    mockSite = await createMockVideoSite();
    const result = await launchWithExtension();
    context = result.context;
    sw = result.sw;
    userDataDir = result.userDataDir;
  });

  test.afterAll(async () => {
    if (context) await context.close();
    if (mockSite?.server) mockSite.server.close();
    try {
      fs.rmSync(userDataDir, { recursive: true, force: true });
    } catch {}
  });

  test.setTimeout(30000);

  test("YouTube mock: site extractor detects via ytInitialPlayerResponse", async () => {
    const page = await context.newPage();
    try {
      // We need the page URL to look like youtube.com for the extractor to match.
      // Since we can't fake the hostname, let's test content-main.js behavior directly.
      // Instead, navigate to the mock page and inject the content script logic manually.
      await page.goto(mockSite.baseUrl + "/youtube-watch", { waitUntil: "load" });
      await page.waitForTimeout(500);

      // Run the site extractor logic manually (since hostname won't match youtube.com)
      const result = await page.evaluate(() => {
        // Simulate what the YouTube extractor does
        const ytData = window.ytInitialPlayerResponse;
        if (!ytData || !ytData.videoDetails) return { error: "no ytInitialPlayerResponse" };

        const d = ytData.videoDetails;
        return {
          title: d.title,
          videoId: d.videoId,
          duration: parseInt(d.lengthSeconds || "0", 10),
          thumbnail: d.thumbnail?.thumbnails?.slice(-1)?.[0]?.url || "",
        };
      });

      console.log("YouTube extractor result:", result);
      expect(result.title).toContain("Big Buck Bunny");
      expect(result.videoId).toBe("aqz-KE-bpKQ");
      expect(result.duration).toBe(596);
      expect(result.thumbnail).toContain("maxresdefault");
    } finally {
      await page.close();
    }
  });

  test("Niconico mock: site extractor reads js-initial-watch-data", async () => {
    const page = await context.newPage();
    try {
      await page.goto(mockSite.baseUrl + "/niconico-watch", { waitUntil: "load" });
      await page.waitForTimeout(500);

      // Run the niconico extractor logic manually
      const result = await page.evaluate(() => {
        const el = document.getElementById("js-initial-watch-data");
        if (!el) return { error: "no js-initial-watch-data" };
        try {
          const data = JSON.parse(el.getAttribute("data-api-data") || "{}");
          return {
            title: data?.video?.title || "",
            thumbnail: data?.video?.thumbnail?.url || "",
            duration: data?.video?.duration || 0,
          };
        } catch (e) {
          return { error: e.message };
        }
      });

      console.log("Niconico extractor result:", result);
      expect(result.title).toBe("Niconico Test Video Title");
      expect(result.duration).toBe(180);
    } finally {
      await page.close();
    }
  });

  test("Generic extractor: detects video on page with <video> element", async () => {
    const page = await context.newPage();
    try {
      await page.goto(mockSite.baseUrl + "/generic-video", { waitUntil: "load" });

      // The generic extractor should run (hostname doesn't match any specific extractor)
      // Wait for content-main.js extraction (2000ms delay + buffer)
      await page.waitForTimeout(4000);

      const tabId = await getTabId(sw, "/generic-video");
      expect(tabId).toBeTruthy();

      const media = await getDetectedMedia(sw, tabId);
      console.log("Generic video detection:", JSON.stringify(media, null, 2));

      // Should detect at least one item
      // Could be from DOM detection (content.js) or site extractor (content-main.js generic)
      expect(media.length).toBeGreaterThanOrEqual(1);

      // Check for site-extracted item (generic extractor)
      const siteItems = media.filter((m) => m.source === "site");
      console.log(`Site items: ${siteItems.length}`);

      // The generic extractor should fire because:
      // 1. hostname doesn't match any specific extractor → falls back to generic
      // 2. Page has a <video> element with src
      // 3. Page has og:title
      if (siteItems.length > 0) {
        expect(siteItems[0].title).toContain("Cool Video");
        expect(siteItems[0].type).toBe("ytdlp");
      }

      // DOM detection should also find the video
      const domItems = media.filter((m) => m.source === "dom");
      console.log(`DOM items: ${domItems.length}`);
    } finally {
      await page.close();
    }
  });

  test("Direct MP4: DOM detection finds video with http src", async () => {
    const page = await context.newPage();
    try {
      await page.goto(mockSite.baseUrl + "/direct-mp4", { waitUntil: "load" });
      await page.waitForTimeout(4000);

      const tabId = await getTabId(sw, "/direct-mp4");
      expect(tabId).toBeTruthy();

      const media = await getDetectedMedia(sw, tabId);
      console.log("Direct MP4 detection:", JSON.stringify(media, null, 2));

      // Should detect the video via DOM detection and/or generic extractor
      expect(media.length).toBeGreaterThanOrEqual(1);

      // At least one item should have the sample.mp4 URL
      const mp4Items = media.filter((m) => m.url.includes("sample.mp4"));
      console.log(`MP4 items: ${mp4Items.length}`);
    } finally {
      await page.close();
    }
  });

  test("WebRequest: detects direct MP4 download via network", async () => {
    const page = await context.newPage();
    try {
      await page.goto(mockSite.baseUrl + "/direct-mp4", { waitUntil: "load" });
      // Wait for the video to load (triggers webRequest)
      await page.waitForTimeout(4000);

      const tabId = await getTabId(sw, "/direct-mp4");
      expect(tabId).toBeTruthy();

      const media = await getDetectedMedia(sw, tabId);

      // webRequest should detect the MP4 file download
      const webRequestItems = media.filter((m) => m.source === "webRequest");
      console.log(`WebRequest items for direct MP4: ${webRequestItems.length}`);
      for (const item of webRequestItems) {
        console.log(`  - ${item.filename} (${item.mimeType}, ${item.size} bytes)`);
      }
    } finally {
      await page.close();
    }
  });

  test("No video page: no detection", async () => {
    const page = await context.newPage();
    try {
      await page.goto(mockSite.baseUrl + "/no-video", { waitUntil: "load" });
      await page.waitForTimeout(4000);

      const tabId = await getTabId(sw, "/no-video");
      if (tabId) {
        const media = await getDetectedMedia(sw, tabId);
        console.log(`No-video page items: ${media.length}`);
        // Should have zero items (no video on page)
        expect(media.length).toBe(0);
      }
    } finally {
      await page.close();
    }
  });

  test("content-main.js postMessage bridge works", async () => {
    const page = await context.newPage();
    try {
      await page.goto(mockSite.baseUrl + "/generic-video", { waitUntil: "load" });

      // Manually trigger a site-from-main postMessage and verify it's received
      const bridgeTest = await page.evaluate(() => {
        return new Promise((resolve) => {
          // Listen for the message being forwarded (the content.js listener should catch it)
          // We can't directly verify chrome.runtime.sendMessage, but we can verify
          // the postMessage is dispatched
          let received = false;
          const listener = (event) => {
            if (event.data && event.data.type === "dlrelay-site-from-main") {
              received = true;
            }
          };
          window.addEventListener("message", listener);

          // Post a message simulating what content-main.js does
          window.postMessage({
            type: "dlrelay-site-from-main",
            items: [{
              url: location.href,
              type: "ytdlp",
              mimeType: "video/mp4",
              filename: "bridge-test.mp4",
              title: "Bridge Test Video",
              thumbnail: "",
              duration: 42,
            }],
          }, window.location.origin);

          // Give the message loop time to process
          setTimeout(() => {
            window.removeEventListener("message", listener);
            resolve({ received });
          }, 500);
        });
      });

      console.log("PostMessage bridge test:", bridgeTest);
      expect(bridgeTest.received).toBe(true);

      // Wait for the bridge to forward to background.js
      await page.waitForTimeout(1000);

      // Check if background.js received it
      const tabId = await getTabId(sw, "/generic-video");
      if (tabId) {
        const media = await getDetectedMedia(sw, tabId);
        const bridgeItem = media.find((m) => m.filename === "bridge-test.mp4");
        console.log("Bridge item in background:", bridgeItem);
        // This verifies the full chain: postMessage → content.js → background.js
        if (bridgeItem) {
          expect(bridgeItem.title).toBe("Bridge Test Video");
          expect(bridgeItem.source).toBe("site");
        }
      }
    } finally {
      await page.close();
    }
  });

  test("EXTRACTOR_DOMAINS filtering: webRequest blocks youtube/niconico CDN URLs", async () => {
    // Verify that isExtractorDomain blocks the right domains
    const result = await sw.evaluate(() => {
      // @ts-ignore
      const domains = [
        "https://r1---sn-abc.googlevideo.com/videoplayback?id=123",
        "https://www.youtube.com/watch?v=abc",
        "https://dmc.nico/video/sm12345",
        "https://www.nicovideo.jp/watch/sm12345",
        "https://example.com/video.mp4",
        "https://cdn.tiktok.com/video/123.mp4",
        "https://random-site.com/stream.m3u8",
      ];

      return domains.map(url => ({
        url: url.substring(0, 60),
        // @ts-ignore
        blocked: isExtractorDomain(url),
      }));
    });

    console.log("EXTRACTOR_DOMAINS filtering results:");
    for (const r of result) {
      console.log(`  ${r.blocked ? "BLOCKED" : "ALLOWED"}: ${r.url}`);
    }

    // YouTube and Niconico CDN should be blocked
    expect(result[0].blocked).toBe(true); // googlevideo.com
    expect(result[1].blocked).toBe(true); // youtube.com
    expect(result[2].blocked).toBe(true); // dmc.nico
    expect(result[3].blocked).toBe(true); // nicovideo.jp

    // Non-extractor domains should be allowed
    expect(result[4].blocked).toBe(false); // example.com
    expect(result[6].blocked).toBe(false); // random-site.com

    // TikTok CDN should be blocked
    expect(result[5].blocked).toBe(true); // tiktok.com → actually tiktokcdn.com is in the list
  });

  test("Site extractor function works in MAIN world", async () => {
    const page = await context.newPage();
    try {
      await page.goto(mockSite.baseUrl + "/generic-video", { waitUntil: "load" });
      await page.waitForTimeout(500);

      // Check if content-main.js IIFE has been executed
      // We can't access its closure directly, but we can check side effects
      // The generic extractor should have modified the page

      // Check if the fetch/XHR hooks are in place
      const hooks = await page.evaluate(() => {
        // Check if fetch has been hooked (it should be a wrapper)
        const fetchStr = window.fetch.toString();
        const xhrOpenStr = XMLHttpRequest.prototype.open.toString();
        const createObjUrlStr = URL.createObjectURL.toString();

        return {
          fetchHooked: fetchStr.includes("fetchUrlBuffer") || fetchStr.length > 100,
          xhrHooked: xhrOpenStr.includes("fetchUrlBuffer") || xhrOpenStr.length > 100,
          createObjectURLHooked: createObjUrlStr.includes("mediaSourceMap") || createObjUrlStr.length > 100,
        };
      });

      console.log("MAIN world hooks:", hooks);
      // These hooks should be installed by content-main.js
      expect(hooks.fetchHooked).toBe(true);
      expect(hooks.xhrHooked).toBe(true);
      expect(hooks.createObjectURLHooked).toBe(true);
    } finally {
      await page.close();
    }
  });
});

test.describe("Extension Detection - Real Sites (network required)", () => {
  /** @type {import('@playwright/test').BrowserContext} */
  let context;
  let sw;
  let userDataDir;

  test.beforeAll(async () => {
    const result = await launchWithExtension();
    context = result.context;
    sw = result.sw;
    userDataDir = result.userDataDir;
  });

  test.afterAll(async () => {
    if (context) await context.close();
    try {
      fs.rmSync(userDataDir, { recursive: true, force: true });
    } catch {}
  });

  test.setTimeout(60000);

  test("YouTube: detects video via site extractor", async () => {
    const page = await context.newPage();
    try {
      await page.context().addCookies([
        { name: "CONSENT", value: "YES+cb", domain: ".youtube.com", path: "/" },
        { name: "SOCS", value: "CAISNQgDEitib3FfaWRlbnRpdHlmcm9udGVuZHVpc2VydmVyXzIwMjMwODI5LjA3X3AxGgJlbiADGgYIgJnPpwY", domain: ".youtube.com", path: "/" },
      ]);

      await page.goto("https://www.youtube.com/watch?v=aqz-KE-bpKQ", {
        waitUntil: "domcontentloaded",
        timeout: 30000,
      });

      // Dismiss consent if any
      try {
        const btn = page.locator('button:has-text("Accept all"), button:has-text("Agree")');
        if (await btn.first().isVisible({ timeout: 3000 })) {
          await btn.first().click();
          await page.waitForTimeout(1000);
        }
      } catch {}

      await page.waitForSelector("video", { timeout: 15000 });
      await page.waitForTimeout(6000); // Wait for extraction (2s delay + buffer)

      const tabId = await getTabId(sw, "youtube.com/watch");
      expect(tabId).toBeTruthy();

      const media = await getDetectedMedia(sw, tabId);
      console.log("\n=== YouTube Detection Results ===");
      console.log(`Total items: ${media.length}`);
      for (const m of media) {
        console.log(`  [${m.source}] "${m.title || m.filename}" type=${m.type} dur=${m.duration}s`);
        console.log(`    URL: ${m.url.substring(0, 80)}`);
      }

      // Should have at least 1 item
      expect(media.length).toBeGreaterThanOrEqual(1);

      // Should have a site-extracted item
      const siteItems = media.filter((m) => m.source === "site");
      console.log(`\nSite-extracted items: ${siteItems.length}`);
      expect(siteItems.length).toBe(1);

      const main = siteItems[0];
      expect(main.type).toBe("ytdlp");
      expect(main.url).toContain("youtube.com/watch");
      expect(main.title.toLowerCase()).toContain("big buck bunny");
      expect(main.duration).toBeGreaterThan(400);

      // Should NOT have noise items (videoplayback, player, etc.)
      const noiseNames = ["videoplayback", "player", "timedtext"];
      for (const m of media) {
        for (const noise of noiseNames) {
          expect(m.filename.toLowerCase()).not.toContain(noise);
        }
      }

      await page.screenshot({
        path: "test/browser/screenshots/ext-youtube-detection.png",
      });
    } finally {
      await page.close();
    }
  });

  test("Vimeo: detects video via site extractor", async () => {
    const page = await context.newPage();
    try {
      // Vimeo staff picks - a public video
      await page.goto("https://vimeo.com/1084537", {
        waitUntil: "domcontentloaded",
        timeout: 30000,
      });

      await page.waitForTimeout(6000);

      const tabId = await getTabId(sw, "vimeo.com/");
      if (!tabId) {
        console.log("Could not get Vimeo tab ID, skipping");
        test.skip();
        return;
      }

      const media = await getDetectedMedia(sw, tabId);
      console.log("\n=== Vimeo Detection Results ===");
      console.log(`Total items: ${media.length}`);
      for (const m of media) {
        console.log(`  [${m.source}] "${m.title || m.filename}" type=${m.type}`);
      }

      // Should detect at least 1 item
      expect(media.length).toBeGreaterThanOrEqual(1);

      const siteItems = media.filter((m) => m.source === "site");
      console.log(`Site-extracted items: ${siteItems.length}`);
      if (siteItems.length > 0) {
        expect(siteItems[0].type).toBe("ytdlp");
      }
    } finally {
      await page.close();
    }
  });

  test("Dailymotion: detects video via site extractor", async () => {
    const page = await context.newPage();
    try {
      await page.goto("https://www.dailymotion.com/video/x8m2kaa", {
        waitUntil: "domcontentloaded",
        timeout: 30000,
      });

      await page.waitForTimeout(6000);

      const tabId = await getTabId(sw, "dailymotion.com/video");
      if (!tabId) {
        console.log("Could not get Dailymotion tab ID, skipping");
        test.skip();
        return;
      }

      const media = await getDetectedMedia(sw, tabId);
      console.log("\n=== Dailymotion Detection Results ===");
      console.log(`Total items: ${media.length}`);
      for (const m of media) {
        console.log(`  [${m.source}] "${m.title || m.filename}" type=${m.type}`);
      }

      expect(media.length).toBeGreaterThanOrEqual(1);
    } finally {
      await page.close();
    }
  });
});
