"use client";

import { useEffect, useRef } from "react";
// @ts-ignore: CSS module import for side effects
import "./ui.css";

type Props = {
  open: boolean;
  title: string;
  onClose: () => void;
  children: React.ReactNode;
  actions?: React.ReactNode;
};

export default function Modal({ open, title, onClose, children, actions }: Props) {
  const panel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    panel.current?.querySelector<HTMLElement>("button, a, input")?.focus();

    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key !== "Tab" || !panel.current) return;
      // Keep keyboard focus inside the dialog.
      const items = panel.current.querySelectorAll<HTMLElement>(
        'button, a[href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
      );
      if (!items.length) return;
      const first = items[0];
      const last = items[items.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = overflow;
      previous?.focus();
    };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="ui-modal"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        className="ui-modal__panel"
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <button type="button" className="ui-modal__close" aria-label="Close" onClick={onClose}>
          &times;
        </button>
        <h2 className="ui-modal__title">{title}</h2>
        <div className="ui-modal__body">{children}</div>
        {actions ? <div className="ui-modal__actions">{actions}</div> : null}
      </div>
    </div>
  );
}
