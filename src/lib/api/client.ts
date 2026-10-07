import type { ApiErrorBody } from "./types";

// Empty means the API is on the same origin, which is how the dev proxy and production run.
const BASE = process.env.NEXT_PUBLIC_API_URL ?? "";

export class ApiError extends Error {
  status: number;
  code: string;
  /** Seconds to wait before trying again, from a 429 response. */
  retryAfter?: number;

  constructor(status: number, code: string, message: string, retryAfter?: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.retryAfter = retryAfter;
  }
}

export function isApiError(err: unknown, code?: string): err is ApiError {
  return err instanceof ApiError && (code === undefined || err.code === code);
}

/** A readable message for any error, safe to show to a customer. */
export function errorMessage(err: unknown): string {
  if (isApiError(err, "RATE_LIMITED")) {
    return err.retryAfter
      ? `Too many attempts. Please try again in ${formatWait(err.retryAfter)}.`
      : "Too many attempts. Please try again shortly.";
  }
  if (err instanceof ApiError) return err.message;
  return "Something went wrong. Please try again.";
}

function formatWait(seconds: number): string {
  if (seconds < 90) return `${Math.max(1, Math.round(seconds))} seconds`;
  return `${Math.ceil(seconds / 60)} minutes`;
}

type Query = Record<string, string | number | boolean | undefined | null>;

type Options = {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  query?: Query;
  /** Makes a retried request safe. Used when placing orders. */
  idempotencyKey?: string;
  signal?: AbortSignal;
};

function buildUrl(path: string, query?: Query): string {
  const url = BASE + path;
  if (!query) return url;
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v !== undefined && v !== null && v !== "") params.set(k, String(v));
  }
  const qs = params.toString();
  return qs ? `${url}?${qs}` : url;
}

/**
 * Calls the SATI API. Cookies are always sent, so the signed-in session works.
 * Failures throw an ApiError carrying the API's stable error code.
 */
export async function api<T>(path: string, options: Options = {}): Promise<T> {
  const { method = "GET", body, query, idempotencyKey, signal } = options;

  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined && !(body instanceof FormData)) {
    headers["Content-Type"] = "application/json";
  }
  if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;

  let res: Response;
  try {
    res = await fetch(buildUrl(path, query), {
      method,
      headers,
      credentials: "include",
      signal,
      body:
        body === undefined
          ? undefined
          : body instanceof FormData
            ? body
            : JSON.stringify(body),
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError(0, "NETWORK_ERROR", "Could not reach the server. Check your connection.");
  }

  if (res.status === 204) return undefined as T;

  let data: unknown = null;
  try {
    data = await res.json();
  } catch {
    // An empty or non-JSON body is handled below.
  }

  if (!res.ok) {
    const failure = (data as ApiErrorBody | null)?.error;
    const retry = Number(res.headers.get("Retry-After"));
    throw new ApiError(
      res.status,
      failure?.code ?? "UNKNOWN",
      failure?.message ?? "Something went wrong. Please try again.",
      Number.isFinite(retry) && retry > 0 ? retry : undefined,
    );
  }
  return data as T;
}

api.get = <T>(path: string, query?: Query, signal?: AbortSignal) =>
  api<T>(path, { query, signal });
api.post = <T>(path: string, body?: unknown, options: Omit<Options, "method" | "body"> = {}) =>
  api<T>(path, { ...options, method: "POST", body });
api.put = <T>(path: string, body?: unknown) => api<T>(path, { method: "PUT", body });
api.patch = <T>(path: string, body?: unknown) => api<T>(path, { method: "PATCH", body });
api.delete = <T>(path: string) => api<T>(path, { method: "DELETE" });

export const newIdempotencyKey = () => crypto.randomUUID();
