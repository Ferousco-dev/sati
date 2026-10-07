"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { usePathname, useRouter } from "next/navigation";

// Chrome and Android fire this when the app can be installed. It is not in the standard DOM types.
type InstallPromptEvent = Event & {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
};

type PwaContext = {
  /** True when the browser can show its own install prompt. */
  canInstall: boolean;
  /** True when the app is running from the home screen. */
  isStandalone: boolean;
  /** iPhones cannot be prompted from code, so they are shown instructions instead. */
  needsIosInstructions: boolean;
  install: () => Promise<"accepted" | "dismissed" | "unavailable">;
};

const Ctx = createContext<PwaContext | null>(null);

function detectStandalone(): boolean {
  if (typeof window === "undefined") return false;
  const iosStandalone = (navigator as Navigator & { standalone?: boolean }).standalone === true;
  return window.matchMedia("(display-mode: standalone)").matches || iosStandalone;
}

function detectIos(): boolean {
  if (typeof navigator === "undefined") return false;
  const ua = navigator.userAgent;
  const iPadOnMac = navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1;
  return /iPhone|iPad|iPod/.test(ua) || iPadOnMac;
}

export function PwaProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const [prompt, setPrompt] = useState<InstallPromptEvent | null>(null);
  const [isStandalone, setStandalone] = useState(false);
  const [isIos, setIos] = useState(false);

  useEffect(() => {
    setStandalone(detectStandalone());
    setIos(detectIos());

    const media = window.matchMedia("(display-mode: standalone)");
    const onDisplayChange = () => setStandalone(detectStandalone());
    media.addEventListener("change", onDisplayChange);

    const onPrompt = (e: Event) => {
      e.preventDefault();
      setPrompt(e as InstallPromptEvent);
    };
    const onInstalled = () => {
      setPrompt(null);
      setStandalone(true);
    };
    window.addEventListener("beforeinstallprompt", onPrompt);
    window.addEventListener("appinstalled", onInstalled);

    if ("serviceWorker" in navigator && process.env.NODE_ENV === "production") {
      navigator.serviceWorker.register("/sw.js").catch(() => {});
    }

    return () => {
      media.removeEventListener("change", onDisplayChange);
      window.removeEventListener("beforeinstallprompt", onPrompt);
      window.removeEventListener("appinstalled", onInstalled);
    };
  }, []);

  // The installed app has no landing page, it opens straight on the menu.
  useEffect(() => {
    if (isStandalone && pathname === "/") router.replace("/menu");
  }, [isStandalone, pathname, router]);

  const install = useCallback(async () => {
    if (!prompt) return "unavailable" as const;
    await prompt.prompt();
    const { outcome } = await prompt.userChoice;
    setPrompt(null);
    return outcome;
  }, [prompt]);

  const value = useMemo<PwaContext>(
    () => ({
      canInstall: prompt !== null && !isStandalone,
      isStandalone,
      needsIosInstructions: isIos && !isStandalone,
      install,
    }),
    [prompt, isStandalone, isIos, install],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function usePwa(): PwaContext {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("usePwa must be used within PwaProvider");
  return ctx;
}
