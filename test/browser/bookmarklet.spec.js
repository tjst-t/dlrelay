// @ts-check
const { test, expect } = require("@playwright/test");

const BASE_URL = process.env.TEST_URL || "http://localhost:8200";

/**
 * Extract the decoded bookmarklet code from the /bookmarklet page.
 */
async function getBookmarkletCode(page) {
  await page.goto(BASE_URL + "/bookmarklet");
  const code = await page.evaluate(() => {
    const el = document.getElementById("code-display");
    return el ? el.textContent : null;
  });
  expect(code).toBeTruthy();
  expect(code).toMatch(/^javascript:/);
  return code;
}

test.describe("Bookmarklet Page", () => {
  test("renders with correct design elements", async ({ page }) => {
    await page.goto(BASE_URL + "/bookmarklet");

    await expect(page).toHaveTitle("DL Relay — Bookmarklet");
    // Header logo
    await expect(page.locator(".logo")).toContainText("DL");
    await expect(page.locator(".logo")).toContainText("Bookmarklet");
    // Hero section
    await expect(page.locator(".hero h1")).toBeVisible();
    // Bookmarklet link
    await expect(page.locator("#bookmarklet-link")).toBeVisible();
    // Copy button
    await expect(page.locator("#copy-btn")).toBeVisible();
    // Code block should contain javascript:
    const codeBlock = page.locator("#code-display");
    await expect(codeBlock).toContainText("javascript:");
  });

  test("bookmarklet link has valid href", async ({ page }) => {
    await page.goto(BASE_URL + "/bookmarklet");
    const href = await page.locator("#bookmarklet-link").getAttribute("href");
    expect(href).toMatch(/^javascript:/);
    // Should contain the server URL encoded
    expect(href).toContain(encodeURIComponent(BASE_URL));
  });

  test("copy button copies code to clipboard", async ({ page, context }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.goto(BASE_URL + "/bookmarklet");
    await page.locator("#copy-btn").click();
    await expect(page.locator("#copy-btn")).toContainText("Copied!");
  });
});

test.describe("Bookmarklet on test page with video elements", () => {
  /** Create a local HTML page with video elements for testing */
  async function setupVideoPage(page) {
    // Navigate away from server URL first (self-URL protection)
    await page.goto("about:blank");
    await page.setContent(`
      <!DOCTYPE html>
      <html>
      <head><title>Test Video Page</title></head>
      <body>
        <h1>Test Video Page</h1>
        <video src="https://www.w3schools.com/html/mov_bbb.mp4" controls></video>
        <video>
          <source src="https://www.w3schools.com/html/movie.mp4" type="video/mp4">
        </video>
        <audio src="https://www.w3schools.com/html/horse.mp3" controls></audio>
      </body>
      </html>
    `);
    // Wait for elements to be ready
    await page.waitForSelector("video");
  }

  test("bookmarklet opens overlay on video page", async ({ page }) => {
    // Get the bookmarklet code first
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    // Navigate to a test page with video
    await setupVideoPage(page);

    // Execute the bookmarklet
    await page.evaluate(jsCode);

    // Overlay should appear
    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible();

    // Header should show DL Relay
    await expect(overlay.locator("span").first()).toContainText("DL Relay");

    // yt-dlp button should be present with page title
    const buttons = overlay.locator("button");
    const ytdlpBtn = buttons.filter({ hasText: "yt-dlp:" });
    await expect(ytdlpBtn).toBeVisible();
    await expect(ytdlpBtn).toContainText("Test Video Page");
  });

  test("bookmarklet detects video and source elements", async ({ page }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    await setupVideoPage(page);
    await page.evaluate(jsCode);

    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible();

    // Should show "Detected media" separator
    await expect(overlay).toContainText("Detected media");

    // Should have buttons for detected video URLs
    const content = await overlay.textContent();
    expect(content).toContain("mov_bbb.mp4");
    expect(content).toContain("movie.mp4");
    expect(content).toContain("horse.mp3");
  });

  test("bookmarklet close button removes overlay", async ({ page }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    await setupVideoPage(page);
    await page.evaluate(jsCode);

    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible();

    // Click close button (the X button)
    await overlay.locator("button", { hasText: "\u2715" }).click();
    await expect(overlay).not.toBeVisible();
  });

  test("bookmarklet toggle - second execution removes overlay", async ({
    page,
  }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    await setupVideoPage(page);

    // Execute once - should appear
    await page.evaluate(jsCode);
    await expect(page.locator("#dl-bm")).toBeVisible();

    // Execute again - should disappear (toggle behavior)
    await page.evaluate(jsCode);
    await expect(page.locator("#dl-bm")).not.toBeVisible();
  });

  test("bookmarklet sends yt-dlp download request", async ({ page }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    await setupVideoPage(page);
    await page.evaluate(jsCode);

    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible();

    // Intercept the API call
    const [request] = await Promise.all([
      page.waitForRequest(
        (req) =>
          req.url().includes("/api/downloads") && req.method() === "POST"
      ),
      overlay
        .locator("button")
        .filter({ hasText: "yt-dlp:" })
        .click(),
    ]);

    const postData = request.postDataJSON();
    expect(postData.method).toBe("ytdlp");
    expect(postData.url).toBeTruthy();
    expect(postData.filename).toBeTruthy();
  });

  test("bookmarklet sends direct video download request", async ({ page }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    await setupVideoPage(page);
    await page.evaluate(jsCode);

    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible();

    // Click a detected video button (not the yt-dlp one)
    const [request] = await Promise.all([
      page.waitForRequest(
        (req) =>
          req.url().includes("/api/downloads") && req.method() === "POST"
      ),
      overlay
        .locator("button")
        .filter({ hasText: "mov_bbb.mp4" })
        .click(),
    ]);

    const postData = request.postDataJSON();
    expect(postData.url).toContain("mov_bbb.mp4");
    expect(postData.method).toBeUndefined();
    expect(postData.filename).toContain("mov_bbb.mp4");
  });
});

