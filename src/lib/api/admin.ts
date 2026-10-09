import { api } from "./client";
import type { User } from "./types";
import type { PaymentMethod, Fulfillment } from "./orders";

// ---------- Dashboard ----------

export type DashboardStats = {
  today_orders: number;
  today_revenue_kobo: number;
  today_paid: number;
  pending: number;
  awaiting_transfer: number;
  ready: number;
  out_for_delivery: number;
  refund_pending: number;
};

export const getDashboard = (tz?: string) =>
  api.get<DashboardStats>("/api/v1/admin/dashboard", { tz });

// ---------- Admin orders ----------

export type AdminOrderFilters = {
  status?: string;
  payment_status?: string;
  payment_method?: string;
  fulfillment_type?: string;
  from?: string;
  to?: string;
  q?: string;
  limit?: number;
  offset?: number;
};

export type AdminOrderItem = {
  product_id: string;
  name: string;
  unit_price_kobo: number;
  quantity: number;
  line_total_kobo: number;
  image_url?: string;
};

export type AdminOrderPayment = {
  id: string;
  reference?: string;
  amount_kobo: number;
  method: string;
  provider: string;
  status: string;
  paid_at: string | null;
  created_at: string;
};

export type AdminOrderCustomer = {
  id: string;
  name: string;
  email: string;
  phone: string;
};

export type AdminOrderDelivery = {
  id: string;
  rider_id?: string;
  rider_name?: string;
  status: string;
  failure_reason?: string;
  picked_up_at?: string;
  delivered_at?: string;
  failed_at?: string;
};

export type AdminOrderAddress = {
  recipient_name: string;
  phone: string;
  address_line: string;
  area: string;
  city: string;
  instructions: string;
  zone_name?: string;
};

export type AdminOrder = {
  id: string;
  order_number: string;
  status: string;
  payment_status: string;
  payment_method: PaymentMethod;
  fulfillment_type: Fulfillment;
  subtotal_kobo: number;
  delivery_fee_kobo: number;
  total_kobo: number;
  status_reason?: string;
  notes: string;
  created_at: string;
  updated_at: string;
  customer: AdminOrderCustomer;
  items: AdminOrderItem[];
  delivery_address: AdminOrderAddress | null;
  payments: AdminOrderPayment[];
  delivery: AdminOrderDelivery | null;
};

export type AdminOrderListItem = {
  id: string;
  order_number: string;
  status: string;
  payment_status: string;
  payment_method: PaymentMethod;
  fulfillment_type: Fulfillment;
  total_kobo: number;
  created_at: string;
  customer_name: string;
};

export type AdminOrderPage = {
  orders: AdminOrderListItem[];
  total: number;
  limit: number;
  offset: number;
};

export const listAdminOrders = (filters: AdminOrderFilters = {}) =>
  api.get<AdminOrderPage>("/api/v1/admin/orders", filters as Record<string, string | number | boolean | undefined>);

export const getAdminOrder = (id: string) =>
  api.get<{ order: AdminOrder }>(`/api/v1/admin/orders/${id}`).then((r) => r.order);

export const updateOrderStatus = (id: string, status: string, reason?: string) =>
  api.patch<{ order: AdminOrder }>(`/api/v1/admin/orders/${id}/status`, { status, reason }).then((r) => r.order);

export const recordPayment = (id: string, note?: string) =>
  api.post<{ order: AdminOrder }>(`/api/v1/admin/orders/${id}/payment`, { note }).then((r) => r.order);

export const assignRider = (id: string, rider_id: string, notes?: string) =>
  api.post<{ order: AdminOrder }>(`/api/v1/admin/orders/${id}/rider`, { rider_id, notes }).then((r) => r.order);

// ---------- Admin payments ----------

export type AdminPaymentFilters = {
  needs_refund?: boolean;
  limit?: number;
  offset?: number;
};

export type AdminPayment = {
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
  customer_name: string;
};

export type AdminPaymentPage = {
  payments: AdminPayment[];
  total: number;
  limit: number;
  offset: number;
};

export const listAdminPayments = (filters: AdminPaymentFilters = {}) =>
  api.get<AdminPaymentPage>("/api/v1/admin/payments", filters as Record<string, string | number | boolean | undefined>);

export const markRefunded = (id: string, note?: string) =>
  api.post<void>(`/api/v1/admin/payments/${id}/refunded`, { note });

// ---------- Products ----------

export type AdminProduct = {
  id: string;
  name: string;
  description: string;
  price_kobo: number;
  category_id: string;
  category_name: string;
  available: boolean;
  archived: boolean;
  image_url?: string;
  images: { id: string; url: string }[];
  created_at: string;
};

export type AdminProductPage = {
  products: AdminProduct[];
  total: number;
  limit: number;
  offset: number;
};

export type CreateProductInput = {
  category_id: string;
  name: string;
  description: string;
  price_kobo: number;
};

