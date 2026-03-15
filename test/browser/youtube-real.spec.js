// @ts-check
/**
 * Real YouTube integration tests for the DL Relay extension.
 *
 * These tests load the actual Chrome extension into a browser, navigate to
 * real YouTube pages, and verify that the extension correctly detects videos.
 *
 * Requirements:
 *   - Xvfb (for headed Chromium; extensions don't work in headless mode)
 *   - Run via: xvfb-run npx playwright test test/browser/youtube-real.spec.js
 *
 * These tests hit the real YouTube, so they are slower and may be flaky
 * if YouTube changes their page structure. They are meant to be run manually
 * or in CI with `xvfb-run`, NOT as part of the default test suite.
 */
const { test, expect, chromium } = require("@playwright/test");
const path = require("path");
const fs = require("fs");
const os = require("os");

const EXTENSION_PATH = path.resolve(__dirname, "../../extension");

// YouTube video: short, publicly available, unlikely to be removed
// Using a Creative Commons / public domain video
const TEST_VIDEOS = [
  {
    // Big Buck Bunny (Blender Foundation, CC-BY)
    url: "https://www.youtube.com/watch?v=aqz-KE-bpKQ",
    titleContains: "Big Buck Bunny",
    minDuration: 500, // ~10 min
  },
];

/** Launch Chromium with the extension loaded. */
async function launchWithExtension() {
  const userDataDir = fs.mkdtempSync(path.join(os.tmpdir(), "dlrelay-test-"));
  const context = await chromium.launchPersistentContext(userDataDir, {
    headless: false, // Extensions require headed mode
    args: [
      `--disable-extensions-except=${EXTENSION_PATH}`,
      `--load-extension=${EXTENSION_PATH}`,
      "--no-first-run",
      "--disable-default-apps",
      "--disable-popup-blocking",
      "--disable-translate",
      "--disable-sync",
      "--mute-audio",
      // Bypass YouTube consent page (EEA cookie wall)
      "--disable-features=PrivacySandboxSettings4",
    ],
    viewport: { width: 1280, height: 720 },
    locale: "en-US",
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
    // @ts-ignore — background.js globals
    const media = typeof getTabMedia === "function" ? getTabMedia(tid) : [];
    return media.map((m) => ({
      url: m.url,
      title: m.title,
      thumbnail: m.thumbnail,
      duration: m.duration,
      resolution: m.resolution,
      type: m.type,
      filename: m.filename,
      mimeType: m.mimeType,
      variantCount: (m.variants || []).length,
      source: m.source,
    }));
  }, tabId);
}

/** Known YouTube noise filenames that should NOT appear in the popup. */
const YOUTUBE_NOISE_NAMES = [
  "videoplayback",
  "timedtext",
  "get_watch",
  "player_api",
  "heartbeat",
  "log_event",
  "generate_204",
];

/** Dismiss YouTube consent dialog if it appears. */
async function dismissYouTubeConsent(page) {
  try {
    // Look for the consent form and accept it
    const acceptBtn = page.locator(
      'button[aria-label*="Accept"], button:has-text("Accept all"), button:has-text("Agree"), form[action*="consent"] button'
    );
    if (await acceptBtn.first().isVisible({ timeout: 3000 })) {
      await acceptBtn.first().click();
      await page.waitForTimeout(1000);
    }
  } catch {
    // No consent dialog, that's fine
  }
}

