"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth";
import Avatar from "@/components/Avatar/Avatar";
import NotificationBell from "@/components/NotificationBell/NotificationBell";
// @ts-ignore: CSS module import for side effects
import "./navbar.css";
// @ts-ignore: CSS module import for side effects
import "@/components/styles/auth.css";
import Img from "@/components/ui/Img";
import { fallbackLogo } from "@/lib/fallbacks";
import type { Navigation as NavigationContent } from "@/lib/content/types";

export default function Navbar({ content }: { content: NavigationContent }) {
  const links = content.links.map((l) => ({ label: l.label, href: l.url }));
  const [open, setOpen] = useState(false);
  const router = useRouter();
  const { user, loading, logout } = useAuth();
  const [menuOpen, setMenuOpen] = useState(false);
  const accountRef = useRef<HTMLDivElement>(null);

  // Close the account menu on an outside click or Escape.
  useEffect(() => {
    if (!menuOpen) return;
    const onClick = (e: MouseEvent) => {
      if (!accountRef.current?.contains(e.target as Node)) setMenuOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setMenuOpen(false);
    document.addEventListener("mousedown", onClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [menuOpen]);

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

          <div className="nav__actions">
            {!loading && !user ? (
              <a href="/signin" className="nav__link" onClick={() => setOpen(false)}>
                Sign in
              </a>
            ) : null}

            {user ? <NotificationBell /> : null}

            {user ? (
              <div className="nav-account" ref={accountRef}>
                <button
                  type="button"
                  className="nav-account__button"
                  aria-haspopup="menu"
                  aria-expanded={menuOpen}
                  onClick={() => setMenuOpen((v) => !v)}
                >
                  <Avatar user={user} className="nav-account__avatar" />
                  <span className="nav-account__name">{user.name.split(" ")[0]}</span>
                </button>
                {menuOpen ? (
                  <div className="nav-account__menu" role="menu">
                    <a href="/account" className="nav-account__item" role="menuitem" onClick={() => setOpen(false)}>
                      Profile
                    </a>
                    <a href="/orders" className="nav-account__item" role="menuitem" onClick={() => setOpen(false)}>
                      My orders
                    </a>
                    <a href="/payments" className="nav-account__item" role="menuitem" onClick={() => setOpen(false)}>
                      My payments
                    </a>
                    <a href="/addresses" className="nav-account__item" role="menuitem" onClick={() => setOpen(false)}>
                      Addresses
                    </a>
                    <button
                      type="button"
                      className="nav-account__item"
                      role="menuitem"
                      onClick={async () => {
                        setOpen(false);
                        setMenuOpen(false);
                        await logout();
                        router.push("/");
                      }}
                    >
                      Sign out
                    </button>
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>

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
