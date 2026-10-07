// Run with: npm run test:lib
import { test } from "node:test";
import assert from "node:assert/strict";
import { api, ApiError, errorMessage, isApiError } from "./client.ts";
import { formatKobo } from "../money.ts";

type FetchArgs = { url: string; init: RequestInit };

function mockFetch(respond: (args: FetchArgs) => Response | Promise<Response>) {
  const calls: FetchArgs[] = [];
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    calls.push({ url, init });
    return respond({ url, init });
  }) as typeof fetch;
  return calls;
}

const json = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), { headers: { "Content-Type": "application/json" }, ...init });

test("requests always send cookies and ask for JSON", async () => {
  const calls = mockFetch(() => json({ ok: true }));
  await api("/api/v1/auth/me");
  assert.equal(calls[0].init.credentials, "include");
  assert.equal((calls[0].init.headers as Record<string, string>).Accept, "application/json");
});

test("a JSON body sets the content type and is serialised", async () => {
  const calls = mockFetch(() => json({}, { status: 201 }));
  await api.post("/api/v1/orders", { items: [] }, { idempotencyKey: "abc" });
  const headers = calls[0].init.headers as Record<string, string>;
  assert.equal(headers["Content-Type"], "application/json");
  assert.equal(headers["Idempotency-Key"], "abc");
  assert.equal(calls[0].init.body, '{"items":[]}');
  assert.equal(calls[0].init.method, "POST");
});

test("query values are encoded and empty ones are left out", async () => {
  const calls = mockFetch(() => json({}));
  await api.get("/api/v1/products", { q: "cake & tea", limit: 20, category_id: undefined, empty: "" });
  assert.equal(calls[0].url, "/api/v1/products?q=cake+%26+tea&limit=20");
});

test("form data is sent as is, without a JSON content type", async () => {
  const calls = mockFetch(() => json({}));
  const form = new FormData();
  form.append("file", "x");
  await api.post("/api/v1/admin/media", form);
  assert.equal(calls[0].init.body, form);
  assert.equal((calls[0].init.headers as Record<string, string>)["Content-Type"], undefined);
});

test("204 answers with no body", async () => {
  mockFetch(() => new Response(null, { status: 204 }));
  assert.equal(await api.post("/api/v1/auth/logout"), undefined);
});

test("API errors keep their stable code and message", async () => {
  mockFetch(() => json({ error: { code: "EMAIL_TAKEN", message: "An account with this email already exists" } }, { status: 409 }));
  await assert.rejects(api.post("/api/v1/auth/register", {}), (err: unknown) => {
    assert.ok(isApiError(err, "EMAIL_TAKEN"));
    assert.equal((err as ApiError).status, 409);
    assert.equal(errorMessage(err), "An account with this email already exists");
    return true;
  });
});

test("a 429 carries how long to wait", async () => {
  mockFetch(() => json({ error: { code: "RATE_LIMITED", message: "Too many requests" } }, { status: 429, headers: { "Retry-After": "120" } }));
  await assert.rejects(api.post("/api/v1/auth/login", {}), (err: unknown) => {
    assert.equal((err as ApiError).retryAfter, 120);
    assert.match(errorMessage(err), /2 minutes/);
    return true;
  });
});

test("short waits are shown in seconds", () => {
  assert.match(errorMessage(new ApiError(429, "RATE_LIMITED", "x", 30)), /30 seconds/);
  assert.match(errorMessage(new ApiError(429, "RATE_LIMITED", "x")), /shortly/);
});

test("an unreadable error body still gives a friendly error", async () => {
  mockFetch(() => new Response("<html>bad gateway</html>", { status: 502 }));
  await assert.rejects(api("/api/v1/health"), (err: unknown) => {
    assert.equal((err as ApiError).status, 502);
    assert.equal((err as ApiError).code, "UNKNOWN");
    return true;
  });
});

test("being offline becomes a NETWORK_ERROR", async () => {
  globalThis.fetch = (async () => {
    throw new TypeError("Failed to fetch");
  }) as typeof fetch;
  await assert.rejects(api("/api/v1/health"), (err: unknown) => isApiError(err, "NETWORK_ERROR"));
});

test("a cancelled request is not turned into an API error", async () => {
  globalThis.fetch = (async () => {
    throw new DOMException("aborted", "AbortError");
  }) as typeof fetch;
  await assert.rejects(api("/api/v1/health"), (err: unknown) => !(err instanceof ApiError));
});

test("unknown errors get a generic message", () => {
  assert.equal(errorMessage(new Error("boom")), "Something went wrong. Please try again.");
});

test("kobo are shown as naira", () => {
  assert.match(formatKobo(750000), /7,500/);
  assert.match(formatKobo(250050), /2,500\.5/);
  assert.match(formatKobo(0), /0/);
});
