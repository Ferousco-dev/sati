"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth";
import { errorMessage } from "@/lib/api/client";
import { listOrders, type ApiOrderFull } from "@/lib/api/orders";
import { formatKobo } from "@/lib/money";
import { paymentLabel, paymentTone, statusLabel, statusTone } from "@/lib/order-status";
import { useReorder } from "@/lib/reorder";
import StatusBadge from "@/components/StatusBadge/StatusBadge";
import EmptyState from "@/components/ui/EmptyState";
import Skeleton from "@/components/ui/Skeleton";
// @ts-ignore: CSS module import for side effects
import "../order/order.css";
// @ts-ignore: CSS module import for side effects
import "../account/account.css";
// @ts-ignore: CSS module import for side effects
import "./orders.css";

const PAGE = 20;

export default function OrdersView() {
  const { user, loading } = useAuth();
  const router = useRouter();
  const reorder = useReorder();
  const [orders, setOrders] = useState<ApiOrderFull[]>([]);
  const [total, setTotal] = useState(0);
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");
  const [error, setError] = useState("");
  const [more, setMore] = useState(false);

  useEffect(() => {
    if (!loading && !user) router.replace("/signin?next=/orders");
  }, [loading, user, router]);

  const load = useCallback(async (offset: number) => {
    try {
      const page = await listOrders(PAGE, offset);
      setOrders((prev) => (offset === 0 ? page.orders : [...prev, ...page.orders]));
      setTotal(page.total);
      setState("ready");
    } catch (err) {
      setError(errorMessage(err));
      setState("error");
    }
  }, []);

  useEffect(() => {
    if (user) load(0);
  }, [user, load]);

  if (loading || !user) return <main className="account-page" />;

  return (
    <main className="account-page">
      <div className="account-wrap account-wrap--narrow">
        <header className="account-intro">
          <span className="order__eyebrow">Your account</span>
          <h1 className="order__title">My orders</h1>
        </header>

        {state === "loading" && (
          <div className="account-card orders-skeleton" aria-hidden="true">
            <Skeleton height="1.2rem" width="40%" />
            <Skeleton height="1rem" width="70%" />
            <Skeleton height="1rem" width="55%" />
          </div>
        )}

        {state === "error" && (
          <EmptyState
            title="We couldn't load your orders"
            text={error}
            action={
              <button type="button" className="order-form__submit" onClick={() => load(0)}>
                Try again
              </button>
            }
          />
        )}

        {state === "ready" && orders.length === 0 && (
          <div className="account-card account__empty">
            <p>No orders yet.</p>
            <a href="/menu" className="order-form__submit order-form__submit--link">
              Start an order
            </a>
          </div>
        )}

        {orders.length > 0 && (
          <ul className="account__orders">
            {orders.map((order) => (
              <li key={order.id} className="record">
                <div className="record__head">
                  <span className="record__id">#{order.order_number}</span>
                  <span className="record__date">
                    {new Date(order.created_at).toLocaleDateString()} ·{" "}
                    {new Date(order.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                  </span>
                  <span className="record__total">{formatKobo(order.total_kobo)}</span>
                </div>

                <div className="orders-badges">
                  <StatusBadge label={statusLabel(order.status)} tone={statusTone(order.status)} />
                  <StatusBadge label={paymentLabel(order.payment_status)} tone={paymentTone(order.payment_status)} />
                  <span className="orders-badges__kind">
                    {order.fulfillment_type === "DELIVERY" ? "Delivery" : "Pickup"}
                  </span>
                </div>

                <ul className="record__lines orders-lines">
                  {order.items.map((line) => (
                    <li key={line.product_id}>
                      <span>
                        {line.quantity}× {line.name}
                      </span>
                      <span className="record__line-sum">{formatKobo(line.line_total_kobo)}</span>
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
            ))}
          </ul>
        )}

        {state === "ready" && orders.length < total && (
          <button
            type="button"
            className="order-form__submit orders-more"
            disabled={more}
            onClick={async () => {
              setMore(true);
              await load(orders.length);
              setMore(false);
            }}
          >
            {more ? "Loading..." : "Show more"}
          </button>
        )}
      </div>
    </main>
  );
}
