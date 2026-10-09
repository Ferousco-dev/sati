// Runs public/sw.js against a fake browser, because a service worker cannot be started from Node directly.
// Usage: node --test scripts/sw.test.mjs
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const source = readFileSync(new URL("../public/sw.js", import.meta.url), "utf8");

function setup({ network = () => Promise.resolve(new FakeResponse("net")), existing = {}, activeWorker = false } = {}) {
  const origin = "https://shop.example";
  const stores = new Map(Object.entries(existing).map(([name, entries]) => [name, new Map(Object.entries(entries))]));
  const listeners = {};
  const calls = { fetch: [], skipWaiting: 0, claim: 0 };

  const match = (store, req, options = {}) => {
    const requestUrl = new URL(typeof req === "string" ? req : req.url, origin);
    for (const [key, response] of store) {
      const cachedUrl = new URL(key, origin);
      if (cachedUrl.href === requestUrl.href) return response;
      if (options.ignoreSearch && cachedUrl.origin === requestUrl.origin && cachedUrl.pathname === requestUrl.pathname) {
        return response;
      }
    }
    return undefined;
  };

  const caches = {
    open: async (name) => {
      if (!stores.has(name)) stores.set(name, new Map());
      const store = stores.get(name);
      return {
        add: async (url) => store.set(url, new FakeResponse(`cached ${url}`)),
        put: async (req, res) => store.set(typeof req === "string" ? req : req.url, res),
        match: async (req) => store.get(typeof req === "string" ? req : req.url),
      };
    },
    keys: async () => [...stores.keys()],
    delete: async (name) => stores.delete(name),
    match: async (req, options) => {
      for (const store of stores.values()) {
        const hit = match(store, req, options);
        if (hit) return hit;
      }
      return undefined;
    },
  };

  const self = {
    location: { origin },
    registration: { active: activeWorker ? {} : null },
    addEventListener: (type, fn) => (listeners[type] = fn),
    skipWaiting: () => { calls.skipWaiting++; return Promise.resolve(); },
    clients: { claim: () => { calls.claim++; return Promise.resolve(); } },
  };

  vm.runInNewContext(source, {
    self, caches, URL, Response: FakeResponse,
    fetch: (req) => { calls.fetch.push(req.url); return network(req); },
  });
  return { listeners, stores, calls };
}

class FakeResponse {
  constructor(body, ok = true, type = "basic") { this.body = body; this.ok = ok; this.type = type; }
  clone() { return this; }
  static error() { return new FakeResponse("error", false); }
}

const request = (path, init = {}) => ({ url: "https://shop.example" + path, method: "GET", mode: "cors", ...init });

async function dispatch(listeners, type, event) {
  let pending;
  listeners[type]({ ...event, waitUntil: (p) => (pending = p), respondWith: (p) => (pending = p) });
  return pending === undefined ? undefined : await pending;
}

test("install saves the offline page and takes over at once", async () => {
  const { listeners, stores, calls } = setup();
  await dispatch(listeners, "install", {});
  assert.ok(stores.get("sati-shell-v1").has("/offline.html"));
  assert.equal(calls.skipWaiting, 1);
});

test("an update waits for the user instead of skipping the waiting period", async () => {
  const { listeners, calls } = setup({ activeWorker: true });
  await dispatch(listeners, "install", {});
  assert.equal(calls.skipWaiting, 0);
});

test("activate removes old SATI caches and leaves others alone", async () => {
  const { listeners, stores, calls } = setup({
    existing: { "sati-shell-v0": {}, "sati-assets-v0": {}, "sati-shell-v1": {}, "other-app": {} },
  });
  await dispatch(listeners, "activate", {});
  assert.deepEqual([...stores.keys()].sort(), ["other-app", "sati-shell-v1"]);
  assert.equal(calls.claim, 1);
});

test("personal API calls are never touched, so personal data is never stored", async () => {
  const { listeners, stores } = setup();
  let responded = false;
  listeners.fetch({ request: request("/api/v1/auth/me"), respondWith: () => (responded = true) });
  assert.equal(responded, false);
  assert.equal(stores.size, 0);
});

test("public product and category responses are cached for offline menu use", async () => {
  const online = setup();
  const productsRequest = request("/api/v1/products?limit=100&offset=0");
  const categoriesRequest = request("/api/v1/categories");
  await dispatch(online.listeners, "fetch", { request: productsRequest });
  await dispatch(online.listeners, "fetch", { request: categoriesRequest });
  await new Promise((r) => setTimeout(r, 0));

  const cache = online.stores.get("sati-assets-v1");
  assert.equal(cache.get(productsRequest.url).body, "net");
  assert.equal(cache.get(categoriesRequest.url).body, "net");

  const offline = setup({
    network: () => Promise.reject(new Error("offline")),
    existing: { "sati-assets-v1": {
      [productsRequest.url]: new FakeResponse("saved products"),
      [categoriesRequest.url]: new FakeResponse("saved categories"),
    } },
  });
  const products = await dispatch(offline.listeners, "fetch", { request: productsRequest });
  const categories = await dispatch(offline.listeners, "fetch", { request: categoriesRequest });
  assert.equal(products.body, "saved products");
  assert.equal(categories.body, "saved categories");
});

