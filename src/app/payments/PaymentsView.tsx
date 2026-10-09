"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth";
import { errorMessage } from "@/lib/api/client";
import { listPayments, type PaymentsPage } from "@/lib/api/orders";
import { formatKobo } from "@/lib/money";
import { METHOD_NAME, paymentLabel, paymentTone } from "@/lib/order-status";
import StatusBadge from "@/components/StatusBadge/StatusBadge";
import EmptyState from "@/components/ui/EmptyState";
import Skeleton from "@/components/ui/Skeleton";
// @ts-ignore: CSS module import for side effects
import "../order/order.css";
// @ts-ignore: CSS module import for side effects
import "../account/account.css";
// @ts-ignore: CSS module import for side effects
import "../orders/orders.css";
// @ts-ignore: CSS module import for side effects
import "./payments.css";

export default function PaymentsView() {
  const { user, loading } = useAuth();
  const router = useRouter();
  const [data, setData] = useState<PaymentsPage | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!loading && !user) router.replace("/signin?next=/payments");
  }, [loading, user, router]);

  const load = useCallback(() => {
    setError("");
    listPayments().then(setData, (err) => setError(errorMessage(err)));
  }, []);

  useEffect(() => {
    if (user) load();
  }, [user, load]);

  if (loading || !user) return <main className="account-page" />;

  return (
    <main className="account-page">
      <div className="account-wrap account-wrap--narrow">
        <header className="account-intro">
          <span className="order__eyebrow">Your account</span>
          <h1 className="order__title">My payments</h1>
        </header>

        {error && (
          <EmptyState
            title="We couldn't load your payments"
            text={error}
            action={
              <button type="button" className="order-form__submit" onClick={load}>
                Try again
              </button>
            }
          />
        )}

        {!data && !error && (
          <div className="account-card orders-skeleton" aria-hidden="true">
            <Skeleton height="1.2rem" width="40%" />
            <Skeleton height="1rem" width="70%" />
          </div>
        )}

        {data && (
          <>
            <dl className="pay-summary">
              <div className="account-card">
                <dt>Paid</dt>
                <dd>{formatKobo(data.summary.paid_kobo)}</dd>
              </div>
              <div className="account-card">
                <dt>Refunded</dt>
                <dd>{formatKobo(data.summary.refunded_kobo)}</dd>
              </div>
              <div className="account-card">
                <dt>Waiting for refund</dt>
                <dd>{formatKobo(data.summary.refund_pending_kobo)}</dd>
              </div>
            </dl>

            {data.payments.length === 0 ? (
              <div className="account-card account__empty">
                <p>No payments yet.</p>
                <a href="/menu" className="order-form__submit order-form__submit--link">
                  Start an order
                </a>
              </div>
            ) : (
              <ul className="account__orders">
                {data.payments.map((p) => {
                  const when = new Date(p.paid_at ?? p.created_at);
                  return (
                    <li key={p.id} className="record pay-row">
                      <div className="pay-row__main">
                        <a href={`/orders/${p.order_id}`} className="record__id">
                          #{p.order_number}
                        </a>
                        <span className="pay-row__meta">
                          {METHOD_NAME[p.method] ?? p.method} · {when.toLocaleDateString()}
                        </span>
                        {p.reference && <span className="pay-row__ref">Ref: {p.reference}</span>}
                      </div>
                      <div className="pay-row__side">
                        <span className="record__total">{formatKobo(p.amount_kobo)}</span>
                        <span className="pay-row__badges">
                          <StatusBadge label={paymentLabel(p.status)} tone={paymentTone(p.status)} />
                          {p.refund_pending && <StatusBadge label="Refund pending" tone="warn" />}
                        </span>
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </>
        )}
      </div>
    </main>
  );
}
