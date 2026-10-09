import type { Tone } from "@/lib/order-status";
// @ts-ignore: CSS module import for side effects
import "./status-badge.css";

/** A small coloured pill for an order or payment state. */
export default function StatusBadge({ label, tone }: { label: string; tone: Tone }) {
  return <span className={`status-badge is-${tone}`}>{label}</span>;
}
