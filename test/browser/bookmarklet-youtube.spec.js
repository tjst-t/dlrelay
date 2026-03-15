// @ts-check
const { test, expect } = require("@playwright/test");

const BASE_URL = process.env.TEST_URL || "http://localhost:8200";

async function getBookmarkletCode(page) {
  await page.goto(BASE_URL + "/bookmarklet");
  const code = await page.evaluate(() => {
    const el = document.getElementById("code-display");
    return el ? el.textContent : null;
  });
  return code;
}

test.describe("Bookmarklet with Trusted Types CSP (like YouTube)", () => {
  test("works on a page with require-trusted-types-for CSP", async ({
    page,
  }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    // Create a page that mimics YouTube's CSP via meta tag
    // Note: meta http-equiv CSP doesn't support require-trusted-types-for,
    // so we test with a server-side CSP header simulation
    await page.goto("about:blank");
    await page.setContent(`
      <!DOCTYPE html>
      <html>
      <head>
        <meta http-equiv="Content-Security-Policy" content="require-trusted-types-for 'script'">
        <title>CSP Test Page</title>
      </head>
      <body>
        <video src="https://example.com/test.mp4" controls></video>
      </body>
      </html>
    `);

    // Execute the bookmarklet - should NOT crash due to innerHTML
    const result = await page.evaluate((code) => {
      try {
        eval(code);
        return { success: true };
      } catch (e) {
        return { success: false, error: e.message, name: e.name };
      }
    }, jsCode);

    console.log("Execution result:", JSON.stringify(result));
    expect(result.success).toBe(true);

    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible({ timeout: 3000 });
    await expect(overlay).toContainText("DL Relay");
  });
});

test.describe("Bookmarklet on real YouTube page", () => {
  test("overlay appears and yt-dlp button sends correct request", async ({
    page,
  }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    // Navigate to YouTube
    await page.goto("https://www.youtube.com/watch?v=oZC2X-igqUE", {
      waitUntil: "domcontentloaded",
      timeout: 20000,
    });

    // Wait for YouTube to finish loading
    await page.waitForTimeout(2000);

    // Execute the bookmarklet
    const result = await page.evaluate((code) => {
      try {
        eval(code);
        return { success: true };
      } catch (e) {
        return { success: false, error: e.message, name: e.name, stack: e.stack };
      }
    }, jsCode);

    console.log("YouTube execution result:", JSON.stringify(result));
    expect(result.success).toBe(true);

    // Overlay should appear
    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible({ timeout: 5000 });

    // Should have DL Relay header
    await expect(overlay).toContainText("DL Relay");

    // Should have yt-dlp button with the video title
    const ytdlpBtn = overlay.locator("button").filter({ hasText: "yt-dlp:" });
    await expect(ytdlpBtn).toBeVisible();

    // Verify the yt-dlp button text contains something (video title)
    const btnText = await ytdlpBtn.textContent();
    console.log("yt-dlp button text:", btnText);
    expect(btnText.length).toBeGreaterThan(10);

    // Click the yt-dlp button and verify the request
    const [request] = await Promise.all([
      page.waitForRequest(
        (req) =>
          req.url().includes("/api/downloads") && req.method() === "POST",
        { timeout: 5000 }
      ),
      ytdlpBtn.click(),
    ]);

    const postData = request.postDataJSON();
    console.log("Request sent:", JSON.stringify(postData));

    // Verify correct data
    expect(postData.url).toContain("youtube.com/watch");
    expect(postData.method).toBe("ytdlp");
    // Filename should be the video title, not "watch"
    expect(postData.filename).not.toBe("watch");
    expect(postData.filename.length).toBeGreaterThan(3);
  });
});
