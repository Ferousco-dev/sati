// The real order statuses from the API, and how each one reads to a customer.
import type { Fulfillment } from "./api/orders";

export type Step = { key: string; label: string; caption: string };

const PLACED: Step = { key: "PENDING", label: "Order placed", caption: "We got your order" };
const CONFIRMED: Step = { key: "CONFIRMED", label: "Confirmed", caption: "We've accepted your order" };
const PREPARING: Step = { key: "PREPARING", label: "Preparing", caption: "Being prepared fresh" };

const STEPS: Record<Fulfillment, Step[]> = {
  PICKUP: [
    PLACED,
    CONFIRMED,
    PREPARING,
    { key: "READY", label: "Ready", caption: "Ready for you to collect" },
    { key: "COMPLETED", label: "Collected", caption: "Enjoy!" },
  ],
  DELIVERY: [
    PLACED,
    CONFIRMED,
    PREPARING,
    { key: "OUT_FOR_DELIVERY", label: "On the way", caption: "Your rider is on the way" },
    { key: "DELIVERED", label: "Delivered", caption: "Enjoy!" },
  ],
};

export const stepsFor = (fulfillment: Fulfillment) => STEPS[fulfillment];

/** Where an order is on its journey. Statuses a journey doesn't list sit on the nearest step. */
export function stepIndex(status: string, fulfillment: Fulfillment): number {
  const steps = STEPS[fulfillment];
  const found = steps.findIndex((s) => s.key === status);
  if (found >= 0) return found;
  if (status === "READY") return steps.findIndex((s) => s.key === "PREPARING");
  if (status === "COMPLETED" || status === "DELIVERED") return steps.length - 1;
  return 0;
}

export const isStopped = (status: string) => status === "CANCELLED" || status === "FAILED";
export const isFinished = (status: string) =>
  isStopped(status) || status === "COMPLETED" || status === "DELIVERED";

const LABEL: Record<string, string> = {
  PENDING: "Placed",
  CONFIRMED: "Confirmed",
  PREPARING: "Preparing",
  READY: "Ready",
  OUT_FOR_DELIVERY: "On the way",
  DELIVERED: "Delivered",
  COMPLETED: "Completed",
  CANCELLED: "Cancelled",
  FAILED: "Failed",
};

export const statusLabel = (status: string) => LABEL[status] ?? status;

export type Tone = "good" | "warn" | "bad" | "plain";

export function statusTone(status: string): Tone {
  if (status === "COMPLETED" || status === "DELIVERED") return "good";
  if (isStopped(status)) return "bad";
  return "warn";
}

const PAY_LABEL: Record<string, string> = {
  UNPAID: "Unpaid",
  PENDING: "Payment pending",
  PAID: "Paid",
  FAILED: "Payment failed",
  REFUNDED: "Refunded",
  PARTIALLY_REFUNDED: "Partly refunded",
};

export const paymentLabel = (status: string) => PAY_LABEL[status] ?? status;

export function paymentTone(status: string): Tone {
  if (status === "PAID") return "good";
  if (status === "FAILED") return "bad";
  if (status === "REFUNDED" || status === "PARTIALLY_REFUNDED") return "plain";
  return "warn";
}

export const METHOD_NAME: Record<string, string> = {
  ONLINE: "Online",
  BANK_TRANSFER: "Bank transfer",
  PAY_ON_PICKUP: "Pay on pickup",
  PAY_ON_DELIVERY: "Pay on delivery",
};
