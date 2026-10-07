"use client";

import { useEffect, useState } from "react";
import {
  STAGES,
  currentStageIndex,
  isDelivered,
} from "@/lib/order-status";
// @ts-ignore: CSS module import for side effects
import "./order-tracker.css";

// Live status timeline for an order. Status is derived from elapsed time,
// so it advances on its own (mocked, no backend). Ticks once a second until
// the order is delivered, then stops.
export default function OrderTracker({ placedAt }: { placedAt: number }) {
  const [now, setNow] = useState(placedAt);

  // Sync to the real clock on mount (avoids SSR/hydration time mismatch).
  useEffect(() => {
    setNow(Date.now());
    const id = setInterval(() => {
      const t = Date.now();
      setNow(t);
      if (isDelivered(placedAt, t)) clearInterval(id);
    }, 1000);
    return () => clearInterval(id);
  }, [placedAt]);

  const active = currentStageIndex(placedAt, now);
  const done = active >= STAGES.length - 1;
  // Progress along the rail (0..1) for the connecting line.
  const progress = active / (STAGES.length - 1);

  return (
    <div className={`tracker ${done ? "is-done" : ""}`}>
      <div className="tracker__head">
        <span className="tracker__status">{STAGES[active].label}</span>
        <span className="tracker__caption">{STAGES[active].caption}</span>
      </div>

      <ol className="tracker__steps">
        <span className="tracker__rail" aria-hidden="true">
          <span
            className="tracker__rail-fill"
            style={{ width: `${progress * 100}%` }}
          />
        </span>
        {STAGES.map((stage, i) => {
          const state =
            i < active ? "is-complete" : i === active ? "is-active" : "";
          return (
            <li key={stage.key} className={`tracker__step ${state}`}>
              <span className="tracker__dot">
                {i < active || done ? (
                  <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                    <path
                      d="M5 13l4 4L19 7"
                      stroke="currentColor"
                      strokeWidth="2.5"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    />
                  </svg>
                ) : (
                  <span className="tracker__dot-inner" />
                )}
              </span>
              <span className="tracker__label">{stage.label}</span>
            </li>
          );
        })}
      </ol>
    </div>
  );
}