test("only same-origin GET requests are handled", async () => {
  const { listeners } = setup();
  let responded = 0;
  const respondWith = () => responded++;
  listeners.fetch({ request: request("/images/a.png", { method: "POST" }), respondWith });
  listeners.fetch({ request: { url: "https://cdn.example/a.png", method: "GET", mode: "cors" }, respondWith });
  assert.equal(responded, 0);
});

test("a page that cannot be loaded falls back to the offline page", async () => {
  const { listeners } = setup({
    network: () => Promise.reject(new Error("offline")),
    existing: { "sati-shell-v1": { "/offline.html": new FakeResponse("offline page") } },
  });
  const res = await dispatch(listeners, "fetch", { request: request("/menu", { mode: "navigate" }) });
  assert.equal(res.body, "offline page");
});

test("a page that loads is served from the network", async () => {
  const { listeners } = setup();
  const res = await dispatch(listeners, "fetch", { request: request("/menu", { mode: "navigate" }) });
  assert.equal(res.body, "net");
});

test("the latest menu document is saved and served offline", async () => {
  const online = setup();
  await dispatch(online.listeners, "fetch", {
    request: request("/menu?source=pwa", { mode: "navigate" }),
  });
  assert.equal(online.stores.get("sati-shell-v1").get("/menu").body, "net");

  const offline = setup({
    network: () => Promise.reject(new Error("offline")),
    existing: { "sati-shell-v1": { "/menu": new FakeResponse("saved menu") } },
  });
  const res = await dispatch(offline.listeners, "fetch", {
    request: request("/menu?source=pwa", { mode: "navigate" }),
  });
  assert.equal(res.body, "saved menu");
});

test("static files are served from the copy on the device and refreshed in the background", async () => {
  const { listeners, stores, calls } = setup({
    existing: { "sati-assets-v1": { "https://shop.example/images/a.webp": new FakeResponse("old copy") } },
    network: () => Promise.resolve(new FakeResponse("new copy")),
  });
  const res = await dispatch(listeners, "fetch", { request: request("/images/a.webp") });
  assert.equal(res.body, "old copy");
  assert.equal(calls.fetch.length, 1);
  await new Promise((r) => setTimeout(r, 0));
  assert.equal(stores.get("sati-assets-v1").get("https://shop.example/images/a.webp").body, "new copy");
});

test("a static file seen for the first time is fetched and saved", async () => {
  const { listeners, stores } = setup();
  const res = await dispatch(listeners, "fetch", { request: request("/_next/static/chunks/app.js") });
  assert.equal(res.body, "net");
  await new Promise((r) => setTimeout(r, 0));
  assert.ok(stores.get("sati-assets-v1").has("https://shop.example/_next/static/chunks/app.js"));
});

test("same-origin images are cached even outside the images directory", async () => {
  const { listeners, stores } = setup();
  await dispatch(listeners, "fetch", { request: request("/media/catalog/item.webp") });
  await new Promise((r) => setTimeout(r, 0));
  assert.ok(stores.get("sati-assets-v1").has("https://shop.example/media/catalog/item.webp"));
});

test("public CDN images use stale-while-revalidate and cache opaque responses", async () => {
  const { listeners, stores } = setup({
    network: () => Promise.resolve(new FakeResponse("opaque image", false, "opaque")),
  });
  await dispatch(listeners, "fetch", {
    request: { url: "https://cdn.example/item", method: "GET", mode: "no-cors", destination: "image" },
  });
  await new Promise((r) => setTimeout(r, 0));
  assert.ok(stores.get("sati-assets-v1").has("https://cdn.example/item"));
});

test("failed responses are not saved", async () => {
  const { listeners, stores } = setup({ network: () => Promise.resolve(new FakeResponse("boom", false)) });
  await dispatch(listeners, "fetch", { request: request("/icons/icon-192.png") });
  await new Promise((r) => setTimeout(r, 0));
  assert.equal(stores.get("sati-assets-v1").size, 0);
});

test("an uncached image requested offline returns a network error response", async () => {
  const { listeners } = setup({ network: () => Promise.reject(new Error("offline")) });
  const res = await dispatch(listeners, "fetch", {
    request: request("/images/not-saved.webp"),
  });
  assert.equal(res.ok, false);
  assert.equal(res.body, "error");
});

test("other requests, such as html data, go straight to the network", async () => {
  const { listeners } = setup();
  let responded = false;
  listeners.fetch({ request: request("/some-data.json"), respondWith: () => (responded = true) });
  assert.equal(responded, false);
});
