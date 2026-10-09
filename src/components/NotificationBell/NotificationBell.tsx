"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  getUnreadCount,
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  type AppNotification,
} from "@/lib/api/orders";
// @ts-ignore: CSS module import for side effects
import "./notification-bell.css";

const POLL_MS = 60_000;

function ago(iso: string): string {
  const mins = Math.round((Date.now() - new Date(iso).getTime()) / 60000);
  if (mins < 1) return "Just now";
  if (mins < 60) return `${mins} min ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours} h ago`;
  return new Date(iso).toLocaleDateString();
}

/** The bell in the top bar: an unread count, and the latest updates about the customer's orders. */
export default function NotificationBell() {
  const router = useRouter();
  const ref = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [unread, setUnread] = useState(0);
  const [items, setItems] = useState<AppNotification[] | null>(null);
  const [failed, setFailed] = useState(false);

  const refreshCount = useCallback(() => {
    getUnreadCount().then(setUnread, () => {});
  }, []);

  // A light check once a minute, and again when the customer comes back to the tab.
  useEffect(() => {
    refreshCount();
    const timer = window.setInterval(() => document.visibilityState === "visible" && refreshCount(), POLL_MS);
    window.addEventListener("focus", refreshCount);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener("focus", refreshCount);
    };
  }, [refreshCount]);

  const load = useCallback(() => {
    setFailed(false);
    listNotifications(10).then(
      (r) => {
        setItems(r.notifications);
        setUnread(r.unread_count);
      },
      () => setFailed(true),
    );
  }, []);

  useEffect(() => {
    if (!open) return;
    load();
    const onClick = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, load]);

  const openItem = async (n: AppNotification) => {
    setOpen(false);
    if (!n.read_at) {
      setUnread((c) => Math.max(0, c - 1));
      markNotificationRead(n.id).catch(() => {});
    }
    router.push(n.order_id ? `/orders/${n.order_id}` : "/orders");
  };

  const readAll = async () => {
    setItems((list) => list?.map((n) => ({ ...n, read_at: n.read_at ?? new Date().toISOString() })) ?? list);
    setUnread(0);
    markAllNotificationsRead().catch(load);
  };

  return (
    <div className="bell" ref={ref}>
      <button
        type="button"
        className="bell__button"
        aria-label={unread > 0 ? `Notifications, ${unread} unread` : "Notifications"}
        aria-haspopup="true"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path
            d="M6 9a6 6 0 1 1 12 0c0 5 2 6.5 2 6.5H4S6 14 6 9ZM10 19a2 2 0 0 0 4 0"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
        {unread > 0 && <span className="bell__count">{unread > 9 ? "9+" : unread}</span>}
      </button>

      {open && (
        <div className="bell__panel" role="dialog" aria-label="Notifications">
          <div className="bell__head">
            <strong>Notifications</strong>
            {unread > 0 && (
              <button type="button" className="bell__link" onClick={readAll}>
                Mark all as read
              </button>
            )}
          </div>

          {failed ? (
            <p className="bell__empty">
              Couldn&apos;t load updates.{" "}
              <button type="button" className="bell__link" onClick={load}>
                Try again
              </button>
            </p>
          ) : items === null ? (
            <p className="bell__empty">Loading...</p>
          ) : items.length === 0 ? (
            <p className="bell__empty">Nothing yet. Updates about your orders show up here.</p>
          ) : (
            <ul className="bell__list">
              {items.map((n) => (
                <li key={n.id}>
                  <button type="button" className={`bell__item ${n.read_at ? "" : "is-unread"}`} onClick={() => openItem(n)}>
                    <span className="bell__title">{n.title}</span>
                    <span className="bell__body">{n.body}</span>
                    <span className="bell__time">{ago(n.created_at)}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          <a href="/orders" className="bell__foot" onClick={() => setOpen(false)}>
            View all orders
          </a>
        </div>
      )}
    </div>
  );
}
