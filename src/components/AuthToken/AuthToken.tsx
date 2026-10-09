"use client";

import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { api, errorMessage } from "@/lib/api/client";
import { authErrorMessage, useAuth } from "@/lib/auth";
// @ts-ignore: CSS module import for side effects
import "@/app/order/order.css";
// @ts-ignore: CSS module import for side effects
import "@/components/styles/auth.css";

type Mode = "forgot" | "reset" | "verify";

/** Forgot password, reset password and email verification share one small card. */
export default function AuthToken({ mode }: { mode: Mode }) {
  const token = useSearchParams().get("token") ?? "";
  const { refresh } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);
  const verified = useRef(false);

  // The verification link carries its own token, so it runs on arrival.
  useEffect(() => {
    if (mode !== "verify" || verified.current) return;
    verified.current = true;
    if (!token) return setError("This link is invalid or has expired. Please request a new one.");
    api
      .post("/api/v1/auth/verify-email", { token })
      .then(() => {
        setDone(true);
        refresh();
      })
      .catch((err) => setError(authErrorMessage(err) ?? errorMessage(err)));
  }, [mode, token, refresh]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (mode === "forgot" && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) {
      return setError("Please enter a valid email.");
    }
    if (mode === "reset" && password.length < 8) {
      return setError("Use at least 8 characters for your password.");
    }
    setError("");
    setBusy(true);
    try {
      if (mode === "forgot") await api.post("/api/v1/auth/forgot-password", { email: email.trim() });
      else await api.post("/api/v1/auth/reset-password", { token, password });
      setDone(true);
    } catch (err) {
      setError(authErrorMessage(err) ?? errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const copy = {
    forgot: { eyebrow: "Account", title: "Forgot password", lede: "Enter your email and we'll send you a reset link.", ok: "If an account exists for that email, a reset link is on its way." },
    reset: { eyebrow: "Account", title: "New password", lede: "Choose a new password for your account.", ok: "Your password has been changed. You can sign in now." },
    verify: { eyebrow: "Account", title: "Verify email", lede: "", ok: "Your email is verified. Thank you!" },
  }[mode];

  return (
    <main className="order">
      <div className="order__verify">
        <span className="order__eyebrow">{copy.eyebrow}</span>
        <h1 className="order__title">{copy.title}</h1>

        {done ? (
          <>
            <p className="order__lede">{copy.ok}</p>
            <a href={mode === "reset" ? "/signin" : "/menu"} className="order-form__submit order-form__submit--link">
              {mode === "reset" ? "Sign in" : "Back to menu"}
            </a>
          </>
        ) : mode === "verify" ? (
          <p className={error ? "order-form__error" : "order__lede"} role={error ? "alert" : undefined}>
            {error || "Verifying your email..."}
          </p>
        ) : (
          <>
            <p className="order__lede">{copy.lede}</p>
            <form className="order-form" onSubmit={submit} noValidate>
              {mode === "forgot" ? (
                <label className="order-form__field">
                  <span>Email</span>
                  <input
                    type="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="you@email.com"
                    autoComplete="email"
                  />
                </label>
              ) : (
                <label className="order-form__field">
                  <span>New password</span>
                  <input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="new-password"
                  />
                </label>
              )}
              {error && (
                <p className="order-form__error" role="alert">
                  {error}
                </p>
              )}
              <button type="submit" className="order-form__submit" disabled={busy}>
                {busy ? "Please wait..." : mode === "forgot" ? "Send reset link" : "Change password"}
              </button>
            </form>
            <a href="/signin" className="order__back">
              Back to sign in
            </a>
          </>
        )}
      </div>
    </main>
  );
}
