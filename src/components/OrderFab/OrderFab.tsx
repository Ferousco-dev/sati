"use client";

import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useStore } from "@/lib/store";
// @ts-ignore: CSS module import for side effects
import "@/components/styles/floating.css";

// Pages where the button would only get in the way.
const HIDDEN_ON = ["/order", "/account", "/admin"];

/**
 * The main action on every screen size, a floating button at the bottom right where a thumb reaches it,
 * under the WhatsApp button. It opens the cart when there is something in it, and the menu when there is not.
 */
export default function OrderFab() {
  const { count, cartOpen, setCartOpen } = useStore();
  const pathname = usePathname();
  const router = useRouter();

  const onPage = !HIDDEN_ON.some((p) => pathname === p || pathname.startsWith(p + "/"));

  useEffect(() => {
    document.documentElement.classList.toggle("has-order-fab", onPage);
    return () => document.documentElement.classList.remove("has-order-fab");
  }, [onPage]);

  if (!onPage) return null;

  return (
    <button
      type="button"
      className={`order-fab ${cartOpen ? "is-hidden" : ""}`}
      aria-label={count > 0 ? `View cart, ${count} items` : "Order now"}
      tabIndex={cartOpen ? -1 : 0}
      onClick={() => (count > 0 ? setCartOpen(true) : router.push("/menu"))}
    >
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path
          d="M3 4h2l2.2 11.2a1.5 1.5 0 0 0 1.5 1.2h8.6a1.5 1.5 0 0 0 1.5-1.1L20.5 8H6.1M10 20.5a.9.9 0 1 0 0-.01M17 20.5a.9.9 0 1 0 0-.01"
          stroke="currentColor"
          strokeWidth="1.9"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      {count > 0 ? <span className="order-fab__count">{count}</span> : null}
    </button>
  );
}
