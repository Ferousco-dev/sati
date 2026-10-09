"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { usePwa } from "@/lib/pwa";
import { useStore } from "@/lib/store";
import { useAuth } from "@/lib/auth";
import Avatar from "@/components/Avatar/Avatar";
import NotificationBell from "@/components/NotificationBell/NotificationBell";
// @ts-ignore: CSS module import for side effects
import "./installed-app-shell.css";

const tabs = [
  { label: "Menu", href: "/menu", icon: "menu" },
  { label: "Cart", href: null, icon: "cart" },
  { label: "Orders", href: "/orders", icon: "orders" },
  { label: "Account", href: "/account", icon: "account" },
] as const;

function TabIcon({ name }: { name: (typeof tabs)[number]["icon"] }) {
  if (name === "menu") return <><path d="M4 7h16M4 12h16M4 17h16" /></>;
  if (name === "cart") return <><path d="M3 4h2l2.1 11.2a2 2 0 0 0 2 1.6h7.8a2 2 0 0 0 1.9-1.4L21 8H6" /><circle cx="9" cy="20" r="1" /><circle cx="17" cy="20" r="1" /></>;
  if (name === "orders") return <><path d="M6 4h12v16H6zM9 8h6M9 12h6M9 16h4" /></>;
  return <><circle cx="12" cy="8" r="3.5" /><path d="M5 20a7 7 0 0 1 14 0" /></>;
}

export default function InstalledAppShell({ children }: { children: React.ReactNode }) {
  const { ready, isStandalone } = usePwa();
  const { count, cartOpen, setCartOpen } = useStore();
  const { user, loading } = useAuth();
  const pathname = usePathname();
  const [splashVisible, setSplashVisible] = useState(true);

  useEffect(() => {
    if (!ready || !isStandalone) return;
    const timer = window.setTimeout(() => setSplashVisible(false), 550);
    return () => window.clearTimeout(timer);
  }, [ready, isStandalone]);

  if (!isStandalone) return <>{children}</>;

  return (
    <div className="installed-app">
      <header className="installed-appbar">
        <Link href="/menu" className="installed-appbar__brand" aria-label="SATI-L menu">
          <span className="installed-appbar__mark" aria-hidden="true">S</span>
          <span>ATI-L</span>
        </Link>
        <div className="installed-appbar__actions">
          {user ? <NotificationBell /> : null}
          {!loading ? (
            <Link
              href={user ? "/account" : "/signin"}
              className="installed-appbar__account"
              aria-label={user ? `Account: ${user.name}` : "Sign in"}
            >
              {user ? (
                <Avatar user={user} className="installed-appbar__avatar" />
              ) : (
                <span>Sign in</span>
              )}
            </Link>
          ) : null}
        </div>
      </header>
      <div className="installed-app__content">{children}</div>
      <nav className="installed-tabs" aria-label="Main navigation">
        {tabs.map((tab) => {
          const active = tab.href ? pathname === tab.href || pathname.startsWith(`${tab.href}/`) : cartOpen;
          const content = (
            <>
              <span className="installed-tabs__icon" aria-hidden="true">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                  <TabIcon name={tab.icon} />
                </svg>
                {tab.icon === "cart" && count > 0 ? <span className="installed-tabs__badge" aria-hidden="true">{count > 99 ? "99+" : count}</span> : null}
              </span>
              <span>{tab.label}</span>
            </>
          );

          return tab.href ? (
            <Link
              key={tab.label}
              href={tab.href}
              className={`installed-tabs__item${active ? " is-active" : ""}`}
              aria-current={active ? "page" : undefined}
            >
              {content}
            </Link>
          ) : (
            <button
              key={tab.label}
              type="button"
              className={`installed-tabs__item${active ? " is-active" : ""}`}
              aria-label={`Cart${count ? `, ${count} items` : ""}`}
              aria-expanded={cartOpen}
              onClick={() => setCartOpen(!cartOpen)}
            >
              {content}
            </button>
          );
        })}
      </nav>
      {splashVisible ? (
        <div className="installed-splash" role="status" aria-label="Opening SATI-L">
          <span className="installed-splash__mark" aria-hidden="true">S</span>
          <span className="installed-splash__name">SATI-L</span>
        </div>
      ) : null}
    </div>
  );
}