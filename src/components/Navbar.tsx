"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useStore } from "@/lib/store";
// @ts-ignore: CSS module import for side effects
import "./navbar.css";
import Img from "@/components/ui/Img";
import { fallbackLogo } from "@/lib/fallbacks";
import type { Navigation as NavigationContent } from "@/lib/content/types";

export default function Navbar({ content }: { content: NavigationContent }) {
  const links = content.links.map((l) => ({ label: l.label, href: l.url }));
  const [open, setOpen] = useState(false);
  const { count, setCartOpen } = useStore();
  const router = useRouter();

  return (
    <nav className="nav">
      <div className="nav__container">
        <a href="/" className="nav__logo" aria-label={`${content.logo_alt} home`}>
          <Img src={content.logo?.url ?? fallbackLogo} alt={content.logo_alt} />
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
          onClick={() => (count > 0 ? setCartOpen(true) : router.push("/menu"))}
          data-magnetic
        >
          {count > 0 ? `${content.cart_label} (${count})` : content.order_label}
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