test.describe("Bookmarklet self-URL protection", () => {
  test("shows alert when executed on DL Relay server page", async ({
    page,
  }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    // Stay on the bookmarklet page (server URL)
    // Set up dialog handler BEFORE evaluate (alert blocks execution)
    let dialogMessage = "";
    page.on("dialog", async (dialog) => {
      dialogMessage = dialog.message();
      await dialog.accept();
    });

    await page.evaluate(jsCode);

    expect(dialogMessage).toContain("DL Relay");

    // Overlay should NOT appear
    await expect(page.locator("#dl-bm")).not.toBeVisible();
  });
});

test.describe("Bookmarklet filename logic", () => {
  test("yt-dlp request uses document.title as filename", async ({ page }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    await page.goto("about:blank");
    await page.setContent(`
      <!DOCTYPE html>
      <html><head><title>My Great Video Title</title></head>
      <body><h1>Video page</h1></body>
      </html>
    `);

    await page.evaluate(jsCode);
    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible();

    const [request] = await Promise.all([
      page.waitForRequest(
        (req) =>
          req.url().includes("/api/downloads") && req.method() === "POST"
      ),
      overlay
        .locator("button")
        .filter({ hasText: "yt-dlp:" })
        .click(),
    ]);

    const postData = request.postDataJSON();
    // yt-dlp method should use document.title, not URL path
    expect(postData.filename).toBe("My Great Video Title");
    expect(postData.method).toBe("ytdlp");
  });

  test("direct video download uses URL filename", async ({ page }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    await page.goto("about:blank");
    await page.setContent(`
      <!DOCTYPE html>
      <html><head><title>Page Title</title></head>
      <body><video src="https://example.com/my-video.mp4"></video></body>
      </html>
    `);

    await page.evaluate(jsCode);
    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible();

    const [request] = await Promise.all([
      page.waitForRequest(
        (req) =>
          req.url().includes("/api/downloads") && req.method() === "POST"
      ),
      overlay
        .locator("button")
        .filter({ hasText: "my-video.mp4" })
        .click(),
    ]);

    const postData = request.postDataJSON();
    // Direct download should use URL filename
    expect(postData.filename).toBe("my-video.mp4");
  });
});

test.describe("Bookmarklet on page without videos", () => {
  test("shows no-video message when page has no media", async ({ page }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    // Navigate to a page with no videos
    await page.goto("about:blank");
    await page.setContent(`
      <!DOCTYPE html>
      <html><head><title>No Videos</title></head>
      <body><h1>Just text</h1><p>No videos here.</p></body>
      </html>
    `);

    await page.evaluate(jsCode);

    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible();

    // Should show yt-dlp button
    await expect(
      overlay.locator("button").filter({ hasText: "yt-dlp:" })
    ).toBeVisible();

    // Should show no-video message
    await expect(overlay).toContainText("No <video>/<source> elements");
  });
});

test.describe("Bookmarklet ignores blob and data URLs", () => {
  test("filters out blob: and data: URLs from video elements", async ({
    page,
  }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    await page.goto("about:blank");
    await page.setContent(`
      <!DOCTYPE html>
      <html><head><title>Blob Test</title></head>
      <body>
        <video src="https://example.com/real-video.mp4"></video>
        <video>
          <source src="data:video/mp4;base64,AAAA" type="video/mp4">
        </video>
      </body>
      </html>
    `);

    await page.evaluate(jsCode);

    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible();

    const content = await overlay.textContent();
    // Should detect the real video
    expect(content).toContain("real-video.mp4");
    // Should NOT show data: URLs
    expect(content).not.toContain("data:video");
  });
});
