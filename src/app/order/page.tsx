"use client";

import { useState } from "react";
import { useStore, type Order } from "@/lib/store";
import { ADDONS, naira } from "@/lib/menu";
import OrderSuccess from "@/components/OrderSuccess";
// @ts-ignore: CSS module import for side effects
import "./order.css";
import Img from "@/components/ui/Img";

const emailOk = (v: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v.trim());

export default function OrderPage() {
  const {
    cart,
    total,
    account,
    addQuiet,
    setAccount,
    setQty,
    remove,
    placeOrder,
    signOut,
  } = useStore();

  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [placed, setPlaced] = useState<Order | null>(null);

  // After an order is placed, show the success animation.
  if (placed) {
    return (
      <main className="order">
        <OrderSuccess order={placed} name={account?.name ?? name} />
      </main>
    );
  }

  // Step 1 — verify with name + email so orders can be looked up later.
  if (!account) {
    const verify = (e: React.FormEvent) => {
      e.preventDefault();
      if (name.trim().length < 2) return setError("Please enter your name.");
      if (!emailOk(email)) return setError("Please enter a valid email.");
      setError("");
      setAccount({ name: name.trim(), email: email.trim().toLowerCase() });
    };

    return (
      <main className="order">
        <div className="order__verify">
          <span className="order__eyebrow">Almost there</span>
          <h1 className="order__title">Verify to order</h1>
          <p className="order__lede">
            Pop in your name and email so we can save this order to your records you can look it up any time.
          </p>

          <form className="order-form" onSubmit={verify} noValidate>
            <label className="order-form__field">
              <span>Full name</span>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Ada Okeke"
                autoComplete="name"
              />
            </label>
            <label className="order-form__field">
              <span>Email</span>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@email.com"
                autoComplete="email"
              />
            </label>

            {error && <p className="order-form__error">{error}</p>}

            <button type="submit" className="order-form__submit">
              Continue
            </button>
          </form>

          <a href="/" className="order__back">
             Back to menu
          </a>
        </div>
      </main>
    );
  }

  // Step 2 — empty cart guard.
  if (cart.length === 0) {
    return (
      <main className="order">
        <div className="order__empty">
          <h1 className="order__title">Your bag is empty</h1>
          <p className="order__lede">Add something from the menu to order.</p>
          <a href="/" className="order-form__submit order-form__submit--link">
            Browse the menu
          </a>
        </div>
      </main>
    );
  }

  // Step 3 — review and place the order.
  const submit = () => {
    const order = placeOrder();
    if (order) setPlaced(order);
  };

  return (
    <main className="order">
      <div className="order__review">
        <header className="order__head">
          <div>
            <span className="order__eyebrow">Review &amp; confirm</span>
            <h1 className="order__title">Your order</h1>
          </div>
          <p className="order__who">
            {account.name}
            <span>{account.email}</span>
            <button type="button" className="order__switch" onClick={signOut}>
              Not you?
            </button>
          </p>
        </header>

        <ul className="order-lines">
          {cart.map((line) => (
            <li key={line.id} className="order-line">
              <Img src={line.image} alt={line.name} />
              <div className="order-line__body">
                <p className="order-line__name">{line.name}</p>
                <p className="order-line__price">{naira(line.price)} each</p>
              </div>
              <div className="order-line__qty">
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
              <span className="order-line__sum">
                {naira(line.price * line.qty)}
              </span>
              <button
                type="button"
                className="order-line__remove"
                aria-label={`Remove ${line.name}`}
                onClick={() => remove(line.id)}
              >
                ✕
              </button>
            </li>
          ))}
        </ul>

        {ADDONS.filter((a) => !cart.some((l) => l.id === a.id)).length > 0 && (
          <div className="upsell">
            <p className="upsell__title">Make it a feast 🍟</p>
            <div className="upsell__row">
              {ADDONS.filter((a) => !cart.some((l) => l.id === a.id)).map(
                (addon) => (
                  <button
                    key={addon.id}
                    type="button"
                    className="upsell__item"
                    onClick={() => addQuiet(addon)}
                  >
                    <Img src={addon.image} alt="" />
                    <span className="upsell__name">{addon.name}</span>
                    <span className="upsell__add">
                      + {naira(addon.price)}
                    </span>
                  </button>
                ),
              )}
            </div>
          </div>
        )}

        <div className="order__summary">
          <div className="order__total">
            <span>Total</span>
            <span>{naira(total)}</span>
          </div>
          <button
            type="button"
            className="order__place"
            onClick={submit}
          >
            Place order
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
          <a href="/" className="order__back">
             Add more items
          </a>
        </div>
      </div>
    </main>
  );
}
