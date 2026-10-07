"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { useStore } from "@/lib/store";
import { naira } from "@/lib/menu";
// @ts-ignore: CSS module import for side effects
import "./cart-drawer.css";

export default function CartDrawer() {
  const { cart, count, total, cartOpen, setCartOpen, remove, setQty } =
    useStore();
  const router = useRouter();

  // Lock body scroll while the drawer is open.
  useEffect(() => {
    if (!cartOpen) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setCartOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener("keydown", onKey);
    };
  }, [cartOpen, setCartOpen]);

  const checkout = () => {
    setCartOpen(false);
    router.push("/order");
  };

  return (
    <div className={`drawer ${cartOpen ? "is-open" : ""}`} aria-hidden={!cartOpen}>
      <div className="drawer__scrim" onClick={() => setCartOpen(false)} />

      <aside
        className="drawer__panel"
        role="dialog"
        aria-modal="true"
        aria-label="Your cart"
      >
        <header className="drawer__head">
          <h3 className="drawer__title">
            Your bag {count > 0 && <span>({count})</span>}
          </h3>
          <button
            type="button"
            className="drawer__close"
            aria-label="Close cart"
            onClick={() => setCartOpen(false)}
          >
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M6 6l12 12M18 6L6 18"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
              />
            </svg>
          </button>
        </header>

        {cart.length === 0 ? (
          <div className="drawer__empty">
            <p>Your bag is empty.</p>
            <span>Add something tasty from the menu.</span>
            <button
              type="button"
              className="drawer__browse"
              onClick={() => setCartOpen(true)}
            >
              Browse the menu
            </button>
          </div>
        ) : (
          <ul className="drawer__list">
            {cart.map((line) => (
              <li key={line.id} className="drawer-line">
                <img
                  className="drawer-line__img"
                  src={line.image}
                  alt={line.name}
                />
                <div className="drawer-line__body">
                  <p className="drawer-line__name">{line.name}</p>
                  <p className="drawer-line__price">{naira(line.price)}</p>
                  <div className="drawer-line__qty">
                    <button
                      type="button"
                      aria-label={`Decrease ${line.name}`}
                      onClick={() => setQty(line.id, line.qty - 1)}
                    >
                      −
                    </button>
                    <span>{line.qty}</span>
                    <button
                      type="button"
                      aria-label={`Increase ${line.name}`}
                      onClick={() => setQty(line.id, line.qty + 1)}
                    >
                      +
                    </button>
                  </div>
                </div>
                <button
                  type="button"
                  className="drawer-line__remove"
                  aria-label={`Remove ${line.name}`}
                  onClick={() => remove(line.id)}
                >
                  Remove
                </button>
              </li>
            ))}
          </ul>
        )}

        <footer className="drawer__foot">
          <div className="drawer__total">
            <span>Total</span>
            <span>{naira(total)}</span>
          </div>
          {cart.length === 0 ? (
            <button
              type="button"
              className="drawer__continue"
              onClick={() => setCartOpen(false)}
            >
              Continue shopping
            </button>
          ) : (
            <button
              type="button"
              className="drawer__checkout"
              onClick={checkout}
            >
              Checkout
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <path
                  d="M5 12h14M13 6l6 6-6 6"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
            </button>
          )}
        </footer>
      </aside>
    </div>
  );
}
