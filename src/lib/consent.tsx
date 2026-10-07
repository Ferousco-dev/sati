"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { clearOptional, CONSENT_KEY, readConsent, optionalStorage, type ConsentState } from "./consent-storage";

export { optionalStorage };
export type { ConsentState };

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
