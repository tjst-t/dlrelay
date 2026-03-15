// @ts-check
const { test, expect } = require("@playwright/test");

const BASE_URL = process.env.TEST_URL || "http://localhost:8200";

test.describe("Download Status Dashboard", () => {
  test("root page loads and has correct title", async ({ page }) => {
    await page.goto(BASE_URL);
    await expect(page).toHaveTitle("DL Relay — Downloads");
    await expect(page.locator(".stats")).toBeVisible();
    await page.screenshot({
      path: "test/browser/screenshots/status-empty.png",
      fullPage: true,
    });
  });

  test("shows downloads after creating one", async ({ page, request }) => {
    // Create a download (will fail since URL is fake, but shows in list)
    await request.post(BASE_URL + "/api/downloads", {
      data: {
        url: "http://example.com/test-video.mp4",
        filename: "test-video.mp4",
      },
    });

    await page.goto(BASE_URL);

    // Wait for auto-refresh to populate
    await page.waitForSelector("table", { timeout: 5000 });

    // Verify table has content
    await expect(page.locator("table")).toBeVisible();
    await expect(
      page.locator(".filename", { hasText: "test-video.mp4" }).first()
    ).toBeVisible();

    // Verify stats show non-zero total
    const total = page.locator("#stat-total");
    await expect(total).not.toHaveText("-");

    await page.screenshot({
      path: "test/browser/screenshots/status-with-downloads.png",
      fullPage: true,
    });
  });

  test("header has Extension and Bookmarklet links", async ({ page }) => {
    await page.goto(BASE_URL);
    await expect(page.locator('a[href*="/setup"]')).toBeVisible();
    await expect(page.locator('a[href*="/bookmarklet"]')).toBeVisible();
  });

  test("page structure is correct", async ({ page }) => {
    await page.goto(BASE_URL);
    await expect(page.locator(".stats")).toBeVisible();
    // Verify all 4 stat cards exist
    await expect(page.locator("#stat-total")).toBeVisible();
    await expect(page.locator("#stat-active")).toBeVisible();
    await expect(page.locator("#stat-completed")).toBeVisible();
    await expect(page.locator("#stat-failed")).toBeVisible();
  });
});
