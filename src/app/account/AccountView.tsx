"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth, authErrorMessage, AVATAR_MAX_BYTES, AVATAR_TYPES } from "@/lib/auth";
import { errorMessage } from "@/lib/api/client";
import { useToast } from "@/components/ui/Toast";
import Avatar from "@/components/Avatar/Avatar";
import Modal from "@/components/ui/Modal";
import { listOrders, type ApiOrderFull } from "@/lib/api/orders";
import { formatKobo } from "@/lib/money";
import { paymentLabel, paymentTone, statusLabel, statusTone } from "@/lib/order-status";
import { useReorder } from "@/lib/reorder";
import StatusBadge from "@/components/StatusBadge/StatusBadge";
// @ts-ignore: CSS module import for side effects
import "../order/order.css";
// @ts-ignore: CSS module import for side effects
import "./account.css";
// @ts-ignore: CSS module import for side effects
import "@/components/styles/auth.css";
// @ts-ignore: CSS module import for side effects
import "../orders/orders.css";

export default function AccountView() {
  const reorder = useReorder();
  const [recent, setRecent] = useState<ApiOrderFull[] | null>(null);
  const [orderTotal, setOrderTotal] = useState(0);
  const { user, loading, logout, resendVerification, updateProfile, uploadAvatar, removeAvatar, deleteAccount } = useAuth();
  const router = useRouter();
  const toast = useToast();
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!user) return;
    listOrders(3, 0).then(
      (page) => {
        setRecent(page.orders);
        setOrderTotal(page.total);
      },
      () => setRecent([]),
    );
  }, [user]);
  const [photoBusy, setPhotoBusy] = useState(false);
  const [photoError, setPhotoError] = useState("");
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({ name: "", phone: "" });
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [password, setPassword] = useState("");
  const [deleteError, setDeleteError] = useState("");
  const [deleting, setDeleting] = useState(false);

  if (loading) return <main className="account-page" />;

  // Not signed in: send people to sign in, then bring them back here.
  if (!user) {
    return (
      <main className="account-page account-page--center">
        <div className="account-card account-card--narrow">
          <span className="order__eyebrow">Your account</span>
          <h1 className="order__title">Sign in</h1>
          <p className="order__lede">Sign in to see your profile and orders.</p>
          <a
            href="/signin?next=/account"
            className="order-form__submit order-form__submit--link"
          >
            Sign in
          </a>
          <p className="auth__switch">
            New here?{" "}
            <a href="/register?next=/account" className="auth__link">
              Create an account
            </a>
          </p>
        </div>
      </main>
    );
  }

  const pickPhoto = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    if (!AVATAR_TYPES.includes(file.type)) return setPhotoError("Please choose a JPG, PNG or WebP picture.");
    if (file.size > AVATAR_MAX_BYTES) return setPhotoError("The picture must be 2 MB or smaller.");
    setPhotoError("");
    setPhotoBusy(true);
    try {
      await uploadAvatar(file);
    } catch (err) {
      setPhotoError(errorMessage(err));
    } finally {
      setPhotoBusy(false);
    }
  };

  const dropPhoto = async () => {
    setPhotoError("");
    setPhotoBusy(true);
    try {
      await removeAvatar();
    } catch (err) {
      setPhotoError(errorMessage(err));
    } finally {
      setPhotoBusy(false);
    }
  };

  const startEdit = () => {
    setForm({ name: user.name, phone: user.phone });
    setFormError("");
    setEditing(true);
  };

  const saveDetails = async (e: React.FormEvent) => {
    e.preventDefault();
    if (form.name.trim().length < 1) return setFormError("Please enter your name.");
    setFormError("");
    setSaving(true);
    try {
      await updateProfile({ name: form.name.trim(), phone: form.phone.trim() });
      setEditing(false);
      toast("Your details were saved.", { tone: "success" });
    } catch (err) {
      setFormError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const closeConfirm = () => {
    setConfirmOpen(false);
    setPassword("");
    setDeleteError("");
  };

  const confirmDelete = async () => {
    setDeleteError("");
    setDeleting(true);
    try {
      await deleteAccount(password);
      toast("Your account has been deleted.", { tone: "info" });
      router.push("/");
    } catch (err) {
      setDeleteError(authErrorMessage(err) ?? errorMessage(err));
      setDeleting(false);
    }
  };

  const since = new Date(user.created_at).toLocaleDateString(undefined, {
    month: "long",
    year: "numeric",
  });

  return (
    <main className="account-page">
      <div className="account-wrap">
        <header className="account-intro">
          <span className="order__eyebrow">Your account</span>
          <h1 className="order__title">Hi, {user.name.split(" ")[0]}</h1>
        </header>

        <div className="account-layout">
          <aside className="account-card account-profile">
            <div className="account-photo">
              <Avatar user={user} className="account-profile__avatar" />
              <button
                type="button"
                className="account-photo__edit"
                aria-label="Change profile photo"
                disabled={photoBusy}
                onClick={() => fileRef.current?.click()}
              >
                <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path
                    d="M4 8h3l1.5-2h7L17 8h3v11H4V8Z M12 16.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z"
                    stroke="currentColor"
                    strokeWidth="1.8"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  />
                </svg>
              </button>
              <input
                ref={fileRef}
                type="file"
                accept={AVATAR_TYPES.join(",")}
                className="account-photo__input"
                onChange={pickPhoto}
                tabIndex={-1}
              />
            </div>
            {photoBusy ? (
              <p className="account-profile__hint" role="status">Uploading...</p>
            ) : user.avatar_url ? (
              <button type="button" className="account-profile__link account-profile__link--quiet" onClick={dropPhoto}>
                Remove photo
              </button>
            ) : (
              <p className="account-profile__hint">JPG, PNG or WebP, up to 2 MB</p>
            )}
            {photoError && (
              <p className="order-form__error" role="alert">
                {photoError}
              </p>
            )}

            {editing ? (
              <form className="order-form account-edit" onSubmit={saveDetails} noValidate>
                <label className="order-form__field">
                  <span>Full name</span>
                  <input
                    value={form.name}
                    onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                    autoComplete="name"
                  />
                </label>
                <label className="order-form__field">
                  <span>Phone number (optional)</span>
                  <input
                    type="tel"
                    value={form.phone}
                    onChange={(e) => setForm((f) => ({ ...f, phone: e.target.value }))}
                    placeholder="e.g. 0801 234 5678"
                    autoComplete="tel"
                  />
                </label>
                {formError && (
                  <p className="order-form__error" role="alert">
                    {formError}
                  </p>
                )}
                <button type="submit" className="order-form__submit" disabled={saving}>
                  {saving ? "Saving..." : "Save details"}
                </button>
                <button type="button" className="order__switch" onClick={() => setEditing(false)}>
                  Cancel
                </button>
              </form>
            ) : (
              <>
                <h2 className="account-profile__name">{user.name}</h2>
                <p className="account-profile__email">{user.email}</p>
              </>
            )}

            <span
              className={`account-badge ${user.email_verified ? "is-ok" : "is-pending"}`}
            >
              {user.email_verified ? "Email verified" : "Email not verified"}
            </span>
            {!user.email_verified && (
              <button
                type="button"
                className="account-profile__link"
                onClick={() => resendVerification().catch(() => {})}
              >
                Resend verification email
              </button>
            )}

            <dl className="account-profile__meta">
              <div className="account-profile__wide">
                <dt>Phone</dt>
                <dd>{user.phone || "Not added"}</dd>
              </div>
              <div>
                <dt>Member since</dt>
                <dd>{since}</dd>
              </div>
              <div>
                <dt>Orders</dt>
                <dd>{orderTotal}</dd>
              </div>
            </dl>

            {!editing && (
              <button type="button" className="account__edit" onClick={startEdit}>
                Edit details
              </button>
            )}
            <nav className="account-links" aria-label="Your account">
              <a href="/orders">My orders</a>
              <a href="/payments">My payments</a>
              <a href="/addresses">Addresses</a>
            </nav>
            <button
              type="button"
              className="account__signout"
              onClick={async () => {
                await logout();
                router.push("/");
              }}
            >
              Sign out
            </button>
          </aside>

          <section className="account-orders" aria-labelledby="orders-title">
            <div className="account-orders__bar">
              <h2 id="orders-title" className="account-orders__title">
                Recent orders
              </h2>
              {orderTotal > 0 && (
                <a href="/orders" className="auth__link">
                  View all
                </a>
              )}
            </div>

            {recent === null ? (
              <div className="account-card account__empty" aria-hidden="true">
                <p>Loading...</p>
              </div>
            ) : recent.length === 0 ? (
              <div className="account-card account__empty">
                <p>No orders yet.</p>
                <a href="/menu" className="order-form__submit order-form__submit--link">
                  Start an order
                </a>
              </div>
            ) : (
              <ul className="account__orders">
                {recent.map((order) => {
                  const placed = new Date(order.created_at);
                  return (
                    <li key={order.id} className="record">
                      <div className="record__head">
                        <span className="record__id">#{order.order_number}</span>
                        <span className="record__date">{placed.toLocaleDateString()}</span>
                        <span className="record__total">{formatKobo(order.total_kobo)}</span>
                      </div>
                      <div className="orders-badges">
                        <StatusBadge label={statusLabel(order.status)} tone={statusTone(order.status)} />
                        <StatusBadge label={paymentLabel(order.payment_status)} tone={paymentTone(order.payment_status)} />
                      </div>
                      <ul className="record__lines">
                        {order.items.map((line) => (
                          <li key={line.product_id}>
                            <span>
                              {line.quantity}× {line.name}
                            </span>
                          </li>
                        ))}
                      </ul>
                      <div className="record__foot orders-foot">
                        <a href={`/orders/${order.id}`} className="record__reorder">
                          View details
                        </a>
                        <button type="button" className="record__reorder" onClick={() => reorder(order)}>
                          Reorder
                        </button>
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}

            <section className="account-card account-danger" aria-labelledby="danger-title">
              <h2 id="danger-title" className="account-danger__title">Delete account</h2>
              <p className="account-danger__text">
                You&apos;ll be signed out everywhere and won&apos;t be able to sign in or register with this
                email again. Your order records are kept for our accounts. To use the account again you
                would need to email our support team.
              </p>
              <button type="button" className="account-danger__button" onClick={() => setConfirmOpen(true)}>
                Delete my account
              </button>
            </section>
          </section>
        </div>
      </div>

      <Modal
        open={confirmOpen}
        title="Delete your account?"
        onClose={closeConfirm}
        actions={
          <button
            type="button"
            className="order-form__submit order-form__submit--danger"
            disabled={!password || deleting}
            onClick={confirmDelete}
          >
            {deleting ? "Deleting..." : "Delete my account"}
          </button>
        }
      >
        <p>
          This signs you out everywhere and closes <strong>{user.email}</strong> for good. You won&apos;t be
          able to sign in or register with it again unless you email our support team.
        </p>
        <label className="order-form__field">
          <span>Type your password to confirm</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
        </label>
        {deleteError && (
          <p className="order-form__error" role="alert">
            {deleteError}
          </p>
        )}
      </Modal>
    </main>
  );
}
