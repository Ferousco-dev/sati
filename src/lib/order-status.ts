// Order status is derived purely from how long ago the order was placed,
// so it advances on its own with no backend — perfect for a mocked demo.

export type OrderStage = {
  key: string;
  label: string;
  caption: string;
  // minutes after placedAt at which this stage becomes active
  at: number;
};

// Compressed timeline so the whole journey plays out in ~2 minutes for a demo.
export const STAGES: OrderStage[] = [
  { key: "placed", label: "Order placed", caption: "We got your order", at: 0 },
  {
    key: "preparing",
    label: "Preparing",
    caption: "On the grill now",
    at: 0.25,
  },
  {
    key: "out",
    label: "Out for delivery",
    caption: "Your rider is on the way",
    at: 1,
  },
  {
    key: "delivered",
    label: "Delivered",
    caption: "Enjoy your meal!",
    at: 2,
  },
];

export const ETA_MINUTES = STAGES[STAGES.length - 1].at;

// Index of the current stage given elapsed time (ms) since the order was placed.
export function currentStageIndex(placedAt: number, now: number): number {
  const elapsedMin = (now - placedAt) / 60000;
  let idx = 0;
  for (let i = 0; i < STAGES.length; i++) {
    if (elapsedMin >= STAGES[i].at) idx = i;
  }
  return idx;
}

export function isDelivered(placedAt: number, now: number): boolean {
  return currentStageIndex(placedAt, now) >= STAGES.length - 1;
}
