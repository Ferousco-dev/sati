"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useStore } from "@/lib/store";
import { useAuth } from "@/lib/auth";
import { useCatalog, toCartItem } from "@/lib/catalog";
import { naira } from "@/lib/menu";
import { formatKobo } from "@/lib/money";
import { isApiError, errorMessage, newIdempotencyKey } from "@/lib/api/client";
import {
  getCheckoutOptions,
  listAddresses,
  placeOrder,
  initializePayment,
  type Address,
  type ApiOrder,
  type CheckoutOptions,
  type Fulfillment,
  type PaymentMethod,
} from "@/lib/api/orders";
import OrderSuccess from "@/components/OrderSuccess/OrderSuccess";
import AddressForm from "./AddressForm";
// @ts-ignore: CSS module import for side effects
import "./order.css";
// @ts-ignore: CSS module import for side effects
import "./checkout.css";
import Img from "@/components/ui/Img";

const METHOD_LABEL: Record<PaymentMethod, string> = {
  ONLINE: "Pay online (card or bank)",
  BANK_TRANSFER: "Bank transfer",
  PAY_ON_PICKUP: "Pay when you pick up",
  PAY_ON_DELIVERY: "Pay on delivery",
};

/** Friendly copy for the codes the API returns when an order can't be placed. */
function placeError(err: unknown): string {
  if (isApiError(err, "PRODUCT_UNAVAILABLE") || isApiError(err, "PRODUCT_NOT_FOUND")) {
    return "Some items in your bag are no longer available. We've refreshed your bag, please review it.";
  }
  if (isApiError(err, "ADDRESS_ZONE_REQUIRED")) return "Please choose a delivery zone for this address.";
  if (isApiError(err, "ZONE_UNAVAILABLE")) return "We can't deliver to that zone right now. Try another address or pick up instead.";
  if (isApiError(err, "PAYMENT_METHOD_DISABLED")) return "That payment method isn't available right now. Please choose another.";
  return errorMessage(err);
}

