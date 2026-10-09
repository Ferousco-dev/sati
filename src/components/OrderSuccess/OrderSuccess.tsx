"use client";

import { useEffect, useRef } from "react";
import { gsap } from "gsap";
import type { ApiOrder } from "@/lib/api/orders";
import { formatKobo } from "@/lib/money";
// @ts-ignore: CSS module import for side effects
import "./order-success.css";

type Props = {
  order: ApiOrder;
  name: string;
};

// Confirmation screen with a GSAP "order placed" animation,
// inspired by the order-button morph clip: the button fills,
// punches out, a checkmark draws in, then the receipt slides up.
export default function OrderSuccess({ order, name }: Props) {
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const ctx = gsap.context(() => {
      const tl = gsap.timeline({ defaults: { ease: "power3.out" } });

      tl.set(".os-check__path", { strokeDasharray: 48, strokeDashoffset: 48 })
        // button fills up like a progress bar
        .fromTo(
          ".os-fill",
          { scaleX: 0 },
          { scaleX: 1, duration: 0.7, ease: "power2.inOut" },
        )
        // the pill pops into the circular badge
        .to(".os-badge", {
          width: "5.5rem",
          borderRadius: "50%",
          duration: 0.5,
          ease: "back.out(1.6)",
        })
        .to(".os-badge__label", { opacity: 0, duration: 0.2 }, "<")
        // checkmark draws on
        .to(".os-check", { opacity: 1, duration: 0.1 }, "-=0.1")
        .to(".os-check__path", {
          strokeDashoffset: 0,
          duration: 0.45,
          ease: "power2.out",
        })
        // a little celebratory pop
        .fromTo(
          ".os-badge",
          { scale: 1 },
          { scale: 1.12, duration: 0.18, yoyo: true, repeat: 1 },
        )
        // headline + receipt reveal
        .from(
          ".os-reveal",
          { y: 28, opacity: 0, duration: 0.7, stagger: 0.1 },
          "-=0.1",
        )
        // confetti burst
        .fromTo(
          ".os-confetti span",
          { y: 0, x: 0, opacity: 1, scale: 0 },
          {
            scale: 1,
            opacity: 0,
            duration: 1.1,
            ease: "power2.out",
            stagger: { each: 0.02, from: "center" },
            x: () => gsap.utils.random(-160, 160),
            y: () => gsap.utils.random(-200, -40),
            rotation: () => gsap.utils.random(-180, 180),
          },
          "-=0.6",
        );
    }, root);

    return () => ctx.revert();
  }, []);

  const placed = new Date(order.created_at);
  const pay = order.payment_instructions;

  return (
    <div className="os" ref={root}>
      <div className="os-confetti" aria-hidden="true">
        {Array.from({ length: 18 }).map((_, i) => (
          <span key={i} data-c={i % 3} />
        ))}
      </div>

      <div className="os-badge">
        <span className="os-badge__label">Placing…</span>
        <span className="os-fill" />
        <svg className="os-check" viewBox="0 0 24 24" aria-hidden="true">
          <path
            className="os-check__path"
            d="M5 13l4 4L19 7"
            fill="none"
            stroke="currentColor"
            strokeWidth="2.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </div>

      <h1 className="os-title os-reveal">Order placed!</h1>
      <p className="os-sub os-reveal">
        Thanks{name ? `, ${name.split(" ")[0]}` : ""}.{" "}
        {order.payment_method === "BANK_TRANSFER"
          ? "We'll start on it as soon as your transfer is confirmed."
          : order.payment_method === "PAY_ON_DELIVERY"
            ? "Please have the payment ready when it arrives."
            : order.payment_method === "PAY_ON_PICKUP"
              ? "Pay when you pick it up. We'll have it ready soon."
              : "We'll have it ready soon."}
      </p>

      {pay && order.payment_method === "BANK_TRANSFER" ? (
        <div className="os-receipt os-reveal">
          <div className="os-receipt__head">
            <span>Pay by bank transfer</span>
          </div>
          <ul className="os-receipt__lines">
            <li><span>Bank</span><span>{pay.bank_name}</span></li>
            <li><span>Account number</span><span>{pay.account_number}</span></li>
            <li><span>Account name</span><span>{pay.account_name}</span></li>
            <li><span>Reference</span><span>{pay.reference}</span></li>
            <li><span>Amount</span><span>{formatKobo(pay.amount_kobo)}</span></li>
          </ul>
          {pay.whatsapp_url ? (
            <p className="os-receipt__time">
              <a href={pay.whatsapp_url} target="_blank" rel="noopener noreferrer">
                Send your proof of payment on WhatsApp
              </a>
            </p>
          ) : null}
        </div>
      ) : null}

      <div className="os-receipt os-reveal">
        <div className="os-receipt__head">
          <span>Order</span>
          <span className="os-receipt__id">#{order.order_number}</span>
        </div>
        <ul className="os-receipt__lines">
          {order.items.map((line) => (
            <li key={line.product_id}>
              <span>
                {line.quantity}× {line.name}
              </span>
              <span>{formatKobo(line.line_total_kobo)}</span>
            </li>
          ))}
        </ul>
        {order.delivery_fee_kobo > 0 ? (
          <div className="os-receipt__total">
            <span>Delivery</span>
            <span>{formatKobo(order.delivery_fee_kobo)}</span>
          </div>
        ) : null}
        <div className="os-receipt__total">
          <span>Total</span>
          <span>{formatKobo(order.total_kobo)}</span>
        </div>
        <p className="os-receipt__time">
          {placed.toLocaleDateString()} ·{" "}
          {placed.toLocaleTimeString([], {
            hour: "2-digit",
            minute: "2-digit",
          })}
        </p>
      </div>

      <div className="os-actions os-reveal">
        <a href="/orders" className="os-btn os-btn--solid">
          View my orders
        </a>
        <a href="/" className="os-btn">
          Back to home
        </a>
      </div>
    </div>
  );
}
