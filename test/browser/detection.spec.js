// @ts-check
const { test, expect } = require("@playwright/test");

const BASE_URL = process.env.TEST_URL || "http://localhost:8200";

test.describe("Video Detection Integration", () => {
  test("server accepts DASH download with audio_url", async ({ request }) => {
    const resp = await request.post(BASE_URL + "/api/downloads", {
      data: {
        url: "http://example.com/video_1080p.mp4",
        audio_url: "http://example.com/audio_128k.m4a",
        filename: "dash_test.mp4",
        headers: {
          Referer: "https://example.com/watch",
          Cookie: "session=test123",
        },
      },
    });

    expect(resp.status()).toBe(202);
    const body = await resp.json();
    expect(body.id).toBeTruthy();
    expect(body.state).toBe("queued");
  });

  test("server accepts download with custom headers", async ({ request }) => {
    const resp = await request.post(BASE_URL + "/api/downloads", {
      data: {
        url: "http://example.com/protected-video.mp4",
        filename: "protected.mp4",
        headers: {
          Referer: "https://example.com/members",
          Cookie: "auth=token123; session=abc",
          Authorization: "Bearer eyJhbGciOiJIUzI1NiJ9.test",
        },
      },
    });

    expect(resp.status()).toBe(202);
    const body = await resp.json();
    expect(body.id).toBeTruthy();
  });

  test("server lists downloads after creation", async ({ request }) => {
    // Create a download first
    await request.post(BASE_URL + "/api/downloads", {
      data: {
        url: "http://example.com/list-test.mp4",
        filename: "list-test.mp4",
      },
    });

    const resp = await request.get(BASE_URL + "/api/downloads");
    expect(resp.status()).toBe(200);

    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);

    // Find our download
    const dl = body.find((d) => d.filename === "list-test.mp4");
    expect(dl).toBeTruthy();
    expect(dl.url).toContain("list-test.mp4");
  });

  test("server rejects download without url", async ({ request }) => {
    const resp = await request.post(BASE_URL + "/api/downloads", {
      data: {
        filename: "no-url.mp4",
      },
    });

    expect(resp.status()).toBe(400);
    const body = await resp.json();
    expect(body.error).toContain("url");
  });

  test("server can cancel a download", async ({ request }) => {
    const createResp = await request.post(BASE_URL + "/api/downloads", {
      data: {
        url: "http://example.com/cancel-test.mp4",
        filename: "cancel-test.mp4",
      },
    });

    const { id } = await createResp.json();
    expect(id).toBeTruthy();

    const deleteResp = await request.delete(BASE_URL + "/api/downloads/" + id);
    expect(deleteResp.status()).toBe(204);

    // Should be gone
    const getResp = await request.get(BASE_URL + "/api/downloads/" + id);
    expect(getResp.status()).toBe(404);
  });
});

test.describe("Status Dashboard with DASH downloads", () => {
  test("shows DASH download in dashboard", async ({ page, request }) => {
    // Create a DASH download
    await request.post(BASE_URL + "/api/downloads", {
      data: {
        url: "http://example.com/dash-video.mp4",
        audio_url: "http://example.com/dash-audio.m4a",
        filename: "dash-combined.mp4",
      },
    });

    await page.goto(BASE_URL);
    await page.waitForSelector("table", { timeout: 5000 });

    await expect(page.locator("table")).toBeVisible();
    await expect(
      page.locator(".filename", { hasText: "dash-combined.mp4" }).first()
    ).toBeVisible();

    await page.screenshot({
      path: "test/browser/screenshots/status-dash-download.png",
      fullPage: true,
    });
  });
});
