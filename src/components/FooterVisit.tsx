"use client";

import { useEffect, useState } from "react";
import { dayName, formatClock, hasAnyHours, openStatus, WEEK_ORDER, type OpenStatus } from "@/lib/hours";
import { formatKobo } from "@/lib/money";
import type { StoreInfo, Zone } from "@/lib/content/types";
// @ts-ignore: CSS module import for side effects
import "./footer-visit.css";

const SHORT = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"] as const;

function todayKey(timezone: string): string {
  const name = new Intl.DateTimeFormat("en-US", { timeZone: timezone, weekday: "short" }).format(new Date());
  return SHORT[["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].indexOf(name)];
}

function Panel({ title, children, open = false }: { title: string; children: React.ReactNode; open?: boolean }) {
  return (
    <details className="fv__panel" open={open}>
      <summary className="fv__summary">
        {title}
        <svg className="fv__icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path d="M12 5v14M5 12h14" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
        </svg>
      </summary>
      <div className="fv__body">{children}</div>
    </details>
  );
}

/** Opening hours, delivery areas and contact details, folded into the footer. */
export default function FooterVisit({ store, zones }: { store: StoreInfo; zones: Zone[] }) {
  const [status, setStatus] = useState<OpenStatus | null>(null);
  const [today, setToday] = useState("");

  // Worked out in the browser so it is right when read, whatever the page cache holds.
  useEffect(() => {
    const update = () => {
      setStatus(openStatus(store.opening_hours, store.timezone, store.temporarily_closed));
      setToday(todayKey(store.timezone));
    };
    update();
    const id = window.setInterval(update, 60_000);
    return () => window.clearInterval(id);
  }, [store.opening_hours, store.timezone, store.temporarily_closed]);

  const showHours = hasAnyHours(store.opening_hours) || store.temporarily_closed;
  const hasContact = Boolean(store.address || store.phone || store.email || store.whatsapp_url);
  if (!showHours && !zones.length && !hasContact) return null;

  const directions = store.address
    ? `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(store.address)}`
    : "";

  return (
    <section className="fv" id="visit" aria-label="Come by or order in">
      <h2 className="fv__title">Come by or order in</h2>
      <div className="fv__panels">
        {showHours ? (
          <Panel title="Opening hours" open>
            {status ? (
              <p className={`fv__status ${status.open ? "is-open" : ""}`}>
                <span className="fv__dot" aria-hidden="true" />
                {status.label}
              </p>
            ) : null}
            <ul className="fv__rows">
              {WEEK_ORDER.map((day) => {
                const hours = store.opening_hours[day];
                return (
                  <li className={`fv__row ${today === day ? "is-today" : ""}`} key={day}>
                    <span>{dayName(day)}</span>
                    {hours ? (
                      <span>
                        {formatClock(hours.open)} – {formatClock(hours.close)}
                      </span>
                    ) : (
                      <span className="is-closed">Closed</span>
                    )}
                  </li>
                );
              })}
            </ul>
            {store.temporarily_closed && store.closed_message ? <p className="fv__note">{store.closed_message}</p> : null}
          </Panel>
        ) : null}

        {zones.length ? (
          <Panel title="Delivery areas">
            <ul className="fv__rows">
              {zones.map((zone) => (
                <li className="fv__row" key={zone.id}>
                  <span>{zone.name}</span>
                  <span>{zone.fee_kobo === 0 ? "Free" : formatKobo(zone.fee_kobo)}</span>
                </li>
              ))}
            </ul>
            <p className="fv__note">Pick your area at checkout. The fee is added to your order.</p>
          </Panel>
        ) : null}

        {hasContact ? (
          <Panel title="Find us">
            <ul className="fv__rows is-stacked">
              {store.address ? <li>{store.address}</li> : null}
              {store.phone ? (
                <li>
                  <a href={`tel:${store.phone.replace(/\s+/g, "")}`}>{store.phone}</a>
                </li>
              ) : null}
              {store.email ? (
                <li>
                  <a href={`mailto:${store.email}`}>{store.email}</a>
                </li>
              ) : null}
              {store.whatsapp_url ? (
                <li>
                  <a href={store.whatsapp_url} target="_blank" rel="noopener noreferrer">
                    Chat on WhatsApp
                  </a>
                </li>
              ) : null}
              {directions ? (
                <li>
                  <a href={directions} target="_blank" rel="noopener noreferrer">
                    Get directions
                  </a>
                </li>
              ) : null}
            </ul>
          </Panel>
        ) : null}
      </div>
    </section>
  );
}
