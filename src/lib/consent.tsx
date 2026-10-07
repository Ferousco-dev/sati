"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";

export type ConsentState = "unset" | "accepted" | "declined";

// The choice itself is strictly necessary, so it is the one thing stored without asking.
const CONSENT_KEY = "sati:consent";
const PREFIX = "sati:";

function readConsent(): ConsentState {
  try {
    const v = localStorage.getItem(CONSENT_KEY);
    return v === "accepted" || v === "declined" ? v : "unset";
  } catch {
    return "unset";
  }
}

function clearOptional() {
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

type ConsentContext = {
  state: ConsentState;
  /** False until the saved choice has been read in the browser. */
  ready: boolean;
  accept: () => void;
  decline: () => void;
  /** Forget the choice, delete optional data and ask again. */
  reset: () => void;
};

const Ctx = createContext<ConsentContext | null>(null);

export function ConsentProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<ConsentState>("unset");
  const [ready, setReady] = useState(false);

  useEffect(() => {
    setState(readConsent());
    setReady(true);
  }, []);

  const save = useCallback((next: ConsentState) => {
    try {
      if (next === "unset") localStorage.removeItem(CONSENT_KEY);
      else localStorage.setItem(CONSENT_KEY, next);
    } catch {}
    if (next !== "accepted") clearOptional();
    setState(next);
  }, []);

  const value = useMemo<ConsentContext>(
    () => ({
      state,
      ready,
      accept: () => save("accepted"),
      decline: () => save("declined"),
      reset: () => save("unset"),
    }),
    [state, ready, save],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useConsent(): ConsentContext {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useConsent must be used within ConsentProvider");
  return ctx;
}
