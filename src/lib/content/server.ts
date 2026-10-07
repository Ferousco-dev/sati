import { defaultContent, defaultStore } from "./defaults";
import type { SiteContent, StoreInfo, Zone } from "./types";

// Server side only. Pages that use this are rebuilt in the background every minute,
// so an edit in the admin shows up quickly without slowing any visitor down.
const API = process.env.API_ORIGIN ?? "http://localhost:8080";
const REVALIDATE_SECONDS = 60;

async function getJson<T>(path: string): Promise<T | null> {
  try {
    const res = await fetch(API + path, { next: { revalidate: REVALIDATE_SECONDS } });
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch {
    return null;
  }
}

/** Live content, with the built-in default for any section that is missing. */
export async function getSiteContent(): Promise<SiteContent> {
  const data = await getJson<{ sections: Partial<SiteContent> }>("/api/v1/content");
  return { ...defaultContent, ...(data?.sections ?? {}) };
}

export async function getStoreInfo(): Promise<StoreInfo> {
  const data = await getJson<{ store: Partial<StoreInfo> }>("/api/v1/store");
  return { ...defaultStore, ...(data?.store ?? {}) };
}

export async function getZones(): Promise<Zone[]> {
  const data = await getJson<{ zones: Zone[] }>("/api/v1/delivery/zones");
  return data?.zones ?? [];
}
