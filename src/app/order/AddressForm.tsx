"use client";

import { useState } from "react";
import { createAddress, updateAddress, type Address, type Zone } from "@/lib/api/orders";
import { errorMessage } from "@/lib/api/client";
import { formatKobo } from "@/lib/money";

const empty = {
  recipient_name: "",
  phone: "",
  address_line: "",
  area: "",
  city: "",
  instructions: "",
  zone_id: "",
};

/**
 * Saves a delivery address, or changes one when `initial` is given. The zone decides the delivery
 * fee, so checkout needs it, but an address can be saved without one while managing addresses.
 */
export default function AddressForm({
  zones,
  defaultName,
  initial,
  zoneRequired = true,
  onSaved,
  onCancel,
}: {
  zones: Zone[];
  defaultName: string;
  initial?: Address;
  zoneRequired?: boolean;
  onSaved: (address: Address) => void;
  onCancel?: () => void;
}) {
  const [form, setForm] = useState(
    initial
      ? {
          recipient_name: initial.recipient_name,
          phone: initial.phone,
          address_line: initial.address_line,
          area: initial.area,
          city: initial.city,
          instructions: initial.instructions,
          zone_id: initial.zone_id ?? "",
        }
      : { ...empty, recipient_name: defaultName },
  );
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const set = (key: keyof typeof empty) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setForm((f) => ({ ...f, [key]: e.target.value }));

  const save = async () => {
    if (!form.recipient_name.trim()) return setError("Please enter the recipient's name.");
    if (!form.phone.trim()) return setError("Please enter a phone number.");
    if (!form.address_line.trim()) return setError("Please enter the street address.");
    if (zoneRequired && zones.length > 0 && !form.zone_id) return setError("Please choose a delivery zone.");
    setError("");
    setBusy(true);
    try {
      onSaved(initial ? await updateAddress(initial.id, form) : await createAddress(form));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="order-form order-address">
      <label className="order-form__field">
        <span>Recipient name</span>
        <input value={form.recipient_name} onChange={set("recipient_name")} autoComplete="name" />
      </label>
      <label className="order-form__field">
        <span>Phone</span>
        <input type="tel" value={form.phone} onChange={set("phone")} autoComplete="tel" />
      </label>
      <label className="order-form__field">
        <span>Street address</span>
        <input value={form.address_line} onChange={set("address_line")} autoComplete="street-address" />
      </label>
      <div className="order-address__row">
        <label className="order-form__field">
          <span>Area</span>
          <input value={form.area} onChange={set("area")} />
        </label>
        <label className="order-form__field">
          <span>City</span>
          <input value={form.city} onChange={set("city")} autoComplete="address-level2" />
        </label>
      </div>
      {zones.length > 0 && (
        <label className="order-form__field">
          <span>Delivery zone</span>
          <select value={form.zone_id} onChange={set("zone_id")}>
            <option value="">Choose a zone</option>
            {zones.map((z) => (
              <option key={z.id} value={z.id}>
                {z.name} · {formatKobo(z.fee_kobo)}
              </option>
            ))}
          </select>
        </label>
      )}
      <label className="order-form__field">
        <span>Instructions (optional)</span>
        <input value={form.instructions} onChange={set("instructions")} />
      </label>

      {error && (
        <p className="order-form__error" role="alert">
          {error}
        </p>
      )}
      <div className="order-address__actions">
        <button type="button" className="order-form__submit" disabled={busy} onClick={save}>
          {busy ? "Saving..." : initial ? "Save changes" : "Save address"}
        </button>
        {onCancel && (
          <button type="button" className="order__switch" onClick={onCancel}>
            Cancel
          </button>
        )}
      </div>
    </div>
  );
}
