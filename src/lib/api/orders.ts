import { api } from "./client";

export type Fulfillment = "PICKUP" | "DELIVERY";
export type PaymentMethod = "ONLINE" | "BANK_TRANSFER" | "PAY_ON_PICKUP" | "PAY_ON_DELIVERY";

export type Zone = { id: string; name: string; fee_kobo: number; active: boolean };

export type CheckoutOptions = {
  payment_methods: Record<Fulfillment, PaymentMethod[]>;
  delivery_zones: Zone[];
};

export type Address = {
  id: string;
  recipient_name: string;
  phone: string;
  address_line: string;
  area: string;
  city: string;
  instructions: string;
  zone_id?: string;
  zone_name?: string;
};

export type NewAddress = Omit<Address, "id" | "zone_name"> & { zone_id: string };

export type PaymentInstructions = {
  method: PaymentMethod;
  bank_name: string;
  account_number: string;
  account_name: string;
  reference: string;
  amount_kobo: number;
  whatsapp_url?: string;
};

export type ApiOrder = {
  id: string;
  order_number: string;
  status: string;
  payment_method: PaymentMethod;
  fulfillment_type: Fulfillment;
  subtotal_kobo: number;
  delivery_fee_kobo: number;
  total_kobo: number;
  created_at: string;
  updated_at: string;
  notes: string;
  delivery_address: Address | null;
  payment_instructions: PaymentInstructions | null;
  items: {
    product_id: string;
    name: string;
    unit_price_kobo: number;
    quantity: number;
    line_total_kobo: number;
  }[];
};

export const getCheckoutOptions = () => api.get<CheckoutOptions>("/api/v1/checkout/options");

export const listAddresses = () =>
  api.get<{ addresses: Address[] }>("/api/v1/addresses").then((r) => r.addresses);

export const createAddress = (input: NewAddress) =>
  api.post<{ address: Address }>("/api/v1/addresses", input).then((r) => r.address);

export type PlaceOrderInput = {
  items: { product_id: string; quantity: number }[];
  fulfillment: { type: Fulfillment; address_id?: string };
  payment_method: PaymentMethod;
  notes?: string;
};

/** The key makes a retried request safe: the same key never creates a second order. */
export const placeOrder = (input: PlaceOrderInput, idempotencyKey: string) =>
  api.post<{ order: ApiOrder }>("/api/v1/orders", input, { idempotencyKey }).then((r) => r.order);

export type OrderStatusView = { status: string; payment_status: string; updated_at: string };

export type ApiOrderFull = ApiOrder & { payment_status: string; status_reason?: string };

export const getOrder = (id: string) =>
  api.get<{ order: ApiOrderFull }>(`/api/v1/orders/${id}`).then((r) => r.order);

export const getOrderStatus = (id: string) => api.get<OrderStatusView>(`/api/v1/orders/${id}/status`);

export type PaymentInit = { reference: string; authorization_url: string; amount_kobo: number };

/** Starts a Paystack payment attempt. The customer is then sent to authorization_url. */
export const initializePayment = (orderId: string) =>
  api.post<PaymentInit>("/api/v1/payments/initialize", { order_id: orderId });

export type PaymentCheck = {
  payment: { status: string; gateway_response?: string };
  order_status: string;
  order_payment_status: string;
};

/** Asks the gateway what happened. The browser is never trusted to say a payment worked. */
export const verifyPayment = (reference: string) =>
  api.post<PaymentCheck>("/api/v1/payments/verify", { reference });

// ---------- Order history ----------

export type OrderPage = { orders: ApiOrderFull[]; total: number; limit: number; offset: number };

export const listOrders = (limit = 20, offset = 0) =>
  api.get<OrderPage>("/api/v1/orders", { limit, offset });

/** Cancels an order while it is still pending. */
export const cancelOrder = (id: string) =>
  api.post<{ order: ApiOrderFull }>(`/api/v1/orders/${id}/cancel`).then((r) => r.order);

// ---------- Notifications ----------

export type AppNotification = {
  id: string;
  kind: string;
  title: string;
  body: string;
  order_id?: string;
  read_at: string | null;
  created_at: string;
};

export const listNotifications = (limit = 10) =>
  api.get<{ notifications: AppNotification[]; unread_count: number }>("/api/v1/notifications", { limit });

export const getUnreadCount = () =>
  api.get<{ unread_count: number }>("/api/v1/notifications/unread-count").then((r) => r.unread_count);

export const markNotificationRead = (id: string) => api.post(`/api/v1/notifications/${id}/read`);

export const markAllNotificationsRead = () => api.post("/api/v1/notifications/read-all");

// ---------- Payments ----------

export type PaymentRow = {
  id: string;
  reference?: string;
  order_id: string;
  order_number: string;
  amount_kobo: number;
  method: string;
  provider: string;
  status: string;
  refund_pending: boolean;
  paid_at: string | null;
  created_at: string;
};

export type PaymentsPage = {
  payments: PaymentRow[];
  total: number;
  limit: number;
  offset: number;
  summary: { paid_kobo: number; refunded_kobo: number; refund_pending_kobo: number };
};

export const listPayments = (limit = 50, offset = 0) =>
  api.get<PaymentsPage>("/api/v1/payments", { limit, offset });

// ---------- Addresses ----------

export const updateAddress = (id: string, input: Partial<NewAddress>) =>
  api.patch<{ address: Address }>(`/api/v1/addresses/${id}`, input).then((r) => r.address);

export const deleteAddress = (id: string) => api.delete<void>(`/api/v1/addresses/${id}`);
