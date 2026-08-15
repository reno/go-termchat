import { expect, test } from "@playwright/test";

test("the manifest is served with the right type and content", async ({ request }) => {
  const response = await request.get("/manifest.webmanifest");

  expect(response.status()).toBe(200);
  expect(response.headers()["content-type"]).toContain("application/manifest+json");

  const manifest = (await response.json()) as {
    name: string;
    start_url: string;
    display: string;
    theme_color: string;
    icons: { sizes: string; purpose: string }[];
  };

  expect(manifest.name).toBe("termchat");
  expect(manifest.start_url).toBe("/");
  expect(manifest.display).toBe("standalone");
  expect(manifest.theme_color).toBe("#16161a");

  const sizes = manifest.icons.map((icon) => icon.sizes);
  expect(sizes).toContain("192x192");
  expect(sizes).toContain("512x512");
  expect(manifest.icons.some((icon) => icon.purpose === "maskable")).toBe(true);
});

test("every declared icon is actually served", async ({ request }) => {
  const manifest = (await (await request.get("/manifest.webmanifest")).json()) as {
    icons: { src: string }[];
  };

  for (const icon of manifest.icons) {
    const response = await request.get(icon.src);
    expect(response.status(), `${icon.src} should be served`).toBe(200);
    expect(response.headers()["content-type"]).toContain("image/png");
  }
});

test("the service worker registers and controls the page", async ({ page }) => {
  await page.goto("/");

  const controlled = await page.evaluate(async () => {
    const registration = await navigator.serviceWorker.ready;
    return {
      scope: registration.scope,
      active: registration.active !== null,
    };
  });

  expect(controlled.active).toBe(true);
  expect(controlled.scope).toMatch(/\/$/);
});

test("the service worker precaches the shell but never an API response", async ({ page }) => {
  await page.goto("/");
  await page.evaluate(() => navigator.serviceWorker.ready);

  const cached = await page.evaluate(async () => {
    const names = await caches.keys();
    const urls: string[] = [];
    for (const name of names) {
      const cache = await caches.open(name);
      for (const request of await cache.keys()) {
        urls.push(new URL(request.url).pathname);
      }
    }
    return urls;
  });

  expect(cached.some((url) => url === "/" || url.endsWith("index.html"))).toBe(true);
  expect(cached.some((url) => url.startsWith("/chat.v1"))).toBe(false);
});

test("health and readiness answer", async ({ request }) => {
  expect((await request.get("/healthz")).status()).toBe(200);
  expect((await request.get("/readyz")).status()).toBe(200);
});

test("an unknown route falls back to the app shell", async ({ request }) => {
  const response = await request.get("/some/deep/link");

  expect(response.status()).toBe(200);
  expect(response.headers()["content-type"]).toContain("text/html");
});
