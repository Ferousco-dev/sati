// SATI service worker.
// It keeps the app installable and makes pictures and scripts load fast on repeat visits.
// Only the explicitly public catalog API responses are cached. Personal API data is never stored.

const VERSION = "v1";
const SHELL_CACHE = `sati-shell-${VERSION}`;
const ASSET_CACHE = `sati-assets-${VERSION}`;
const OFFLINE_URL = "/offline.html";

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(SHELL_CACHE)
      .then((cache) => cache.add(OFFLINE_URL))
      .then(() => {
        if (!self.registration.active) return self.skipWaiting();
      }),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((names) =>
        Promise.all(
          names
            .filter((n) => n.startsWith("sati-") && n !== SHELL_CACHE && n !== ASSET_CACHE)
            .map((n) => caches.delete(n)),
        ),
      )
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("message", (event) => {
  if (event.data === "skip-waiting") self.skipWaiting();
});

function isStaticAsset(url, request) {
  return (
    request.destination === "image" ||
    url.pathname.startsWith("/_next/static/") ||
    url.pathname.startsWith("/images/") ||
    url.pathname.startsWith("/icons/") ||
    /\.(?:avif|gif|jpe?g|png|svg|webp)$/i.test(url.pathname) ||
    /\.(?:woff2?|ttf|otf)$/.test(url.pathname)
  );
}

// Serve the saved copy straight away and refresh it in the background.
async function staleWhileRevalidate(request) {
  const cache = await caches.open(ASSET_CACHE);
  const cached = await cache.match(request);
  const refresh = fetch(request)
    .then((response) => {
      if (response.ok || response.type === "opaque") {
        cache.put(request, response.clone()).catch(() => {});
      }
      return response;
    })
    .catch(() => cached || Response.error());
  return cached || refresh;
}

self.addEventListener("fetch", (event) => {
  const { request } = event;
  if (request.method !== "GET") return;

  const url = new URL(request.url);
  const sameOrigin = url.origin === self.location.origin;
  if (!sameOrigin && request.destination !== "image") return;
  const isPublicCatalog =
    sameOrigin &&
    (url.pathname === "/api/v1/products" || url.pathname === "/api/v1/categories");
  if (sameOrigin && url.pathname.startsWith("/api/") && !isPublicCatalog) return;

  if (request.mode === "navigate") {
    event.respondWith(
      fetch(request)
        .then(async (response) => {
          if (url.pathname === "/menu" && response.ok) {
            const cache = await caches.open(SHELL_CACHE);
            await cache.put("/menu", response.clone()).catch(() => {});
          }
          return response;
        })
        .catch(async () =>
          (await caches.match(request, { ignoreSearch: true })) ||
          (await caches.match(OFFLINE_URL)) ||
          Response.error(),
        ),
    );
    return;
  }

  if (isPublicCatalog || isStaticAsset(url, request)) {
    event.respondWith(staleWhileRevalidate(request));
  }
});
