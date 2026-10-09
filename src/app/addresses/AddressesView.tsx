"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth";
import { errorMessage } from "@/lib/api/client";
import {
  deleteAddress,
  getCheckoutOptions,
  listAddresses,
  type Address,
  type Zone,
} from "@/lib/api/orders";
import { useToast } from "@/components/ui/Toast";
import Modal from "@/components/ui/Modal";
import EmptyState from "@/components/ui/EmptyState";
import AddressForm from "../order/AddressForm";
// @ts-ignore: CSS module import for side effects
import "../order/order.css";
// @ts-ignore: CSS module import for side effects
import "../order/checkout.css";
// @ts-ignore: CSS module import for side effects
import "../account/account.css";
// @ts-ignore: CSS module import for side effects
import "../orders/orders.css";
// @ts-ignore: CSS module import for side effects
import "./addresses.css";

export default function AddressesView() {
  const { user, loading } = useAuth();
  const router = useRouter();
  const toast = useToast();
  const [addresses, setAddresses] = useState<Address[] | null>(null);
  const [zones, setZones] = useState<Zone[]>([]);
  const [error, setError] = useState("");
  // "new" shows the add form, an address shows its edit form, null shows neither.
  const [editing, setEditing] = useState<Address | "new" | null>(null);
  const [removing, setRemoving] = useState<Address | null>(null);
  const [removeError, setRemoveError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!loading && !user) router.replace("/signin?next=/addresses");
  }, [loading, user, router]);

  const load = useCallback(() => {
    setError("");
    listAddresses().then(setAddresses, (err) => setError(errorMessage(err)));
    getCheckoutOptions().then((o) => setZones(o.delivery_zones.filter((z) => z.active)), () => {});
  }, []);

  useEffect(() => {
    if (user) load();
  }, [user, load]);

  const confirmRemove = async () => {
    if (!removing) return;
    setBusy(true);
    setRemoveError("");
    try {
      await deleteAddress(removing.id);
      setAddresses((list) => list?.filter((a) => a.id !== removing.id) ?? list);
      setRemoving(null);
      toast("Address removed.");
    } catch (err) {
      setRemoveError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  if (loading || !user) return <main className="account-page" />;

  return (
    <main className="account-page">
      <div className="account-wrap account-wrap--narrow">
        <header className="account-intro">
          <span className="order__eyebrow">Your account</span>
          <h1 className="order__title">My addresses</h1>
        </header>

        {error && (
          <EmptyState
            title="We couldn't load your addresses"
            text={error}
            action={
              <button type="button" className="order-form__submit" onClick={load}>
                Try again
              </button>
            }
          />
        )}

        {editing && (
          <section className="account-card od-card">
            <h2 className="od-card__title">{editing === "new" ? "Add an address" : "Edit address"}</h2>
            <AddressForm
              zones={zones}
              defaultName={user.name}
              initial={editing === "new" ? undefined : editing}
              zoneRequired={false}
              onCancel={() => setEditing(null)}
              onSaved={(saved) => {
                setAddresses((list) => {
                  const rest = (list ?? []).filter((a) => a.id !== saved.id);
                  return editing === "new" ? [saved, ...rest] : (list ?? []).map((a) => (a.id === saved.id ? saved : a));
                });
                setEditing(null);
                toast("Address saved.", { tone: "success" });
              }}
            />
          </section>
        )}

        {addresses && addresses.length === 0 && !editing && (
          <div className="account-card account__empty">
            <p>No saved addresses yet.</p>
            <button type="button" className="order-form__submit order-form__submit--link" onClick={() => setEditing("new")}>
              Add an address
            </button>
          </div>
        )}

        {addresses && addresses.length > 0 && (
          <>
            <ul className="account__orders">
              {addresses.map((a) => (
                <li key={a.id} className="record addr-row">
                  <div className="addr-row__body">
                    <strong>{a.recipient_name}</strong>
                    <span>
                      {a.address_line}
                      {a.area ? `, ${a.area}` : ""}
                      {a.city ? `, ${a.city}` : ""}
                    </span>
                    <span className="addr-row__meta">
                      {a.phone}
                      {a.zone_name ? ` · ${a.zone_name}` : ""}
                    </span>
                    {a.instructions && <span className="addr-row__meta">{a.instructions}</span>}
                  </div>
                  <div className="addr-row__actions">
                    <button type="button" className="record__reorder" onClick={() => setEditing(a)}>
                      Edit
                    </button>
                    <button type="button" className="od-button--quiet" onClick={() => setRemoving(a)}>
                      Remove
                    </button>
                  </div>
                </li>
              ))}
            </ul>
            {!editing && (
              <button type="button" className="order-form__submit addr-add" onClick={() => setEditing("new")}>
                Add an address
              </button>
            )}
          </>
        )}
      </div>

      <Modal
        open={removing !== null}
        title="Remove this address?"
        onClose={() => setRemoving(null)}
        actions={
          <>
            <button type="button" className="order-form__submit order-form__submit--danger" onClick={confirmRemove} disabled={busy}>
              {busy ? "Removing..." : "Remove"}
            </button>
            <button type="button" className="od-button--quiet" onClick={() => setRemoving(null)}>
              Keep it
            </button>
          </>
        }
      >
        <p>
          {removing?.address_line}
          {removing?.area ? `, ${removing.area}` : ""} will be removed from your saved addresses. Past orders keep
          the address they were sent to.
        </p>
        {removeError && (
          <p className="order-form__error" role="alert">
            {removeError}
          </p>
        )}
      </Modal>
    </main>
  );
}
