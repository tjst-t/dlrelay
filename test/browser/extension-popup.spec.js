// @ts-check
const { test, expect } = require("@playwright/test");
const path = require("path");

// Test the popup UI by loading it directly as a file
// (Since we can't load a Chrome extension in standard Playwright,
// we mock the chrome APIs and test the UI behavior)

const POPUP_PATH = path.resolve(__dirname, "../../extension/popup/popup.html");

test.describe("Extension Popup UI", () => {
  test.beforeEach(async ({ page }) => {
    // Mock chrome APIs before loading the page
    await page.addInitScript(() => {
      const storage = {
        enabled: true,
        serverUrl: "http://test-server:8090",
        apiKey: "",
        mode: "video",
        minFileSize: 102400,
        autoDownload: false,
        recentActivity: [],
      };

      window.chrome = {
        storage: {
          local: {
            get: async (defaults) => {
              const result = {};
              for (const key of Object.keys(defaults)) {
                result[key] = storage[key] !== undefined ? storage[key] : defaults[key];
              }
              return result;
            },
            set: async (data) => {
              Object.assign(storage, data);
            },
          },
          session: {
            get: async () => ({}),
            set: async () => {},
            remove: async () => {},
          },
        },
        runtime: {
          sendMessage: (msg, cb) => {
            if (msg.type === "dlrelay-popup-get-media") {
              cb({
                items: [
                  {
                    id: "test1",
                    url: "https://example.com/video.mp4",
                    pageUrl: "https://example.com/watch",
                    type: "http",
                    mimeType: "video/mp4",
                    filename: "My Video.mp4",
                    title: "My Great Video",
                    thumbnail: "",
                    duration: 7773, // 2:09:33
                    size: 52428800,
                    resolution: "1080p",
                    source: "webRequest",
                    headers: {},
                    variants: [],
                    timestamp: Date.now(),
                  },
                  {
                    id: "test2",
                    url: "https://example.com/stream.m3u8",
                    pageUrl: "https://example.com/live",
                    type: "hls",
                    mimeType: "application/vnd.apple.mpegurl",
                    filename: "stream.m3u8",
                    title: "Live Stream",
                    thumbnail: "",
                    duration: 0,
                    size: 0,
                    resolution: "",
                    source: "webRequest",
                    headers: {},
                    variants: [
                      { url: "https://example.com/720p.m3u8", label: "720p", resolution: "1280x720", bandwidth: 2560000 },
                      { url: "https://example.com/1080p.m3u8", label: "1080p", resolution: "1920x1080", bandwidth: 7680000 },
                    ],
                    timestamp: Date.now() - 1000,
                  },
                  {
                    id: "test3",
                    url: "https://example.com/manifest.mpd",
                    pageUrl: "https://example.com/video",
                    type: "dash",
                    mimeType: "application/dash+xml",
                    filename: "manifest.mpd",
                    title: "DASH Video",
                    thumbnail: "",
                    duration: 300,
                    size: 0,
                    resolution: "",
                    source: "webRequest",
                    headers: {},
                    variants: [
                      { url: "https://example.com/video_1080p.mp4", audioUrl: "https://example.com/audio.m4a", label: "1080p + 128kbps audio", resolution: "1920x1080", bandwidth: 5000000 },
                      { url: "https://example.com/video_720p.mp4", audioUrl: "https://example.com/audio.m4a", label: "720p + 128kbps audio", resolution: "1280x720", bandwidth: 2500000 },
                    ],
                    timestamp: Date.now() - 2000,
                  },
                ],
                tabUrl: "https://example.com/watch",
              });
            } else if (msg.type === "dlrelay-popup-download") {
              cb({ id: "dl-123", state: "queued" });
            } else {
              cb({});
            }
          },
        },
        action: {
          setBadgeText: () => {},
          setBadgeBackgroundColor: () => {},
        },
        tabs: {
          create: () => {},
          query: (q, cb) => cb([{ id: 1, url: "https://example.com/watch" }]),
        },
      };
    });
  });

  test("shows detected video list with cards", async ({ page }) => {
    await page.goto("file://" + POPUP_PATH);
    await page.waitForSelector(".media-card");

    const items = page.locator(".media-card");
    await expect(items).toHaveCount(3);

    await page.screenshot({
      path: "test/browser/screenshots/popup-media-list.png",
    });
  });

  test("shows video title in cards", async ({ page }) => {
    await page.goto("file://" + POPUP_PATH);
    await page.waitForSelector(".media-card");

    const firstTitle = page.locator(".media-title").first();
    await expect(firstTitle).toContainText("My Great Video");
  });

  test("shows format badges", async ({ page }) => {
    await page.goto("file://" + POPUP_PATH);
    await page.waitForSelector(".media-card");

    // First card should have MP4 and 1080p badges
    const firstCard = page.locator(".media-card").first();
    await expect(firstCard.locator(".media-badge")).toContainText(["MP4"]);

    // Should show file size
    await expect(firstCard.locator(".media-badge").filter({ hasText: "50.0 MB" })).toBeVisible();
  });

  test("shows duration overlay", async ({ page }) => {
    await page.goto("file://" + POPUP_PATH);
    await page.waitForSelector(".media-card");

    // First card has duration 7773s = 2:09:33
    const duration = page.locator(".media-thumb-duration").first();
    await expect(duration).toContainText("2:09:33");
  });

  test("shows domain in meta info", async ({ page }) => {
    await page.goto("file://" + POPUP_PATH);
    await page.waitForSelector(".media-card");

    const domain = page.locator(".media-domain").first();
    await expect(domain).toContainText("example.com");
  });

  test("shows quality selector for HLS streams", async ({ page }) => {
    await page.goto("file://" + POPUP_PATH);
    await page.waitForSelector(".media-card");

    // HLS card should have a quality selector
    const hlsCard = page.locator(".media-card").nth(1); // second card
    const select = hlsCard.locator(".quality-select");
    await expect(select).toBeVisible();

    const options = select.locator("option");
    await expect(options).toHaveCount(2);
  });

  test("shows quality selector for DASH streams", async ({ page }) => {
    await page.goto("file://" + POPUP_PATH);
    await page.waitForSelector(".media-card");

    const dashCard = page.locator(".media-card").nth(2); // third card
    const select = dashCard.locator(".quality-select");
    await expect(select).toBeVisible();

    const options = select.locator("option");
    await expect(options).toHaveCount(2);
  });

  test("download button sends request", async ({ page }) => {
    await page.goto("file://" + POPUP_PATH);
    await page.waitForSelector(".media-card");

    const firstDlBtn = page.locator(".btn-download").first();
    await firstDlBtn.click();

    await expect(firstDlBtn).toContainText("Sent!");
  });

  test("settings view toggle works", async ({ page }) => {
    await page.goto("file://" + POPUP_PATH);

    await page.click("#settings-btn");

    await expect(page.locator("#view-settings")).toBeVisible();
    await expect(page.locator("#view-main")).not.toBeVisible();

    await expect(page.locator("#server-url")).toHaveValue("http://test-server:8090");

    await page.click("#settings-btn");
    await expect(page.locator("#view-main")).toBeVisible();
    await expect(page.locator("#view-settings")).not.toBeVisible();

    await page.screenshot({
      path: "test/browser/screenshots/popup-settings.png",
    });
  });

  test("settings view shows mode toggle", async ({ page }) => {
    await page.goto("file://" + POPUP_PATH);
    await page.click("#settings-btn");

    const videoBtn = page.locator('.toggle[data-mode="video"]');
    const allBtn = page.locator('.toggle[data-mode="all"]');

    await expect(videoBtn).toHaveClass(/active/);
    await expect(allBtn).not.toHaveClass(/active/);
  });

  test("empty state shown when no media detected", async ({ page }) => {
    await page.addInitScript(() => {
      window.chrome.runtime.sendMessage = (msg, cb) => {
        if (msg.type === "dlrelay-popup-get-media") {
          cb({ items: [], tabUrl: "" });
        } else {
          cb({});
        }
      };
    });

    await page.goto("file://" + POPUP_PATH);

    await expect(page.locator("#empty-state")).toBeVisible();
  });
});
