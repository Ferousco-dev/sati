"use client";

import { useEffect, useRef } from "react";
import { gsap } from "gsap";
import type { Order } from "@/lib/store";
import { naira } from "@/lib/menu";
import OrderTracker from "./OrderTracker";
// @ts-ignore: CSS module import for side effects
import "./order-success.css";

type Props = {
  order: Order;
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

  const placed = new Date(order.placedAt);

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
        Thanks{name ? `, ${name.split(" ")[0]}` : ""}  your food is on the
        grill. We&apos;ll have it ready soon.
      </p>

      <div className="os-receipt os-reveal">
        <div className="os-receipt__head">
          <span>Order</span>
          <span className="os-receipt__id">#{order.id}</span>
        </div>
        <ul className="os-receipt__lines">
          {order.lines.map((line) => (
            <li key={line.id}>
              <span>
                {line.qty}× {line.name}
              </span>
              <span>{naira(line.price * line.qty)}</span>
            </li>
          ))}
        </ul>
        <div className="os-receipt__total">
          <span>Total</span>
          <span>{naira(order.total)}</span>
        </div>
        <p className="os-receipt__time">
          {placed.toLocaleDateString()} ·{" "}
          {placed.toLocaleTimeString([], {
            hour: "2-digit",
            minute: "2-digit",
          })}
        </p>
      </div>

      <div className="os-track os-reveal">
        <OrderTracker placedAt={order.placedAt} />
      </div>

      <div className="os-actions os-reveal">
        <a href="/account" className="os-btn os-btn--solid">
          View my orders
        </a>
        <a href="/" className="os-btn">
          Back to home
        </a>
      </div>
    </div>
  );
}
