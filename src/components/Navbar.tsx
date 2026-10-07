"use client";

import { useState } from "react";
import { useStore } from "@/lib/store";
// @ts-ignore: CSS module import for side effects
import "./navbar.css";

const links = [
  { label: "Our Brand", href: "#brand" },
  { label: "Stores", href: "#stores" },
  { label: "Our Menu", href: "#menu" },
  { label: "My Orders", href: "/account" },
];

export default function Navbar() {
  const [open, setOpen] = useState(false);
  const { count, setCartOpen } = useStore();

  return (
    <nav className="nav">
      <div className="nav__container">
        <a href="/" className="nav__logo" aria-label="SATI-L Yoghurt & confectionery  home">
          <img src="/images/logo.png" alt="SATI-L Yoghurt & confectionery " />
        </a>

        <div className={`nav__menu ${open ? "is-open" : ""}`}>
          {links.map((link) => (
            <a
              key={link.href}
              href={link.href}
              className="nav__link"
              onClick={() => setOpen(false)}
            >
              {link.label}
            </a>
          ))}
        </div>

        <button
          type="button"
          className="nav__order"
          onClick={() => setCartOpen(true)}
          data-magnetic
        >
          {count > 0 ? `Cart (${count})` : "Order Now"}
        </button>

        <button
          type="button"
          className={`nav__toggle ${open ? "is-open" : ""}`}
          aria-label="Toggle menu"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
        >
          <span className="nav__toggle-line is-top" />
          <span className="nav__toggle-line is-middle" />
          <span className="nav__toggle-line is-btm" />
        </button>
      </div>
    </nav>
  );
}
