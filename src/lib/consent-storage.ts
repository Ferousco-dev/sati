export type ConsentState = "unset" | "accepted" | "declined";

// The choice itself is strictly necessary, so it is the one thing stored without asking.
export const CONSENT_KEY = "sati:consent";
const PREFIX = "sati:";

export function readConsent(): ConsentState {
  try {
    const v = localStorage.getItem(CONSENT_KEY);
    return v === "accepted" || v === "declined" ? v : "unset";
  } catch {
    return "unset";
  }
}

export function clearOptional() {
  try {
    for (const key of Object.keys(localStorage)) {
      if (key.startsWith(PREFIX) && key !== CONSENT_KEY) {
        localStorage.removeItem(key);
      }
    }
  } catch {}
}

/**
 * Storage for anything optional, such as the cart or a remembered filter.
 * Nothing is written until the visitor has accepted, and reading returns
 * nothing until then. The session cookie is strictly necessary and does not use this.
 */
export const optionalStorage = {
  allowed(): boolean {
    return typeof window !== "undefined" && readConsent() === "accepted";
  },
  get(key: string): string | null {
    if (!this.allowed()) return null;
    try {
      return localStorage.getItem(PREFIX + key);
    } catch {
      return null;
    }
  },
  set(key: string, value: string): boolean {
    if (!this.allowed()) return false;
    try {
      localStorage.setItem(PREFIX + key, value);
      return true;
    } catch {
      return false;
    }
  },
  remove(key: string) {
    try {
      localStorage.removeItem(PREFIX + key);
    } catch {}
  },
};

