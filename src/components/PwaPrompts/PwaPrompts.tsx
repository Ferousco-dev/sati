"use client";

import { usePwa } from "@/lib/pwa";
// @ts-ignore: CSS module import for side effects
import "./pwa-prompts.css";

export default function PwaPrompts() {
  const { updateAvailable, update } = usePwa();

  return (
    <>
      {updateAvailable ? (
        <aside className="pwa-update" role="status" aria-live="polite">
          <span>A new version is ready.</span>
          <button type="button" onClick={update}>Update</button>
        </aside>
      ) : null}
    </>
  );
}