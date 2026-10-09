"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useAuth } from "@/lib/auth";
import { errorMessage, isApiError } from "@/lib/api/client";
import {
  cancelOrder,
  getOrder,
  getOrderStatus,
  initializePayment,
  verifyPayment,
  type ApiOrderFull,
} from "@/lib/api/orders";
import { formatKobo } from "@/lib/money";
import { METHOD_NAME, isFinished, paymentLabel, paymentTone, statusLabel, statusTone } from "@/lib/order-status";
import { useReorder } from "@/lib/reorder";
import { useToast } from "@/components/ui/Toast";
import StatusBadge from "@/components/StatusBadge/StatusBadge";
import OrderTracker from "@/components/OrderTracker/OrderTracker";
import OrderSuccess from "@/components/OrderSuccess/OrderSuccess";
import Modal from "@/components/ui/Modal";
// @ts-ignore: CSS module import for side effects
import "../../order/order.css";
// @ts-ignore: CSS module import for side effects
import "../../account/account.css";
// @ts-ignore: CSS module import for side effects
import "../orders.css";

// While a payment is pending, check again with a growing delay. Afterwards, a calm steady rhythm.
const BACKOFF_MS = [2000, 3000, 5000, 8000, 12000, 15000, 15000, 15000];
const STEADY_MS = 15000;