test.describe("YouTube Real Detection", () => {
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
    // Clean up temp dir
    try {
      fs.rmSync(userDataDir, { recursive: true, force: true });
    } catch { /* ignore */ }
  });

  // Give YouTube time to load
  test.setTimeout(60000);

  for (const video of TEST_VIDEOS) {
    test(`detects video: ${video.titleContains}`, async () => {
      const page = await context.newPage();

      try {
        // Set cookies to skip consent
        await page.context().addCookies([
          { name: "CONSENT", value: "YES+cb", domain: ".youtube.com", path: "/" },
          { name: "SOCS", value: "CAISNQgDEitib3FfaWRlbnRpdHlmcm9udGVuZHVpc2VydmVyXzIwMjMwODI5LjA3X3AxGgJlbiADGgYIgJnPpwY", domain: ".youtube.com", path: "/" },
        ]);

        await page.goto(video.url, { waitUntil: "domcontentloaded", timeout: 30000 });
        await dismissYouTubeConsent(page);

        // Wait for the video player to be ready
        await page.waitForSelector("video", { timeout: 15000 });

        // Wait for the content script extraction to complete
        // content-main.js has a 2s delay on extraction
        await page.waitForTimeout(5000);

        // Get the tab ID
        const pages = context.pages();
        const ytPage = pages.find((p) => p.url().includes("youtube.com/watch"));
        expect(ytPage).toBeTruthy();

        // Query detected media via the service worker
        // First, find the tab ID from the service worker
        const tabId = await sw.evaluate(async (url) => {
          // @ts-ignore
          const tabs = await chrome.tabs.query({ url: url + "*" });
          return tabs[0]?.id;
        }, video.url.split("?")[0]);

        expect(tabId).toBeTruthy();
        const media = await getDetectedMedia(sw, tabId);

        console.log(`\n=== Detected media for "${video.titleContains}" ===`);
        console.log(`Total items: ${media.length}`);
        for (const m of media) {
          console.log(`  - [${m.source}] ${m.title || m.filename} (${m.type}, ${m.resolution}, duration=${m.duration}s, variants=${m.variantCount})`);
          console.log(`    URL: ${m.url.substring(0, 100)}...`);
        }
        console.log("");

        // --- Assertions ---

        // Should detect at least 1 video
        expect(media.length).toBeGreaterThanOrEqual(1);

        // Find the main video (from site extractor)
        const siteItems = media.filter((m) => m.source === "site");
        console.log(`Site-extracted items: ${siteItems.length}`);

        // The site extractor should produce exactly 1 item for this video
        expect(siteItems.length).toBe(1);

        const mainVideo = siteItems[0];

        // Title should contain expected text
        expect(mainVideo.title.toLowerCase()).toContain(
          video.titleContains.toLowerCase()
        );

        // Should have a thumbnail
        expect(mainVideo.thumbnail).toBeTruthy();
        expect(mainVideo.thumbnail).toMatch(/^https?:\/\//);

        // Should have a duration (in seconds)
        expect(mainVideo.duration).toBeGreaterThanOrEqual(video.minDuration);

        // Should use yt-dlp method (URL is the page URL, not a stream URL)
        expect(mainVideo.type).toBe("ytdlp");
        expect(mainVideo.url).toContain("youtube.com/watch");

        console.log(`✓ Main video: "${mainVideo.title}", ${mainVideo.duration}s, type=${mainVideo.type}`);

        // No protocol-relative URLs should be present
        for (const m of media) {
          expect(m.url).toMatch(/^https?:\/\//);
        }

        // No YouTube noise items (timedtext, videoplayback, etc.) should be present
        for (const m of media) {
          const fname = (m.filename || "").toLowerCase();
          for (const noise of YOUTUBE_NOISE_NAMES) {
            expect(fname).not.toContain(noise);
          }
        }

        // Take a screenshot for debugging
        await page.screenshot({
          path: "test/browser/screenshots/youtube-real-detection.png",
        });
      } finally {
        await page.close();
      }
    });
  }

  test("does not detect media on non-video pages", async () => {
    const page = await context.newPage();
    try {
      await page.context().addCookies([
        { name: "CONSENT", value: "YES+cb", domain: ".youtube.com", path: "/" },
      ]);

      // YouTube homepage (not a video page)
      await page.goto("https://www.youtube.com/", {
        waitUntil: "domcontentloaded",
        timeout: 30000,
      });
      await dismissYouTubeConsent(page);
      await page.waitForTimeout(5000);

      const tabId = await sw.evaluate(async () => {
        // @ts-ignore
        const tabs = await chrome.tabs.query({ url: "https://www.youtube.com/*" });
        // Find the tab that's on the homepage, not a /watch page
        const homeTabs = tabs.filter((t) => !t.url.includes("/watch"));
        return homeTabs[0]?.id;
      });

      if (tabId) {
        const media = await getDetectedMedia(sw, tabId);
        const siteItems = media.filter((m) => m.source === "site");
        // Homepage should not detect any site-extracted videos
        console.log(`Homepage site-extracted items: ${siteItems.length}`);
        expect(siteItems.length).toBe(0);
      }
    } finally {
      await page.close();
    }
  });

  test("detects video after SPA navigation", async () => {
    const page = await context.newPage();
    try {
      await page.context().addCookies([
        { name: "CONSENT", value: "YES+cb", domain: ".youtube.com", path: "/" },
      ]);

      // Start on a video page
      await page.goto("https://www.youtube.com/watch?v=aqz-KE-bpKQ", {
        waitUntil: "domcontentloaded",
        timeout: 30000,
      });
      await dismissYouTubeConsent(page);
      await page.waitForSelector("video", { timeout: 15000 });
      await page.waitForTimeout(5000);

      // Now navigate to a different video via SPA (click a suggestion or use JS)
      // Use YouTube's navigation by modifying the URL client-side
      const navigated = await page.evaluate(() => {
        // Find a suggested/related video link
        const links = Array.from(document.querySelectorAll('a[href*="/watch?v="]'));
        const otherVideo = links.find(
          (a) => !a.getAttribute("href")?.includes("aqz-KE-bpKQ")
        );
        if (otherVideo) {
          // @ts-ignore
          otherVideo.click();
          return true;
        }
        return false;
      });

      if (!navigated) {
        console.log("⚠ No suggested videos found, skipping SPA navigation test");
        test.skip();
        return;
      }

      // Wait for YouTube SPA navigation + content script extraction (3s delay + buffer)
      await page.waitForTimeout(8000);

      const currentUrl = page.url();
      console.log(`SPA navigated to: ${currentUrl}`);
      expect(currentUrl).not.toContain("aqz-KE-bpKQ");

      // Get tab media
      const tabId = await sw.evaluate(async () => {
        // @ts-ignore
        const tabs = await chrome.tabs.query({
          url: "https://www.youtube.com/watch*",
          active: true,
        });
        return tabs[0]?.id;
      });

      if (tabId) {
        const media = await getDetectedMedia(sw, tabId);
        const siteItems = media.filter((m) => m.source === "site");
        console.log(`After SPA nav, site-extracted items: ${siteItems.length}`);
        // Should have detected the new video
        // Note: might have both old and new if tab wasn't cleared
        expect(siteItems.length).toBeGreaterThanOrEqual(1);
      }
    } finally {
      await page.close();
    }
  });
});
