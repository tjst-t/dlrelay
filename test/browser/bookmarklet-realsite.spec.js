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
  return code;
}

test.describe("Bookmarklet on real sites", () => {
  test("works on a real page with HTML5 video (w3schools sample)", async ({
    page,
  }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    // Go to a real page with a video
    await page.goto("https://www.w3schools.com/html/html5_video.asp", {
      waitUntil: "domcontentloaded",
      timeout: 15000,
    });

    // Wait a moment for any dynamic content
    await page.waitForTimeout(1000);

    // Execute the bookmarklet
    const result = await page.evaluate((code) => {
      try {
        eval(code);
        return { success: true };
      } catch (e) {
        return { success: false, error: e.message, stack: e.stack };
      }
    }, jsCode);

    console.log("Bookmarklet execution result:", JSON.stringify(result));
    expect(result.success).toBe(true);

    // Overlay should appear
    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible({ timeout: 5000 });

    // Should have yt-dlp button
    const ytdlpBtn = overlay.locator("button").filter({ hasText: "yt-dlp:" });
    await expect(ytdlpBtn).toBeVisible();

    // Check what was detected
    const overlayText = await overlay.textContent();
    console.log("Overlay content:", overlayText.substring(0, 500));

    // Should detect some media on the page (w3schools has sample videos)
    // Note: The actual detection depends on what the page loads
    expect(overlayText).toContain("DL Relay");
  });

  test("works on a page with no videos (example.com)", async ({ page }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    await page.goto("https://example.com", {
      waitUntil: "domcontentloaded",
      timeout: 15000,
    });

    const result = await page.evaluate((code) => {
      try {
        eval(code);
        return { success: true };
      } catch (e) {
        return { success: false, error: e.message, stack: e.stack };
      }
    }, jsCode);

    console.log("Bookmarklet execution result:", JSON.stringify(result));
    expect(result.success).toBe(true);

    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible({ timeout: 5000 });

    // Should show yt-dlp button with page title
    await expect(
      overlay.locator("button").filter({ hasText: "yt-dlp:" })
    ).toBeVisible();

    // Should show no-video message since example.com has no videos
    await expect(overlay).toContainText("No <video>/<source>");
  });

  test("yt-dlp button sends correct request on real page", async ({
    page,
  }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    await page.goto("https://example.com", {
      waitUntil: "domcontentloaded",
      timeout: 15000,
    });

    await page.evaluate((code) => {
      eval(code);
    }, jsCode);

    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible({ timeout: 5000 });

    // Intercept the API call and verify it goes to the right server
    const [request] = await Promise.all([
      page.waitForRequest(
        (req) =>
          req.url().includes("/api/downloads") && req.method() === "POST",
        { timeout: 5000 }
      ),
      overlay
        .locator("button")
        .filter({ hasText: "yt-dlp:" })
        .click(),
    ]);

    // Verify request details
    expect(request.url()).toContain(BASE_URL);
    const postData = request.postDataJSON();
    expect(postData.url).toBe("https://example.com/");
    expect(postData.method).toBe("ytdlp");
    expect(postData.filename).toBeTruthy();

    console.log("Request sent:", JSON.stringify(postData));
  });

  test("yt-dlp button shows feedback after click", async ({ page }) => {
    const bmCode = await getBookmarkletCode(page);
    const jsCode = decodeURIComponent(bmCode.replace("javascript:", ""));

    await page.goto("https://example.com", {
      waitUntil: "domcontentloaded",
      timeout: 15000,
    });

    await page.evaluate((code) => {
      eval(code);
    }, jsCode);

    const overlay = page.locator("#dl-bm");
    await expect(overlay).toBeVisible({ timeout: 5000 });

    // Get a reference to the yt-dlp button and click it
    const ytdlpBtn = overlay.locator("button").filter({ hasText: "yt-dlp:" });
    await ytdlpBtn.click();

    // After click, the button text changes. Wait for it to show result.
    // Use a broad locator since button text changes from "yt-dlp:..." to "Sending..." to result.
    // The first non-close-button in the overlay should be the yt-dlp button.
    const resultBtn = overlay
      .locator("button")
      .filter({ hasNotText: "\u2715" })
      .first();
    // Wait for the button to show a result (checkmark or error X)
    await expect(resultBtn).toHaveText(/[\u2713\u2717]/, { timeout: 10000 });

    const btnText = await resultBtn.textContent();
    console.log("Button state after click:", btnText);
  });
});
