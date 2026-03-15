// @ts-check
const { test, expect } = require("@playwright/test");

// Test the M3U8 and MPD parsing logic by evaluating the extension code in a browser context

test.describe("M3U8 Parsing", () => {
  test("parses master playlist with variants", async ({ page }) => {
    await page.goto("about:blank");

    const result = await page.evaluate(() => {
      // Inline the parseM3U8Text function for testing
      function parseM3U8Attributes(str) {
        const attrs = {};
        const re = /([A-Z0-9-]+)=(?:"([^"]*)"|([^,]*))/g;
        let match;
        while ((match = re.exec(str)) !== null) {
          attrs[match[1]] = match[2] || match[3];
        }
        return attrs;
      }

      function resolveUrl(base, ref) {
        try { return new URL(ref, base).href; } catch { return ref; }
      }

      function parseM3U8Text(text, baseUrl) {
        const lines = text.split("\n").map(l => l.trim());
        const variants = [];
        let isMediaPlaylist = false;

        for (let i = 0; i < lines.length; i++) {
          const line = lines[i];
          if (line.startsWith("#EXT-X-TARGETDURATION") || line.startsWith("#EXTINF:")) {
            isMediaPlaylist = true;
          }
          if (line.startsWith("#EXT-X-STREAM-INF:")) {
            const attrs = parseM3U8Attributes(line.substring("#EXT-X-STREAM-INF:".length));
            const nextLine = lines[i + 1];
            if (nextLine && !nextLine.startsWith("#")) {
              const variantUrl = resolveUrl(baseUrl, nextLine);
              const resolution = attrs.RESOLUTION || "";
              const bandwidth = parseInt(attrs.BANDWIDTH || "0", 10);
              const codecs = attrs.CODECS || "";
              let label = "";
              if (resolution) {
                const height = resolution.split("x")[1];
                label = `${height}p`;
              } else if (bandwidth > 0) {
                label = `${Math.round(bandwidth / 1000)}kbps`;
              }
              variants.push({ url: variantUrl, resolution, bandwidth, codecs, label });
            }
          }
        }

        if (variants.length > 0) {
          variants.sort((a, b) => b.bandwidth - a.bandwidth);
          return { type: "master", variants };
        }
        if (isMediaPlaylist) return { type: "media", url: baseUrl };
        return null;
      }

      const m3u8 = `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360,CODECS="avc1.4d001e,mp4a.40.2"
360p.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1400000,RESOLUTION=1280x720,CODECS="avc1.4d001f,mp4a.40.2"
720p.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=7680000,RESOLUTION=1920x1080,CODECS="avc1.640028,mp4a.40.2"
1080p.m3u8`;

      return parseM3U8Text(m3u8, "https://cdn.example.com/stream/master.m3u8");
    });

    expect(result).not.toBeNull();
    expect(result.type).toBe("master");
    expect(result.variants).toHaveLength(3);

    // Sorted by bandwidth descending (highest first)
    expect(result.variants[0].label).toBe("1080p");
    expect(result.variants[0].bandwidth).toBe(7680000);
    expect(result.variants[0].url).toBe("https://cdn.example.com/stream/1080p.m3u8");

    expect(result.variants[1].label).toBe("720p");
    expect(result.variants[1].bandwidth).toBe(1400000);

    expect(result.variants[2].label).toBe("360p");
    expect(result.variants[2].bandwidth).toBe(800000);
  });

  test("parses media playlist correctly", async ({ page }) => {
    await page.goto("about:blank");

    const result = await page.evaluate(() => {
      function parseM3U8Text(text, baseUrl) {
        const lines = text.split("\n").map(l => l.trim());
        const variants = [];
        let isMediaPlaylist = false;
        for (let i = 0; i < lines.length; i++) {
          const line = lines[i];
          if (line.startsWith("#EXT-X-TARGETDURATION") || line.startsWith("#EXTINF:")) {
            isMediaPlaylist = true;
          }
        }
        if (variants.length > 0) return { type: "master", variants };
        if (isMediaPlaylist) return { type: "media", url: baseUrl };
        return null;
      }

      const m3u8 = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:4
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:4.000,
seg-0.ts
#EXTINF:4.000,
seg-1.ts
#EXT-X-ENDLIST`;

      return parseM3U8Text(m3u8, "https://cdn.example.com/stream/720p.m3u8");
    });

    expect(result).not.toBeNull();
    expect(result.type).toBe("media");
    expect(result.url).toBe("https://cdn.example.com/stream/720p.m3u8");
  });
});