export default function OrderPage() {
  const { cart, addQuiet, setQty, remove, clearCart } = useStore();
  const { user, loading } = useAuth();
  const { items: catalog, status, reload } = useCatalog();
  const router = useRouter();

  const [options, setOptions] = useState<CheckoutOptions | null>(null);
  const [addresses, setAddresses] = useState<Address[]>([]);
  const [fulfillment, setFulfillment] = useState<Fulfillment>("PICKUP");
  const [addressId, setAddressId] = useState("");
  const [addingAddress, setAddingAddress] = useState(false);
  const [method, setMethod] = useState<PaymentMethod | "">("");
  const [notes, setNotes] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [placed, setPlaced] = useState<ApiOrder | null>(null);
  const [redirecting, setRedirecting] = useState(false);
  const attempt = useRef<{ payload: string; key: string } | null>(null);

  // Ordering needs an account, so send guests to sign in and bring them back here.
  useEffect(() => {
    if (!loading && !user && !placed) router.replace("/signin?next=/order");
  }, [loading, user, placed, router]);

  useEffect(() => {
    if (!user) return;
    getCheckoutOptions().then(setOptions, (e) => setError(errorMessage(e)));
    listAddresses().then(setAddresses, () => {});
  }, [user]);

  // The bag only holds real products. Anything saved before the catalog existed is dropped.
  const byId = useMemo(() => new Map(catalog.map((i) => [i.id, i])), [catalog]);
  useEffect(() => {
    if (status !== "ready") return;
    cart.forEach((l) => !byId.has(l.id) && remove(l.id));
  }, [status, cart, byId, remove]);

  const lines = cart
    .map((l) => ({ line: l, product: byId.get(l.id) }))
    .filter((x) => x.product);
  const soldOut = lines.filter((x) => !x.product!.available);
  const subtotal = lines.reduce((sum, x) => sum + x.product!.price * x.line.qty, 0);

  const zones = options?.delivery_zones.filter((z) => z.active) ?? [];
  const canDeliver = zones.length > 0 && (options?.payment_methods.DELIVERY?.length ?? 0) > 0;
  const methods = (options?.payment_methods[fulfillment] ?? []);

  // Keep the chosen payment method valid when the fulfilment type changes.
  useEffect(() => {
    if (!methods.some((m) => m === method)) setMethod(methods[0] ?? "");
  }, [methods.join(), method]); // eslint-disable-line react-hooks/exhaustive-deps

  const address = addresses.find((a) => a.id === addressId);
  const fee = fulfillment === "DELIVERY" ? zones.find((z) => z.id === address?.zone_id)?.fee_kobo ?? 0 : 0;

  if (placed) {
    return (
      <main className="order">
        <OrderSuccess order={placed} name={user?.name ?? ""} />
      </main>
    );
  }

  if (redirecting) {
    return (
      <main className="order">
        <div className="order__empty">
          <h1 className="order__title">Taking you to payment...</h1>
          <p className="order__lede">Please don&apos;t close this page.</p>
        </div>
      </main>
    );
  }

  if (loading || !user || status === "loading") return <main className="order" />;

  if (lines.length === 0) {
    return (
      <main className="order">
        <div className="order__empty">
          <h1 className="order__title">Your bag is empty</h1>
          <p className="order__lede">Add something from the menu to order.</p>
          <a href="/menu" className="order-form__submit order-form__submit--link">
            Browse the menu
          </a>
        </div>
      </main>
    );
  }

  const submit = async () => {
    if (soldOut.length) return setError("Please remove the sold out items from your bag.");
    if (!method) return setError("Please choose a payment method.");
    if (fulfillment === "DELIVERY" && !address) return setError("Please choose or add a delivery address.");
    setError("");
    setBusy(true);

    const payload = {
      items: lines.map((x) => ({ product_id: x.line.id, quantity: x.line.qty })),
      fulfillment: { type: fulfillment, address_id: fulfillment === "DELIVERY" ? addressId : undefined },
      payment_method: method,
      notes: notes.trim() || undefined,
    };
    // A retry of the same order reuses its key, so it can never be placed twice.
    const json = JSON.stringify(payload);
    if (attempt.current?.payload !== json) attempt.current = { payload: json, key: newIdempotencyKey() };

    try {
      const order = await placeOrder(payload, attempt.current.key);
      if (order.payment_method === "ONLINE") setRedirecting(true);
      clearCart();
      if (order.payment_method === "ONLINE") {
        // Hand over to Paystack. If it can't start, the order page lets the customer try again.
        try {
          const init = await initializePayment(order.id);
          window.location.assign(init.authorization_url);
        } catch {
          router.push(`/orders/${order.id}?retry=1`);
        }
        return;
      }
      setPlaced(order);
    } catch (err) {
      setError(placeError(err));
      if (isApiError(err, "PRODUCT_UNAVAILABLE") || isApiError(err, "PRODUCT_NOT_FOUND")) reload();
    } finally {
      setBusy(false);
    }
  };

  const upsell = catalog.filter((i) => i.available && !cart.some((l) => l.id === i.id)).slice(0, 3);

  return (
    <main className="order">
      <div className="order__review">
        <header className="order__head">
          <div>
            <span className="order__eyebrow">Review &amp; confirm</span>
            <h1 className="order__title">Your order</h1>
          </div>
          <p className="order__who">
            {user.name}
            <span>{user.email}</span>
          </p>
        </header>

        <ul className="order-lines">
          {lines.map(({ line, product }) => (
            <li key={line.id} className="order-line">
              {product!.image ? <Img src={product!.image} alt={line.name} /> : <span className="order-line__noimg" />}
              <div className="order-line__body">
                <p className="order-line__name">{line.name}</p>
                <p className="order-line__price">
                  {product!.available ? `${naira(product!.price)} each` : "Sold out"}
                </p>
              </div>
              <div className="order-line__qty">
                <button type="button" aria-label={`Decrease ${line.name}`} onClick={() => setQty(line.id, line.qty - 1)}>
                  −
                </button>
                <span>{line.qty}</span>
                <button type="button" aria-label={`Increase ${line.name}`} onClick={() => setQty(line.id, line.qty + 1)}>
                  +
                </button>
              </div>
              <span className="order-line__sum">{naira(product!.price * line.qty)}</span>
              <button type="button" className="order-line__remove" aria-label={`Remove ${line.name}`} onClick={() => remove(line.id)}>
                ✕
              </button>
            </li>
          ))}
        </ul>

        {upsell.length > 0 && (
          <div className="upsell">
            <p className="upsell__title">Add something more</p>
            <div className="upsell__row">
              {upsell.map((item) => (
                <button key={item.id} type="button" className="upsell__item" onClick={() => addQuiet(toCartItem(item))}>
                  {item.image ? <Img src={item.image} alt="" /> : null}
                  <span className="upsell__name">{item.name}</span>
                  <span className="upsell__add">+ {naira(item.price)}</span>
                </button>
              ))}
            </div>
          </div>
        )}

        <section className="checkout-section">
          <h2 className="checkout-section__title">How would you like it?</h2>
          <div className="checkout-choices" role="radiogroup" aria-label="Pickup or delivery">
            {(["PICKUP", "DELIVERY"] as Fulfillment[]).map((f) => {
              const off = f === "DELIVERY" && !canDeliver;
              return (
                <label key={f} className={`checkout-choice ${fulfillment === f ? "is-active" : ""} ${off ? "is-off" : ""}`}>
                  <input type="radio" name="fulfillment" checked={fulfillment === f} disabled={off} onChange={() => setFulfillment(f)} />
                  <span>{f === "PICKUP" ? "Pickup" : "Delivery"}</span>
                  {off && <small>Not available yet</small>}
                </label>
              );
            })}
          </div>
        </section>

        {fulfillment === "DELIVERY" && (
          <section className="checkout-section">
            <h2 className="checkout-section__title">Delivery address</h2>
            {addresses.length > 0 && !addingAddress && (
              <div className="checkout-choices checkout-choices--stack" role="radiogroup" aria-label="Saved addresses">
                {addresses.map((a) => (
                  <label key={a.id} className={`checkout-choice ${addressId === a.id ? "is-active" : ""}`}>
                    <input type="radio" name="address" checked={addressId === a.id} onChange={() => setAddressId(a.id)} />
                    <span>
                      {a.recipient_name}, {a.address_line}
                      {a.area ? `, ${a.area}` : ""}
                    </span>
                    {a.zone_name && <small>{a.zone_name}</small>}
                  </label>
                ))}
              </div>
            )}
            {addingAddress || addresses.length === 0 ? (
              <AddressForm
                zones={zones}
                defaultName={user.name}
                onCancel={addresses.length ? () => setAddingAddress(false) : undefined}
                onSaved={(a) => {
                  setAddresses((list) => [a, ...list]);
                  setAddressId(a.id);
                  setAddingAddress(false);
                }}
              />
            ) : (
              <button type="button" className="order__switch" onClick={() => setAddingAddress(true)}>
                + Add a new address
              </button>
            )}
          </section>
        )}

        <section className="checkout-section">
          <h2 className="checkout-section__title">Payment</h2>
          {methods.length === 0 ? (
            <p className="order__lede">No payment methods are available right now. Please try again shortly.</p>
          ) : (
            <div className="checkout-radios" role="radiogroup" aria-label="Payment method">
              {methods.map((m) => (
                <label key={m} className="checkout-radio">
                  <input type="radio" name="payment" checked={method === m} onChange={() => setMethod(m)} />
                  <span>{METHOD_LABEL[m]}</span>
                </label>
              ))}
            </div>
          )}
          <label className="order-form__field checkout-notes">
            <span>Note for the kitchen (optional)</span>
            <input value={notes} maxLength={200} onChange={(e) => setNotes(e.target.value)} />
          </label>
        </section>

        <div className="order__summary">
          <div className="checkout-sum">
            <span>Subtotal</span>
            <span>{naira(subtotal)}</span>
          </div>
          {fulfillment === "DELIVERY" && (
            <div className="checkout-sum">
              <span>Delivery</span>
              <span>{address ? formatKobo(fee) : "Choose an address"}</span>
            </div>
          )}
          <div className="order__total">
            <span>Total</span>
            <span>{formatKobo(Math.round(subtotal * 100) + fee)}</span>
          </div>
          <p className="checkout-hint">The final total is confirmed by us when you place the order.</p>

          {error && (
            <p className="order-form__error" role="alert">
              {error}
            </p>
          )}

          <button type="button" className="order__place" onClick={submit} disabled={busy || methods.length === 0}>
            {busy ? "Placing order..." : "Place order"}
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path d="M5 12h14M13 6l6 6-6 6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </button>
          <a href="/menu" className="order__back">
            Add more items
          </a>
        </div>
      </div>
    </main>
  );
}
