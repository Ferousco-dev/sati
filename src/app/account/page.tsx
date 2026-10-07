"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useStore } from "@/lib/store";
import { naira } from "@/lib/menu";
import OrderTracker from "@/components/OrderTracker";
// @ts-ignore: CSS module import for side effects
import "../order/order.css";
// @ts-ignore: CSS module import for side effects
import "./account.css";

const emailOk = (v: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v.trim());

export default function AccountPage() {
  const { account, orders, setAccount, signOut, reorder } = useStore();
  const router = useRouter();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");

  // Not signed in — same lightweight verify, used here as "look up my records".
  if (!account) {
    const lookup = (e: React.FormEvent) => {
      e.preventDefault();
      if (name.trim().length < 2) return setError("Please enter your name.");
      if (!emailOk(email)) return setError("Please enter a valid email.");
      setError("");
      setAccount({ name: name.trim(), email: email.trim().toLowerCase() });
    };

    return (
      <main className="order">
        <div className="order__verify">
          <span className="order__eyebrow">Your records</span>
          <h1 className="order__title">Find my orders</h1>
          <p className="order__lede">
            Enter the name and email you ordered with to see your records.
          </p>

          <form className="order-form" onSubmit={lookup} noValidate>
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
              View my records
            </button>
          </form>

          <a href="/" className="order__back">
             Back to menu
          </a>
        </div>
      </main>
    );
  }

  return (
    <main className="order order--top">
      <div className="account">
        <header className="account__head">
          <div>
            <span className="order__eyebrow">Your records</span>
            <h1 className="order__title">Hi, {account.name.split(" ")[0]}</h1>
            <p className="account__email">{account.email}</p>
          </div>
          <button type="button" className="account__signout" onClick={signOut}>
            Sign out
          </button>
        </header>

        {orders.length === 0 ? (
          <div className="account__empty">
            <p>No orders yet.</p>
            <a
              href="/"
              className="order-form__submit order-form__submit--link"
            >
              Start an order
            </a>
          </div>
        ) : (
          <>
            <p className="account__count">
              {orders.length} order{orders.length > 1 ? "s" : ""} on record
            </p>
            <ul className="account__orders">
              {orders.map((order, i) => {
                const placed = new Date(order.placedAt);
                return (
                  <li key={order.id} className="record">
                    <div className="record__head">
                      <span className="record__id">#{order.id}</span>
                      <span className="record__date">
                        {placed.toLocaleDateString()} ·{" "}
                        {placed.toLocaleTimeString([], {
                          hour: "2-digit",
                          minute: "2-digit",
                        })}
                      </span>
                      <span className="record__total">
                        {naira(order.total)}
                      </span>
                    </div>

                    {/* Live tracker on the most recent order */}
                    {i === 0 && (
                      <div className="record__track">
                        <OrderTracker placedAt={order.placedAt} />
                      </div>
                    )}

                    <ul className="record__lines">
                      {order.lines.map((line) => (
                        <li key={line.id}>
                          <img src={line.image} alt="" />
                          <span>
                            {line.qty}× {line.name}
                          </span>
                          <span className="record__line-sum">
                            {naira(line.price * line.qty)}
                          </span>
                        </li>
                      ))}
                    </ul>

                    <div className="record__foot">
                      <button
                        type="button"
                        className="record__reorder"
                        onClick={() => {
                          reorder(order.lines);
                          router.push("/order");
                        }}
                      >
                        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                          <path
                            d="M4 4v5h.582M4.582 9A8 8 0 1 1 4 13"
                            stroke="currentColor"
                            strokeWidth="2"
                            strokeLinecap="round"
                            strokeLinejoin="round"
                          />
                        </svg>
                        Reorder
                      </button>
                    </div>
                  </li>
                );
              })}
            </ul>
          </>
        )}

        <a href="/" className="order__back">
           Back to menu
        </a>
      </div>
    </main>
  );
}