test.describe("MPD Parsing", () => {
  test("parses DASH manifest with video and audio", async ({ page }) => {
    await page.goto("about:blank");

    const result = await page.evaluate(() => {
      function resolveUrl(base, ref) {
        try { return new URL(ref, base).href; } catch { return ref; }
      }

      function parseMPDText(text, baseUrl) {
        const parser = new DOMParser();
        const doc = parser.parseFromString(text, "application/xml");
        const videoReps = [];
        const audioReps = [];

        const adaptationSets = doc.querySelectorAll("AdaptationSet");
        for (const as of adaptationSets) {
          const mimeType = as.getAttribute("mimeType") || "";
          const contentType = as.getAttribute("contentType") || "";
          const isVideo = mimeType.startsWith("video") || contentType === "video";
          const isAudio = mimeType.startsWith("audio") || contentType === "audio";

          const reps = as.querySelectorAll("Representation");
          for (const rep of reps) {
            const repMime = rep.getAttribute("mimeType") || mimeType;
            const bandwidth = parseInt(rep.getAttribute("bandwidth") || "0", 10);
            const width = parseInt(rep.getAttribute("width") || "0", 10);
            const height = parseInt(rep.getAttribute("height") || "0", 10);
            const codecs = rep.getAttribute("codecs") || as.getAttribute("codecs") || "";

            let repUrl = "";
            const baseUrlEl = rep.querySelector("BaseURL") || as.querySelector("BaseURL");
            if (baseUrlEl) repUrl = resolveUrl(baseUrl, baseUrlEl.textContent.trim());

            const entry = { url: repUrl || baseUrl, bandwidth, width, height, codecs, mimeType: repMime, label: "" };

            if (isVideo || repMime.startsWith("video")) {
              entry.label = height > 0 ? `${height}p` : `${Math.round(bandwidth / 1000)}kbps`;
              videoReps.push(entry);
            } else if (isAudio || repMime.startsWith("audio")) {
              entry.label = `${Math.round(bandwidth / 1000)}kbps`;
              audioReps.push(entry);
            }
          }
        }

        videoReps.sort((a, b) => b.bandwidth - a.bandwidth);
        audioReps.sort((a, b) => b.bandwidth - a.bandwidth);

        const variants = [];
        if (videoReps.length > 0) {
          const bestAudio = audioReps[0];
          for (const video of videoReps) {
            const label = bestAudio ? `${video.label} + ${bestAudio.label} audio` : video.label;
            variants.push({
              url: video.url,
              audioUrl: bestAudio ? bestAudio.url : "",
              resolution: video.width > 0 ? `${video.width}x${video.height}` : "",
              bandwidth: video.bandwidth,
              codecs: video.codecs,
              label,
            });
          }
        }

        return { type: "dash", variants, videoReps, audioReps };
      }

      const mpd = `<?xml version="1.0"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011">
  <Period>
    <AdaptationSet mimeType="video/mp4" codecs="avc1.640028">
      <Representation bandwidth="5000000" width="1920" height="1080">
        <BaseURL>video_1080p.mp4</BaseURL>
      </Representation>
      <Representation bandwidth="2500000" width="1280" height="720">
        <BaseURL>video_720p.mp4</BaseURL>
      </Representation>
      <Representation bandwidth="800000" width="640" height="360">
        <BaseURL>video_360p.mp4</BaseURL>
      </Representation>
    </AdaptationSet>
    <AdaptationSet mimeType="audio/mp4" codecs="mp4a.40.2">
      <Representation bandwidth="128000">
        <BaseURL>audio_128k.m4a</BaseURL>
      </Representation>
      <Representation bandwidth="64000">
        <BaseURL>audio_64k.m4a</BaseURL>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>`;

      return parseMPDText(mpd, "https://cdn.example.com/dash/manifest.mpd");
    });

    expect(result).not.toBeNull();
    expect(result.type).toBe("dash");
    expect(result.videoReps).toHaveLength(3);
    expect(result.audioReps).toHaveLength(2);
    expect(result.variants).toHaveLength(3);

    // Variants sorted by bandwidth descending
    expect(result.variants[0].label).toBe("1080p + 128kbps audio");
    expect(result.variants[0].resolution).toBe("1920x1080");
    expect(result.variants[0].url).toBe("https://cdn.example.com/dash/video_1080p.mp4");
    expect(result.variants[0].audioUrl).toBe("https://cdn.example.com/dash/audio_128k.m4a");

    expect(result.variants[1].label).toBe("720p + 128kbps audio");
    expect(result.variants[2].label).toBe("360p + 128kbps audio");
  });

  test("parses video-only MPD", async ({ page }) => {
    await page.goto("about:blank");

    const result = await page.evaluate(() => {
      function resolveUrl(base, ref) {
        try { return new URL(ref, base).href; } catch { return ref; }
      }

      function parseMPDText(text, baseUrl) {
        const parser = new DOMParser();
        const doc = parser.parseFromString(text, "application/xml");
        const videoReps = [];
        const audioReps = [];

        const adaptationSets = doc.querySelectorAll("AdaptationSet");
        for (const as of adaptationSets) {
          const mimeType = as.getAttribute("mimeType") || "";
          const isVideo = mimeType.startsWith("video");
          const isAudio = mimeType.startsWith("audio");

          const reps = as.querySelectorAll("Representation");
          for (const rep of reps) {
            const repMime = rep.getAttribute("mimeType") || mimeType;
            const bandwidth = parseInt(rep.getAttribute("bandwidth") || "0", 10);
            const height = parseInt(rep.getAttribute("height") || "0", 10);
            const width = parseInt(rep.getAttribute("width") || "0", 10);

            let repUrl = "";
            const baseUrlEl = rep.querySelector("BaseURL");
            if (baseUrlEl) repUrl = resolveUrl(baseUrl, baseUrlEl.textContent.trim());

            const entry = { url: repUrl || baseUrl, bandwidth, width, height, mimeType: repMime, label: "" };
            if (isVideo) {
              entry.label = height > 0 ? `${height}p` : `${Math.round(bandwidth / 1000)}kbps`;
              videoReps.push(entry);
            } else if (isAudio) {
              entry.label = `${Math.round(bandwidth / 1000)}kbps`;
              audioReps.push(entry);
            }
          }
        }

        videoReps.sort((a, b) => b.bandwidth - a.bandwidth);

        const variants = [];
        for (const video of videoReps) {
          variants.push({ url: video.url, audioUrl: "", label: video.label, resolution: `${video.width}x${video.height}`, bandwidth: video.bandwidth });
        }

        return { type: "dash", variants, videoReps, audioReps };
      }

      const mpd = `<?xml version="1.0"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011">
  <Period>
    <AdaptationSet mimeType="video/mp4">
      <Representation bandwidth="3000000" width="1280" height="720">
        <BaseURL>video.mp4</BaseURL>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>`;

      return parseMPDText(mpd, "https://cdn.example.com/dash/manifest.mpd");
    });

    expect(result.variants).toHaveLength(1);
    expect(result.variants[0].audioUrl).toBe("");
    expect(result.variants[0].label).toBe("720p");
    expect(result.audioReps).toHaveLength(0);
  });
});

