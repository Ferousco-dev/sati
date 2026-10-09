"use client";

import { useConsent } from "@/lib/consent";
// @ts-ignore: CSS module import for side effects
import "@/components/styles/floating.css";
// @ts-ignore: CSS module import for side effects
import "@/components/ui/ui.css";

/** Asks once before anything optional, like a saved cart, is stored on the device. */
export default function ConsentBar() {
  const { state, ready, accept, decline } = useConsent();
  if (!ready || state !== "unset") return null;

  return (
    <div className="consent" role="region" aria-label="Storage choice">
      <p className="consent__title">Save your cart?</p>
      <p className="consent__text">
        We can keep your cart and preferences on this device so they are here when you come back. Nothing is
        saved unless you say yes. You can change your mind any time. See our{" "}
        <a href="/privacy">privacy notice</a>.
      </p>
      <div className="consent__actions">
        <button type="button" className="ui-btn is-ghost" onClick={decline}>
          No thanks
        </button>
        <button type="button" className="ui-btn is-primary" onClick={accept}>
          Yes, save it
        </button>
      </div>
    </div>
  );
}
