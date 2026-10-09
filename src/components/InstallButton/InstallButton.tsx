"use client";

import { useState } from "react";
import { usePwa } from "@/lib/pwa";
import Modal from "@/components/ui/Modal";
// @ts-ignore: CSS module import for side effects
import "@/components/ui/ui.css";

/**
 * Offers to install the app. Chrome and Android show their own prompt.
 * iPhones cannot be prompted from code, so they get short instructions.
 */
export default function InstallButton({ className = "" }: { className?: string }) {
  const { canInstall, needsIosInstructions, install } = usePwa();
  const [help, setHelp] = useState(false);

  if (!canInstall && !needsIosInstructions) return null;

  return (
    <>
      <button
        type="button"
        className={`ui-btn is-accent ${className}`}
        onClick={() => (canInstall ? void install() : setHelp(true))}
      >
        Install app
      </button>

      <Modal
        open={help}
        title="Install SATI-L"
        onClose={() => setHelp(false)}
        actions={
          <button type="button" className="ui-btn is-primary" onClick={() => setHelp(false)}>
            Got it
          </button>
        }
      >
        <ol style={{ margin: 0, paddingLeft: "1.25rem", lineHeight: 1.7 }}>
          <li>Tap the Share button in Safari.</li>
          <li>Scroll down and tap &ldquo;Add to Home Screen&rdquo;.</li>
          <li>Tap &ldquo;Add&rdquo;. SATI-L now opens like an app.</li>
        </ol>
      </Modal>
    </>
  );
}
