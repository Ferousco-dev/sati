"use client";

import { useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useAuth, authErrorMessage } from "@/lib/auth";
import { errorMessage, isApiError } from "@/lib/api/client";
import SupportNotice from "@/components/SupportNotice/SupportNotice";
// @ts-ignore: CSS module import for side effects
import "@/app/order/order.css";
// @ts-ignore: CSS module import for side effects
import "@/components/styles/auth.css";

const emailOk = (v: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v.trim());

/** Only follow same-site paths, so a crafted link can't send people elsewhere. */
function safeNext(value: string | null): string {
  return value && value.startsWith("/") && !value.startsWith("//") ? value : "/menu";
}

export default function AuthForm({ mode }: { mode: "signin" | "register" }) {
  const isRegister = mode === "register";
  const { login, register } = useAuth();
  const router = useRouter();
  const next = safeNext(useSearchParams().get("next"));
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [deleted, setDeleted] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (isRegister && name.trim().length < 2) return setError("Please enter your name.");
    if (!emailOk(email)) return setError("Please enter a valid email.");
    if (isRegister && password.length < 8) return setError("Use at least 8 characters for your password.");
    if (!password) return setError("Please enter your password.");
    setError("");
    setBusy(true);
    try {
      const creds = { email: email.trim(), password, remember_me: remember };
      if (isRegister) await register({ ...creds, name: name.trim(), phone: phone.trim() || undefined });
      else await login(creds);
      router.push(next);
    } catch (err) {
      setDeleted(isApiError(err, "ACCOUNT_DELETED"));
      setError(authErrorMessage(err) ?? errorMessage(err));
      setBusy(false);
    }
  };

  const other = `${isRegister ? "/signin" : "/register"}${next !== "/menu" ? `?next=${encodeURIComponent(next)}` : ""}`;

  return (
    <main className="order">
      <div className="order__verify">
        <span className="order__eyebrow">{isRegister ? "Join us" : "Welcome back"}</span>
        <h1 className="order__title">{isRegister ? "Create account" : "Sign in"}</h1>
        <p className="order__lede">
          {isRegister
            ? "Save your details and follow your orders."
            : "Sign in to see your orders and details."}
        </p>

        <form className="order-form" onSubmit={submit} noValidate>
          {isRegister && (
            <label className="order-form__field">
              <span>Full name</span>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Ada Okeke"
                autoComplete="name"
              />
            </label>
          )}
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
          {isRegister && (
            <label className="order-form__field">
              <span>Phone number (optional)</span>
              <input
                type="tel"
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                placeholder="e.g. 0801 234 5678"
                autoComplete="tel"
              />
            </label>
          )}
          <label className="order-form__field">
            <span>Password</span>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete={isRegister ? "new-password" : "current-password"}
            />
          </label>

          <div className="auth__row">
            <label className="auth__check">
              <input
                type="checkbox"
                checked={remember}
                onChange={(e) => setRemember(e.target.checked)}
              />
              Remember me
            </label>
            {!isRegister && (
              <a href="/forgot-password" className="auth__link">
                Forgot password?
              </a>
            )}
          </div>

          {error && (
            <p className="order-form__error" role="alert">
              {error}
            </p>
          )}

          {deleted && <SupportNotice />}

          <button type="submit" className="order-form__submit" disabled={busy}>
            {busy ? "Please wait..." : isRegister ? "Create account" : "Sign in"}
          </button>
        </form>

        <p className="auth__switch">
          {isRegister ? "Already have an account? " : "New here? "}
          <a href={other} className="auth__link">
            {isRegister ? "Sign in" : "Create an account"}
          </a>
        </p>
        <a href="/menu" className="order__back">
          Back to menu
        </a>
      </div>
    </main>
  );
}
