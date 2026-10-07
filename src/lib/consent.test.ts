import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";

// A tiny stand-in for the browser's storage.
const data = new Map<string, string>();
const fakeStorage = {
  getItem: (k: string) => (data.has(k) ? data.get(k)! : null),
  setItem: (k: string, v: string) => void data.set(k, v),
  removeItem: (k: string) => void data.delete(k),
  key: (i: number) => [...data.keys()][i] ?? null,
  get length() {
    return data.size;
  },
};
Object.defineProperty(fakeStorage, Symbol.iterator, { value: undefined });
(globalThis as unknown as { window: unknown }).window = {};
(globalThis as unknown as { localStorage: unknown }).localStorage = new Proxy(fakeStorage, {
  ownKeys: () => [...data.keys()],
  getOwnPropertyDescriptor: (_t, key) => (data.has(String(key)) ? { enumerable: true, configurable: true, value: data.get(String(key)) } : undefined),
});

const { optionalStorage } = await import("./consent-storage.ts");

beforeEach(() => data.clear());

test("nothing is written before the visitor accepts", () => {
  assert.equal(optionalStorage.set("v1", "cart"), false);
  assert.equal(data.size, 0);
  assert.equal(optionalStorage.get("v1"), null);
});

test("declining keeps storage closed", () => {
  data.set("sati:consent", "declined");
  assert.equal(optionalStorage.set("v1", "cart"), false);
  assert.equal(data.has("sati:v1"), false);
});

test("after accepting, data is saved under the sati prefix and can be read back", () => {
  data.set("sati:consent", "accepted");
  assert.equal(optionalStorage.set("v1", "cart"), true);
  assert.equal(data.get("sati:v1"), "cart");
  assert.equal(optionalStorage.get("v1"), "cart");
  optionalStorage.remove("v1");
  assert.equal(optionalStorage.get("v1"), null);
});

test("saved data is hidden again if consent is later withdrawn", () => {
  data.set("sati:consent", "accepted");
  optionalStorage.set("v1", "cart");
  data.set("sati:consent", "declined");
  assert.equal(optionalStorage.get("v1"), null);
});