export type UpdateProductInput = Partial<CreateProductInput & { active: boolean }>;

export const listAdminProducts = (filters: Record<string, string | number | boolean | undefined> = {}) =>
  api.get<AdminProductPage>("/api/v1/admin/products", filters);

export const createProduct = (input: CreateProductInput) =>
  api.post<{ product: AdminProduct }>("/api/v1/admin/products", input).then((r) => r.product);

export const updateProduct = (id: string, input: UpdateProductInput) =>
  api.patch<{ product: AdminProduct }>(`/api/v1/admin/products/${id}`, input).then((r) => r.product);

export const archiveProduct = (id: string) =>
  api.post<{ product: AdminProduct }>(`/api/v1/admin/products/${id}/archive`).then((r) => r.product);

export const setAvailability = (id: string, available: boolean) =>
  api.post<{ product: AdminProduct }>(`/api/v1/admin/products/${id}/availability`, { available }).then((r) => r.product);

export const uploadProductImage = (id: string, file: File) => {
  const form = new FormData();
  form.append("image", file);
  return api.post<{ product: AdminProduct }>(`/api/v1/admin/products/${id}/images`, form).then((r) => r.product);
};

export const deleteProductImage = (id: string, imageId: string) =>
  api.delete<void>(`/api/v1/admin/products/${id}/images/${imageId}`);

// ---------- Categories ----------

export type AdminCategory = {
  id: string;
  name: string;
  description: string;
  sort_order: number;
  product_count: number;
};

export const listCategories = () =>
  api.get<{ categories: AdminCategory[] }>("/api/v1/admin/categories").then((r) => r.categories);

export const createCategory = (input: { name: string; description: string; sort_order: number }) =>
  api.post<{ category: AdminCategory }>("/api/v1/admin/categories", input).then((r) => r.category);

export const updateCategory = (id: string, input: Partial<{ name: string; description: string; sort_order: number }>) =>
  api.patch<{ category: AdminCategory }>(`/api/v1/admin/categories/${id}`, input).then((r) => r.category);

// ---------- Customers ----------

export type AdminCustomer = {
  id: string;
  name: string;
  email: string;
  phone: string;
  order_count: number;
  total_spent_kobo: number;
  created_at: string;
};

export type AdminCustomerPage = {
  customers: AdminCustomer[];
  total: number;
  limit: number;
  offset: number;
};

export const listCustomers = (filters: Record<string, string | number | boolean | undefined> = {}) =>
  api.get<AdminCustomerPage>("/api/v1/admin/customers", filters);

// ---------- Users / Team ----------

export type AdminUser = User & { last_login_at?: string };

export type AdminUserPage = {
  users: AdminUser[];
  total: number;
  limit: number;
  offset: number;
};

export const listUsers = (role?: string) =>
  api.get<AdminUserPage>("/api/v1/admin/users", { role });

export const inviteUser = (input: { name: string; email: string; role: string }) =>
  api.post<{ user: AdminUser }>("/api/v1/admin/users", input).then((r) => r.user);

export const changeRole = (id: string, role: string) =>
  api.patch<{ user: AdminUser }>(`/api/v1/admin/users/${id}/role`, { role }).then((r) => r.user);

export type Rider = { id: string; name: string; email: string; phone: string };

export const listRiders = () =>
  api.get<{ riders: Rider[] }>("/api/v1/admin/riders").then((r) => r.riders);

// ---------- Deliveries ----------

export type AdminDelivery = {
  id: string;
  order_id: string;
  order_number: string;
  rider_id?: string;
  rider_name?: string;
  status: string;
  failure_reason?: string;
  address_line: string;
  area: string;
  customer_name: string;
  customer_phone: string;
  cash_to_collect_kobo?: number;
  created_at: string;
  picked_up_at?: string;
  delivered_at?: string;
  failed_at?: string;
};

export type AdminDeliveryPage = {
  deliveries: AdminDelivery[];
  total: number;
  limit: number;
  offset: number;
};

export const listDeliveries = (filters: Record<string, string | number | boolean | undefined> = {}) =>
  api.get<AdminDeliveryPage>("/api/v1/admin/delivery", filters);

// ---------- Delivery zones ----------

export type AdminZone = {
  id: string;
  name: string;
  fee_kobo: number;
  active: boolean;
  sort_order: number;
};

export const listDeliveryZones = () =>
  api.get<{ zones: AdminZone[] }>("/api/v1/admin/delivery/zones").then((r) => r.zones);

export const createZone = (input: { name: string; fee_kobo: number; sort_order: number }) =>
  api.post<{ zone: AdminZone }>("/api/v1/admin/delivery/zones", input).then((r) => r.zone);

export const updateZone = (id: string, input: Partial<{ name: string; fee_kobo: number; active: boolean; sort_order: number }>) =>
  api.patch<{ zone: AdminZone }>(`/api/v1/admin/delivery/zones/${id}`, input).then((r) => r.zone);

// ---------- Settings ----------

