"use client";

import { useRouter } from "next/navigation";
import { useStore } from "./store";
import { useCatalog, toCartItem } from "./catalog";
import { useToast } from "@/components/ui/Toast";
import type { ApiOrder } from "./api/orders";

/**
 * Puts an old order's items back in the bag using today's menu, so prices and
 * availability are current. Items that are gone or sold out are skipped, and we say so.
 */
export function useReorder() {
  const { reorder } = useStore();
  const { items, status } = useCatalog();
  const router = useRouter();
  const toast = useToast();

  return (order: Pick<ApiOrder, "items">) => {
    if (status !== "ready") return toast("The menu is still loading. Please try again in a moment.");
    const byId = new Map(items.map((i) => [i.id, i]));
    const lines = [];
    let skipped = 0;
    for (const line of order.items) {
      const product = byId.get(line.product_id);
      if (product?.available) lines.push({ ...toCartItem(product), qty: line.quantity });
      else skipped += 1;
    }
    if (lines.length === 0) {
      return toast("None of those items are available right now.", { tone: "error" });
    }
    reorder(lines);
    if (skipped > 0) {
      toast(`${skipped} item${skipped > 1 ? "s were" : " was"} no longer available and left out.`);
    }
    router.push("/order");
  };
}
