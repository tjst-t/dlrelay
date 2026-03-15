// @ts-check
const { test, expect } = require("@playwright/test");

const BASE_URL = process.env.TEST_URL || "http://localhost:8200";

test.describe("DL Relay Landing Page", () => {
  test("displays page title and status", async ({ page }) => {
    await page.goto(BASE_URL);

    await expect(page).toHaveTitle("DL Relay");
    await expect(page.locator("h1")).toContainText("DL");
    await expect(page.locator(".status")).toContainText("Server Online");
  });

  test("extension download button is prominent and visible", async ({ page }) => {
    await page.goto(BASE_URL);

    const extBtn = page.locator("#ext-download-btn");
    await expect(extBtn).toBeVisible();
    await expect(extBtn).toContainText("Extension");
    await expect(extBtn).toHaveAttribute("href", /\/api\/extension\.zip/);
  });

  test("browser install tabs work (Chrome/Firefox)", async ({ page }) => {
    await page.goto(BASE_URL);

    // Chrome tab active by default
    await expect(page.locator("#browser-chrome")).toBeVisible();
    await expect(page.locator("#browser-firefox")).not.toBeVisible();

    // Switch to Firefox
    await page.locator(".browser-tab", { hasText: "Firefox" }).click();
    await expect(page.locator("#browser-firefox")).toBeVisible();
    await expect(page.locator("#browser-chrome")).not.toBeVisible();
  });

  test("API Endpoints card has working health link", async ({ page }) => {
    await page.goto(BASE_URL);

    const healthLink = page.locator('a[href*="/api/health"]');
    await expect(healthLink).toBeVisible();
  });
});

test.describe("API Endpoints via Browser", () => {
  test("/api/health returns ok", async ({ page }) => {
    const response = await page.goto(BASE_URL + "/api/health");
    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe("ok");
  });

  test("/api/downloads returns empty array", async ({ page }) => {
    const response = await page.goto(BASE_URL + "/api/downloads");
    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });
});

test.describe("Screenshots", () => {
  test("take full page screenshot", async ({ page }) => {
    await page.goto(BASE_URL);
    await page.screenshot({
      path: "test/browser/screenshots/landing-page.png",
      fullPage: true,
    });
  });
});