export type StoreSettings = {
  store_name: string;
  store_address: string;
  store_phone: string;
  store_email: string;
  instagram_url: string;
  tiktok_url: string;
  facebook_url: string;
  map_embed_url: string;
  opening_hours: Record<string, { open: string; close: string } | null>;
  temporarily_closed: boolean;
  closed_message: string;
  delivery_fee_kobo: number;
  online_enabled: boolean;
  bank_transfer_enabled: boolean;
  pay_on_pickup_enabled: boolean;
  pay_on_delivery_enabled: boolean;
  bank_name: string;
  bank_account_number: string;
  bank_account_name: string;
  whatsapp_number: string;
  whatsapp_message: string;
  notification_email: string;
};

export const getSettings = () =>
  api.get<{ settings: StoreSettings }>("/api/v1/admin/settings").then((r) => r.settings);

export const updateSettings = (input: Partial<StoreSettings>) =>
  api.patch<{ settings: StoreSettings }>("/api/v1/admin/settings", input).then((r) => r.settings);

// ---------- Audit log ----------

export type AuditEntry = {
  id: string;
  actor_id: string;
  actor_name: string;
  action: string;
  entity_type: string;
  entity_id: string;
  detail: string;
  created_at: string;
};

export type AuditPage = {
  entries: AuditEntry[];
  total: number;
  limit: number;
  offset: number;
};

export const searchAuditLog = (filters: Record<string, string | number | boolean | undefined> = {}) =>
  api.get<AuditPage>("/api/v1/admin/audit-logs", filters);

// ---------- Media library ----------

export type MediaItem = {
  id: string;
  url: string;
  alt_text: string;
  content_type: string;
  size_bytes: number;
  created_at: string;
};

export type MediaPage = {
  media: MediaItem[];
  total: number;
  limit: number;
  offset: number;
};

export const listMedia = (q?: string, limit = 50, offset = 0) =>
  api.get<MediaPage>("/api/v1/admin/media", { q, limit, offset });

export const uploadMedia = (file: File, alt?: string) => {
  const form = new FormData();
  form.append("file", file);
  if (alt) form.append("alt", alt);
  return api.post<{ media: MediaItem }>("/api/v1/admin/media", form).then((r) => r.media);
};

export const updateMediaAlt = (id: string, alt_text: string) =>
  api.patch<{ media: MediaItem }>(`/api/v1/admin/media/${id}`, { alt_text }).then((r) => r.media);

export const deleteMedia = (id: string) =>
  api.delete<void>(`/api/v1/admin/media/${id}`);

// ---------- Content (Phase B) ----------

export type ContentSection = {
  key: string;
  label: string;
  has_draft: boolean;
  published_at: string | null;
  updated_at: string;
};

export type ContentDetail = {
  key: string;
  label: string;
  published: Record<string, unknown> | null;
  draft: Record<string, unknown> | null;
  has_draft: boolean;
  published_at: string | null;
};

export type ContentVersion = {
  id: string;
  published_at: string;
  published_by: string;
  note: string;
};

export const listContent = () =>
  api.get<{ sections: ContentSection[] }>("/api/v1/admin/content").then((r) => r.sections);

export const getContentPreview = () =>
  api.get<Record<string, unknown>>("/api/v1/admin/content/preview");

export const getContentSection = (key: string) =>
  api.get<{ section: ContentDetail }>(`/api/v1/admin/content/${key}`).then((r) => r.section);

export const saveDraft = (key: string, data: Record<string, unknown>) =>
  api.put<{ section: ContentDetail }>(`/api/v1/admin/content/${key}/draft`, { data }).then((r) => r.section);

export const deleteDraft = (key: string) =>
  api.delete<void>(`/api/v1/admin/content/${key}/draft`);

export const publishSection = (key: string, note?: string) =>
  api.post<{ section: ContentDetail }>(`/api/v1/admin/content/${key}/publish`, { note }).then((r) => r.section);

export const rollbackSection = (key: string, version_id: string) =>
  api.post<{ section: ContentDetail }>(`/api/v1/admin/content/${key}/rollback`, { version_id }).then((r) => r.section);

export const listVersions = (key: string) =>
  api.get<{ versions: ContentVersion[] }>(`/api/v1/admin/content/${key}/versions`).then((r) => r.versions);

// ---------- Rider endpoints ----------

export type RiderDelivery = {
  id: string;
  order_id: string;
  order_number: string;
  status: string;
  address_line: string;
  area: string;
  customer_name: string;
  customer_phone: string;
  cash_to_collect_kobo?: number;
  created_at: string;
};

export const listRiderDeliveries = () =>
  api.get<{ deliveries: RiderDelivery[] }>("/api/v1/rider/deliveries").then((r) => r.deliveries);

export const updateRiderDelivery = (id: string, status: string, failure_reason?: string) =>
  api.patch<void>(`/api/v1/rider/deliveries/${id}`, { status, failure_reason });
