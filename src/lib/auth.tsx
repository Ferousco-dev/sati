"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { api, isApiError } from "./api/client";
import type { User } from "./api/types";

type Credentials = { email: string; password: string; remember_me: boolean };

export const AVATAR_MAX_BYTES = 2 * 1024 * 1024;
export const AVATAR_TYPES = ["image/jpeg", "image/png", "image/webp"];

type AuthCtx = {
  user: User | null;
  /** True until the first /auth/me answer, so pages don't flash the signed-out state. */
  loading: boolean;
  login: (input: Credentials) => Promise<User>;
  register: (input: Credentials & { name: string; phone?: string }) => Promise<User>;
  logout: () => Promise<void>;
  resendVerification: () => Promise<void>;
  updateProfile: (input: { name?: string; phone?: string }) => Promise<void>;
  uploadAvatar: (file: File) => Promise<void>;
  removeAvatar: () => Promise<void>;
  /** Closes the account. It is kept, not erased, and only an administrator can bring it back. */
  deleteAccount: (password: string) => Promise<void>;
  refresh: () => Promise<void>;
};

const AuthContext = createContext<AuthCtx | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const res = await api.get<{ user: User }>("/api/v1/auth/me");
      setUser(res.user);
    } catch (err) {
      // 401 just means signed out. Any other failure also leaves the visitor browsing.
      if (!isApiError(err)) throw err;
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh().catch(() => setLoading(false));
  }, [refresh]);

  const value = useMemo<AuthCtx>(
    () => ({
      user,
      loading,
      refresh,
      login: async (input) => {
        const res = await api.post<{ user: User }>("/api/v1/auth/login", input);
        setUser(res.user);
        return res.user;
      },
      register: async (input) => {
        const res = await api.post<{ user: User }>("/api/v1/auth/register", input);
        setUser(res.user);
        return res.user;
      },
      logout: async () => {
        try {
          await api.post("/api/v1/auth/logout");
        } finally {
          setUser(null);
        }
      },
      resendVerification: async () => {
        await api.post("/api/v1/auth/resend-verification");
      },
      updateProfile: async (input) => {
        const res = await api<{ user: User }>("/api/v1/auth/me", { method: "PATCH", body: input });
        setUser(res.user);
      },
      uploadAvatar: async (file) => {
        const form = new FormData();
        form.append("image", file);
        const res = await api.post<{ user: User }>("/api/v1/auth/me/avatar", form);
        setUser(res.user);
      },
      removeAvatar: async () => {
        const res = await api<{ user: User }>("/api/v1/auth/me/avatar", { method: "DELETE" });
        setUser(res.user);
      },
      deleteAccount: async (password) => {
        await api("/api/v1/auth/me", { method: "DELETE", body: { password } });
        setUser(null);
      },
    }),
    [user, loading, refresh],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}

/** Friendly copy for the auth error codes the customer can hit. */
export function authErrorMessage(err: unknown): string | null {
  if (isApiError(err, "EMAIL_TAKEN")) return "An account with this email already exists. Try signing in.";
  if (isApiError(err, "INVALID_CREDENTIALS")) return "Email or password is incorrect.";
  if (isApiError(err, "INVALID_PASSWORD")) return "That password is not correct.";
  if (isApiError(err, "ACCOUNT_DELETED")) return "This account was deleted. To use it again, please email our support team.";
  if (isApiError(err, "INVALID_TOKEN")) return "This link is invalid or has expired. Please request a new one.";
  return null;
}
