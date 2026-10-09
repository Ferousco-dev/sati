import type { Fulfillment } from "@/lib/api/orders";
import { isStopped, stepIndex, stepsFor, statusLabel } from "@/lib/order-status";
// @ts-ignore: CSS module import for side effects
import "./order-tracker.css";

/** The order's real journey, from the status the API reports. Nothing here is simulated. */
export default function OrderTracker({
  status,
  fulfillment,
  reason,
}: {
  status: string;
  fulfillment: Fulfillment;
  /** Why an order was cancelled or failed. */
  reason?: string;
}) {
  if (isStopped(status)) {
    return (
      <div className="tracker is-stopped" role="status">
        <div className="tracker__head">
          <span className="tracker__status">Order {statusLabel(status).toLowerCase()}</span>
          <span className="tracker__caption">{reason || "This order will not be fulfilled."}</span>
        </div>
      </div>
    );
  }

  const steps = stepsFor(fulfillment);
  const active = stepIndex(status, fulfillment);
  const done = active >= steps.length - 1;
  const progress = active / (steps.length - 1);

  return (
    <div className={`tracker ${done ? "is-done" : ""}`} role="status" aria-live="polite">
      <div className="tracker__head">
        <span className="tracker__status">{steps[active].label}</span>
        <span className="tracker__caption">{steps[active].caption}</span>
      </div>

      <ol className="tracker__steps">
        <span className="tracker__rail" aria-hidden="true">
          <span className="tracker__rail-fill" style={{ width: `${progress * 100}%` }} />
        </span>
        {steps.map((step, i) => {
          const state = i < active || done ? "is-complete" : i === active ? "is-active" : "";
          return (
            <li key={step.key} className={`tracker__step ${state}`}>
              <span className="tracker__dot">
                {i < active || done ? (
                  <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                    <path d="M5 13l4 4L19 7" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                ) : (
                  <span className="tracker__dot-inner" />
                )}
              </span>
              <span className="tracker__label">{step.label}</span>
            </li>
          );
        })}
      </ol>
    </div>
  );
}