export default function OrderStatusView() {
  const { id } = useParams<{ id: string }>();
  const params = useSearchParams();
  const reference = params.get("reference") ?? params.get("trxref") ?? "";
  const { user, loading } = useAuth();
  const router = useRouter();
  const toast = useToast();
  const reorder = useReorder();

  const [order, setOrder] = useState<ApiOrderFull | null>(null);
  const [error, setError] = useState("");
  const [gaveUp, setGaveUp] = useState(false);
  const [retrying, setRetrying] = useState(false);
  const [cancelOpen, setCancelOpen] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [cancelError, setCancelError] = useState("");
  const attempt = useRef(0);

  useEffect(() => {
    if (!loading && !user) router.replace(`/signin?next=/orders/${id}`);
  }, [loading, user, id, router]);

  const refresh = useCallback(async () => {
    try {
      setOrder(await getOrder(id));
      setError("");
    } catch (err) {
      setError(isApiError(err, "ORDER_NOT_FOUND") ? "We couldn't find that order." : errorMessage(err));
    }
  }, [id]);

  // First load. Coming back from Paystack, ask the gateway before trusting anything.
  useEffect(() => {
    if (!user) return;
    (async () => {
      if (reference) await verifyPayment(reference).catch(() => {});
      await refresh();
    })();
  }, [user, reference, refresh]);

  const paymentPending = order?.payment_status === "PENDING";
  const active = !!order && !isFinished(order.status);

  // Keep the page live. The status endpoint is light, and the full order is only fetched when something changed.
  useEffect(() => {
    if (!order || !active) return;
    const delay = paymentPending ? BACKOFF_MS[attempt.current] ?? STEADY_MS : STEADY_MS;
    const t = window.setTimeout(async () => {
      attempt.current += 1;
      if (paymentPending && attempt.current >= BACKOFF_MS.length) setGaveUp(true);
      try {
        if (paymentPending && reference) await verifyPayment(reference).catch(() => {});
        const latest = await getOrderStatus(id);
        if (
          latest.status !== order.status ||
          latest.payment_status !== order.payment_status ||
          latest.updated_at !== order.updated_at
        ) {
          await refresh();
        } else {
          // Nothing changed, so look again later without redrawing.
          setOrder((o) => (o ? { ...o } : o));
        }
      } catch {
        // A missed check is fine. The next one will catch up.
      }
    }, delay);
    return () => window.clearTimeout(t);
  }, [order, active, paymentPending, reference, id, refresh]);

  const retryPayment = async () => {
    setRetrying(true);
    setError("");
    try {
      const init = await initializePayment(id);
      window.location.assign(init.authorization_url);
    } catch (err) {
      setError(
        isApiError(err, "PAYMENT_ATTEMPTS_EXCEEDED")
          ? "Too many payment attempts for this order. Please place a new order."
          : isApiError(err, "ORDER_NOT_PAYABLE")
            ? "This order can no longer be paid."
            : errorMessage(err),
      );
      setRetrying(false);
    }
  };

  const confirmCancel = async () => {
    setCancelError("");
    setCancelling(true);
    try {
      setOrder(await cancelOrder(id));
      setCancelOpen(false);
      toast("Your order was cancelled.");
    } catch (err) {
      setCancelError(
        isApiError(err, "ORDER_NOT_CANCELLABLE")
          ? "This order has already moved on and can no longer be cancelled."
          : errorMessage(err),
      );
      if (isApiError(err, "ORDER_NOT_CANCELLABLE")) refresh();
    } finally {
      setCancelling(false);
    }
  };

  if (loading || !user) return <main className="account-page" />;

  if (!order) {
    return (
      <main className="account-page account-page--center">
        <div className="account-card account-card--narrow">
          <h1 className="order__title">{error ? "Order not found" : "Loading your order..."}</h1>
          {error && (
            <p className="order-form__error" role="alert">
              {error}
            </p>
          )}
          <a href="/orders" className="order-form__submit order-form__submit--link">
            My orders
          </a>
        </div>
      </main>
    );
  }

  // Just back from a successful payment: keep the celebration the site is known for.
  if (reference && order.payment_status === "PAID") {
    return (
      <main className="order">
        <OrderSuccess order={order} name={user.name} />
      </main>
    );
  }

  const unpaidOnline =
    order.payment_method === "ONLINE" && order.payment_status !== "PAID" && !isFinished(order.status);
  const canCancel = order.status === "PENDING";
  const when = new Date(order.created_at);
  const address = order.delivery_address;
  const pay = order.payment_instructions;

  return (
    <main className="account-page">
      <div className="account-wrap account-wrap--narrow od">
        <a href="/orders" className="od-back">
          <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path d="M19 12H5M11 6l-6 6 6 6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
          Back to orders
        </a>

        <header className="od__head">
          <div>
            <span className="order__eyebrow">Order</span>
            <h1 className="order__title">#{order.order_number}</h1>
            <div className="od__badges">
              <StatusBadge label={statusLabel(order.status)} tone={statusTone(order.status)} />
              <StatusBadge label={paymentLabel(order.payment_status)} tone={paymentTone(order.payment_status)} />
            </div>
          </div>
          <p className="od__date">
            {when.toLocaleDateString()} ·{" "}
            {when.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
          </p>
        </header>

        {unpaidOnline && (
          <section className="account-card od-notice" role="status">
            <h2 className="od-notice__title">
              {paymentPending && !gaveUp
                ? "Confirming your payment"
                : paymentPending
                  ? "Still waiting for confirmation"
                  : "This order isn't paid yet"}
            </h2>
            <p>
              {paymentPending && !gaveUp
                ? "We're waiting for the bank to confirm. You can leave this page and this order will update on its own."
                : paymentPending
                  ? "We haven't heard back from the bank yet. If you were charged, this order updates once it confirms. You can also try again."
                  : "Pay online to confirm it. Unpaid online orders are cancelled after 30 minutes."}
            </p>
            {!paymentPending || gaveUp ? (
              <button type="button" className="order-form__submit" onClick={retryPayment} disabled={retrying}>
                {retrying ? "Please wait..." : "Pay now"}
              </button>
            ) : null}
          </section>
        )}

        {error && (
          <p className="order-form__error" role="alert">
            {error}
          </p>
        )}

        {pay && order.payment_method === "BANK_TRANSFER" && order.payment_status !== "PAID" && !isFinished(order.status) && (
          <section className="account-card od-card">
            <h2 className="od-card__title">Pay by bank transfer</h2>
            <dl className="od-facts">
              <div><dt>Bank</dt><dd>{pay.bank_name}</dd></div>
              <div><dt>Account number</dt><dd>{pay.account_number}</dd></div>
              <div><dt>Account name</dt><dd>{pay.account_name}</dd></div>
              <div><dt>Reference</dt><dd>{pay.reference}</dd></div>
              <div className="od-facts__wide"><dt>Amount</dt><dd>{formatKobo(pay.amount_kobo)}</dd></div>
            </dl>
            {pay.whatsapp_url && (
              <p className="od-notice">
                <a className="auth__link" href={pay.whatsapp_url} target="_blank" rel="noopener noreferrer">
                  Send your proof of payment on WhatsApp
                </a>
              </p>
            )}
          </section>
        )}

        <OrderTracker status={order.status} fulfillment={order.fulfillment_type} reason={order.status_reason} />

        <section className="account-card od-card">
          <h2 className="od-card__title">Items</h2>
          <ul className="od-lines">
            {order.items.map((line) => (
              <li key={line.product_id}>
                <span>
                  {line.quantity}× {line.name}
                </span>
                <span>{formatKobo(line.line_total_kobo)}</span>
              </li>
            ))}
          </ul>
          <div className="od-sum">
            <span>Subtotal</span>
            <span>{formatKobo(order.subtotal_kobo)}</span>
          </div>
          {order.fulfillment_type === "DELIVERY" && (
            <div className="od-sum">
              <span>Delivery</span>
              <span>{formatKobo(order.delivery_fee_kobo)}</span>
            </div>
          )}
          <div className="od-sum od-sum--total">
            <span>Total</span>
            <span>{formatKobo(order.total_kobo)}</span>
          </div>
        </section>

        <section className="account-card od-card">
          <h2 className="od-card__title">Details</h2>
          <dl className="od-facts">
            <div>
              <dt>How you get it</dt>
              <dd>{order.fulfillment_type === "DELIVERY" ? "Delivery" : "Pickup"}</dd>
            </div>
            <div>
              <dt>Payment</dt>
              <dd>
                {METHOD_NAME[order.payment_method] ?? order.payment_method} · {paymentLabel(order.payment_status)}
              </dd>
            </div>
            {address && (
              <div className="od-facts__wide">
                <dt>Delivery address</dt>
                <dd>
                  {address.recipient_name}, {address.address_line}
                  {address.area ? `, ${address.area}` : ""}
                  {address.city ? `, ${address.city}` : ""}
                  {address.phone ? ` · ${address.phone}` : ""}
                </dd>
              </div>
            )}
            {order.notes && (
              <div className="od-facts__wide">
                <dt>Your note</dt>
                <dd>{order.notes}</dd>
              </div>
            )}
          </dl>
        </section>

        <div className="od-actions">
          <button type="button" className="order-form__submit" onClick={() => reorder(order)}>
            Reorder
          </button>
          {canCancel && (
            <button type="button" className="od-button--quiet" onClick={() => setCancelOpen(true)}>
              Cancel order
            </button>
          )}
        </div>
      </div>

      <Modal
        open={cancelOpen}
        title="Cancel this order?"
        onClose={() => setCancelOpen(false)}
        actions={
          <>
            <button type="button" className="order-form__submit order-form__submit--danger" onClick={confirmCancel} disabled={cancelling}>
              {cancelling ? "Cancelling..." : "Yes, cancel it"}
            </button>
            <button type="button" className="od-button--quiet" onClick={() => setCancelOpen(false)}>
              Keep it
            </button>
          </>
        }
      >
        <p>
          Order #{order.order_number} will be cancelled. You can only cancel before we start on it.
          {order.payment_status === "PAID" ? " Because you've already paid, our team will arrange your refund." : ""}
        </p>
        {cancelError && (
          <p className="order-form__error" role="alert">
            {cancelError}
          </p>
        )}
      </Modal>
    </main>
  );
}
