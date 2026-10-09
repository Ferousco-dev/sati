"use client";

import { useEffect, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { useAuth } from "@/lib/auth";
import { isApiError, errorMessage } from "@/lib/api/client";
// @ts-ignore: CSS module import for side effects
import "@/components/styles/auth.css";

const DISMISSED = "sati:verify-dismissed";

/** A gentle reminder. It never blocks ordering and can be dismissed. */
export default function VerifyBanner() {
  const { user, resendVerification } = useAuth();
  const pathname = usePathname();
  const [hidden, setHidden] = useState(false);
  const [ready, setReady] = useState(false);
  const [message, setMessage] = useState("");
  const [cooldown, setCooldown] = useState(0);

  // It shows on every page until it is dismissed, and stays dismissed for this browser session.
  useEffect(() => {
    try {
      setHidden(sessionStorage.getItem(DISMISSED) === "1");
    } catch {}
    setReady(true);
  }, []);

  useEffect(() => {
    if (cooldown <= 0) return;
    const t = window.setTimeout(() => setCooldown((s) => s - 1), 1000);
    return () => window.clearTimeout(t);
  }, [cooldown]);

  const visible = ready && !!user && !user.email_verified && !hidden && pathname !== "/verify-email";
  const bar = useRef<HTMLDivElement>(null);

  // The banner floats under the top bar, so the page is pushed down by its height. Without this it
  // covers the first lines of every page, and on a phone, where it wraps, it hides the page heading.
  useEffect(() => {
    const root = document.documentElement;
    const el = bar.current;
    if (!visible || !el) return;
    const apply = () => root.style.setProperty("--verify-banner-h", `${el.offsetHeight}px`);
    apply();
    root.classList.add("has-verify-banner");
    const observer = new ResizeObserver(apply);
    observer.observe(el);
    return () => {
      observer.disconnect();
      root.classList.remove("has-verify-banner");
      root.style.removeProperty("--verify-banner-h");
    };
  }, [visible]);

  if (!visible) return null;

  const resend = async () => {
    try {
      await resendVerification();
      setMessage("Verification email sent.");
    } catch (err) {
      // RESEND_TOO_SOON is the 60 second cooldown. Registering already sent the first email.
      setMessage(errorMessage(err));
      if (!isApiError(err, "RESEND_TOO_SOON") && !isApiError(err, "RATE_LIMITED")) return;
    }
    setCooldown(60);
  };

  return (
    <div className="verify-banner" role="status" ref={bar}>
      <span>{message || "Please verify your email. You can keep ordering in the meantime."}</span>
      <button type="button" onClick={resend} disabled={cooldown > 0}>
        {cooldown > 0 ? `Resend in ${cooldown}s` : "Resend email"}
      </button>
      <button
        type="button"
        className="verify-banner__close"
        aria-label="Dismiss"
        onClick={() => {
          setHidden(true);
          try {
            sessionStorage.setItem(DISMISSED, "1");
          } catch {}
        }}
      >
        ×
      </button>
    </div>
  );
}