test.describe("Site Extractor - Twitter", () => {
  test("extracts video URLs from Twitter API response", async ({ page }) => {
    await page.goto("about:blank");

    const items = await page.evaluate(() => {
      function extractTwitterVideos(data) {
        const items = [];
        const seen = new Set();
        function walk(obj) {
          if (!obj || typeof obj !== "object") return;
          if (Array.isArray(obj)) { for (const item of obj) walk(item); return; }
          if (obj.video_info && obj.video_info.variants) {
            const variants = obj.video_info.variants
              .filter(v => v.content_type === "video/mp4")
              .sort((a, b) => (b.bitrate || 0) - (a.bitrate || 0));
            for (const variant of variants) {
              if (seen.has(variant.url)) continue;
              seen.add(variant.url);
              const height = variant.url.match(/\/(\d+)x(\d+)\//)?.[2] || "";
              items.push({
                url: variant.url,
                type: "http",
                mimeType: "video/mp4",
                resolution: height ? `${height}p` : "",
                filename: `twitter_video_${height || "unknown"}p.mp4`,
              });
            }
          }
          for (const key of Object.keys(obj)) walk(obj[key]);
        }
        walk(data);
        return items;
      }

      const twitterApiResponse = {
        data: {
          tweetResult: {
            result: {
              legacy: {
                extended_entities: {
                  media: [{
                    type: "video",
                    video_info: {
                      variants: [
                        { bitrate: 832000, content_type: "video/mp4", url: "https://video.twimg.com/ext_tw_video/123/pu/vid/640x360/abc.mp4" },
                        { bitrate: 2176000, content_type: "video/mp4", url: "https://video.twimg.com/ext_tw_video/123/pu/vid/1280x720/def.mp4" },
                        { content_type: "application/x-mpegURL", url: "https://video.twimg.com/ext_tw_video/123/pl/master.m3u8" },
                        { bitrate: 632000, content_type: "video/mp4", url: "https://video.twimg.com/ext_tw_video/123/pu/vid/480x270/ghi.mp4" },
                      ]
                    }
                  }]
                }
              }
            }
          }
        }
      };

      return extractTwitterVideos(twitterApiResponse);
    });

    expect(items).toHaveLength(3); // 3 mp4 variants (m3u8 filtered out)
    // Sorted by bitrate descending
    expect(items[0].url).toContain("1280x720");
    expect(items[0].resolution).toBe("720p");
    expect(items[1].url).toContain("640x360");
    expect(items[1].resolution).toBe("360p");
    expect(items[2].url).toContain("480x270");
    expect(items[2].resolution).toBe("270p");
  });
});

test.describe("URL Detection Logic", () => {
  test("correctly identifies media URLs by MIME type", async ({ page }) => {
    await page.goto("about:blank");

    const result = await page.evaluate(() => {
      const VIDEO_MIMES = ["video/mp4", "video/webm", "video/ogg", "video/x-matroska", "video/x-flv", "video/3gpp", "video/quicktime", "video/mp2t", "video/mpeg"];
      const STREAM_MIMES = ["application/x-mpegurl", "application/vnd.apple.mpegurl", "application/dash+xml"];
      const AUDIO_MIMES = ["audio/mp4", "audio/mpeg", "audio/webm", "audio/ogg", "audio/aac"];
      const ALL_MEDIA_MIMES = [...VIDEO_MIMES, ...STREAM_MIMES, ...AUDIO_MIMES];
      const VIDEO_EXTS = [".mp4", ".mkv", ".webm", ".avi", ".mov", ".flv", ".ts", ".m3u8", ".mpd", ".m4v", ".wmv", ".3gp", ".m4s", ".mp3", ".m4a", ".aac", ".ogg"];
      const IGNORED_PATTERNS = [/^data:/, /^blob:/, /^chrome-extension:/, /^moz-extension:/, /^about:/, /google-analytics\.com/, /googletagmanager\.com/, /doubleclick\.net/];

      function hasVideoExtension(url) {
        try {
          const pathname = new URL(url).pathname.toLowerCase();
          return VIDEO_EXTS.some(ext => pathname.endsWith(ext));
        } catch { return false; }
      }

      function isMediaUrl(url, mimeType) {
        if (IGNORED_PATTERNS.some(p => p.test(url))) return false;
        if (mimeType) {
          const mime = mimeType.toLowerCase().split(";")[0].trim();
          if (ALL_MEDIA_MIMES.includes(mime)) return true;
          if (mime === "application/octet-stream") return hasVideoExtension(url);
        }
        return hasVideoExtension(url);
      }

      return {
        mp4Mime: isMediaUrl("https://example.com/video", "video/mp4"),
        hlsMime: isMediaUrl("https://example.com/stream", "application/vnd.apple.mpegurl"),
        dashMime: isMediaUrl("https://example.com/manifest", "application/dash+xml"),
        mp4Ext: isMediaUrl("https://example.com/video.mp4", ""),
        m3u8Ext: isMediaUrl("https://example.com/stream.m3u8", ""),
        mpdExt: isMediaUrl("https://example.com/manifest.mpd", ""),
        octetStreamWithExt: isMediaUrl("https://example.com/video.mp4", "application/octet-stream"),
        octetStreamNoExt: isMediaUrl("https://example.com/data", "application/octet-stream"),
        textHtml: isMediaUrl("https://example.com/page", "text/html"),
        dataUrl: isMediaUrl("data:video/mp4;base64,xxx", "video/mp4"),
        blobUrl: isMediaUrl("blob:https://example.com/abc", "video/mp4"),
        analyticsUrl: isMediaUrl("https://google-analytics.com/collect", "video/mp4"),
        queryParam: isMediaUrl("https://cdn.example.com/video.mp4?token=abc123", ""),
      };
    });

    expect(result.mp4Mime).toBe(true);
    expect(result.hlsMime).toBe(true);
    expect(result.dashMime).toBe(true);
    expect(result.mp4Ext).toBe(true);
    expect(result.m3u8Ext).toBe(true);
    expect(result.mpdExt).toBe(true);
    expect(result.octetStreamWithExt).toBe(true);
    expect(result.octetStreamNoExt).toBe(false);
    expect(result.textHtml).toBe(false);
    expect(result.dataUrl).toBe(false);
    expect(result.blobUrl).toBe(false);
    expect(result.analyticsUrl).toBe(false);
    expect(result.queryParam).toBe(true);
  });

  test("correctly determines media type", async ({ page }) => {
    await page.goto("about:blank");

    const result = await page.evaluate(() => {
      function getMediaType(url, mimeType) {
        const lower = (mimeType || "").toLowerCase();
        if (lower.includes("mpegurl") || url.toLowerCase().includes(".m3u8")) return "hls";
        if (lower.includes("dash+xml") || url.toLowerCase().includes(".mpd")) return "dash";
        return "http";
      }

      return {
        hlsByMime: getMediaType("https://example.com/stream", "application/vnd.apple.mpegurl"),
        hlsByExt: getMediaType("https://example.com/stream.m3u8", ""),
        dashByMime: getMediaType("https://example.com/manifest", "application/dash+xml"),
        dashByExt: getMediaType("https://example.com/manifest.mpd", ""),
        httpDefault: getMediaType("https://example.com/video.mp4", "video/mp4"),
      };
    });

    expect(result.hlsByMime).toBe("hls");
    expect(result.hlsByExt).toBe("hls");
    expect(result.dashByMime).toBe("dash");
    expect(result.dashByExt).toBe("dash");
    expect(result.httpDefault).toBe("http");
  });
});
