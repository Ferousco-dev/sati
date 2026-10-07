"use client";

import { createContext, useCallback, useContext, useMemo, useRef, useState } from "react";
// @ts-ignore: CSS module import for side effects
import "./ui.css";

type Tone = "info" | "success" | "error";
type ToastItem = { id: number; tone: Tone; title?: string; message: string };
type Show = (message: string, options?: { title?: string; tone?: Tone; duration?: number }) => void;

const Ctx = createContext<Show | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const next = useRef(1);

  const dismiss = useCallback((id: number) => {
    setItems((list) => list.filter((t) => t.id !== id));
  }, []);

  const show = useCallback<Show>(
    (message, options = {}) => {
      const id = next.current++;
      const tone = options.tone ?? "info";
      setItems((list) => [...list.slice(-3), { id, tone, title: options.title, message }]);
      window.setTimeout(() => dismiss(id), options.duration ?? (tone === "error" ? 8000 : 5000));
    },
    [dismiss],
  );

  const value = useMemo(() => show, [show]);

  return (
    <Ctx.Provider value={value}>
      {children}
      <div className="ui-toasts" role="region" aria-live="polite" aria-label="Notifications">
        {items.map((t) => (
          <div key={t.id} className={`ui-toast is-${t.tone}`} role={t.tone === "error" ? "alert" : "status"}>
            <p className="ui-toast__text">
              {t.title ? <span className="ui-toast__title">{t.title}</span> : null}
              {t.message}
            </p>
            <button type="button" className="ui-toast__close" aria-label="Dismiss" onClick={() => dismiss(t.id)}>
              &times;
            </button>
          </div>
        ))}
      </div>
    </Ctx.Provider>
  );
}

export function useToast(): Show {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}
