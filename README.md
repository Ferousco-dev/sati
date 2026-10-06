# SATI Yoghurt backend

The SATI Yoghurt backend is the Go API that powers the SATI Yoghurt ordering platform. It handles customer accounts, sessions, products, categories, carts, orders, payments, pickup and delivery, email notifications, and the admin system. The frontend only talks to this API for business operations, while PostgreSQL remains the source of truth for customers, orders, payments, and store data.

This repository is the `backend` branch of the SATI Yoghurt remote. The app is the sibling `frontend` clone.

## Stack

Go, PostgreSQL, pgx, Redis, REST API, Paystack for payments, Resend for transactional email, and Cloudflare R2 or equivalent object storage for product and store images.

The API is versioned under `/api/v1`.

The backend is a modular monolith. Business areas are separated into internal modules, but they run as one Go service and one deployable application.

## Setup

You need Go, PostgreSQL and Redis available locally.

```bash
git clone --branch backend --single-branch https://github.com/Ferousco-dev/sati.git backend
cd backend

cp .env.example .env

go mod download
go run ./cmd/api
```

Create the PostgreSQL role and database before starting the API, then run the migrations:

```bash
psql -d postgres -c "CREATE ROLE sati LOGIN PASSWORD 'sati'"
createdb -O sati sati
go run ./cmd/migrate up
```

Then configure `DATABASE_URL` in `.env`.

### Running with Docker (optional)

Docker is optional. `docker-compose.yml` starts PostgreSQL, Redis and a Caddy reverse proxy (`:8000`, routing `/api/*` to the API and everything else to the frontend) for anyone who prefers containers. The API itself still runs on your machine.

Stop any local PostgreSQL on port 5432 first, or the container cannot bind the port.

```bash
docker compose up -d
go run ./cmd/migrate up
go run ./cmd/api
```

Open `http://localhost:8000/api/v1/health` to test through the proxy, or `http://localhost:8080/api/v1/health` to hit the API directly.

Stop the containers with:

```bash
docker compose down
```

Add `-v` to also delete the database volume.

Redis is used for features such as rate limiting, caching and temporary state. The API can keep the core ordering system functional without Redis where a feature does not require it.

The development server listens on the configured `PORT`, with `8080` as the default.

## Architecture

```text
Customer / Admin
       |
       v
    Next.js
       |
       v
 Reverse Proxy
       |
       v
    Go API
       |
   +---+---------+----------------+
   |             |                |
   v             v                v
PostgreSQL     Redis         External APIs
                               |       |
                            Paystack  Resend
```

PostgreSQL is the primary source of truth.

Redis is not used as the permanent store for orders or payments.

The API is divided into business modules:

```text
auth
users
products
categories
cart
orders
payments
webhooks
delivery
notifications
email
store
admin
analytics
files
audit
```



## Authentication

SATI uses server-managed sessions instead of storing authentication tokens in browser local storage.

A successful login creates a random session identifier. The browser receives it through a secure HttpOnly cookie, while the backend stores a hash of the session identifier.

Passwords are hashed with Argon2id.

Sessions support:

- Login
- Logout
- Remember Me
- Session expiration
- Individual session revocation
- Multiple active sessions
- Account-level authentication checks

Remember Me does not use a separate authentication system. It creates a longer-lived session.

### Email verification

New customers are created with `email_verified=false`.

Email verification is intentionally not required before a customer can browse or place an order.

The registration flow is:

```text
Register
   |
   v
Create account
   |
   v
Hash password
   |
   v
Create verification token
   |
   v
Create session
   |
   v
Customer can continue using SATI
   |
   v
Verification email is sent
```

Verification tokens are:

- Single use
- Valid for 15 minutes
- Stored only as hashes
- Replaced when a new token is requested
- Rate limited

A new verification email can be requested after 60 seconds.

## Authentication endpoints


| Route                                   | What it does                                          |
| --------------------------------------- | ----------------------------------------------------- |
| `POST /api/v1/auth/register`            | Creates a customer account and starts a session.      |
| `POST /api/v1/auth/login`               | Authenticates a customer and creates a session.       |
| `POST /api/v1/auth/logout`              | Revokes the current session.                          |
| `GET /api/v1/auth/me`                   | Returns the currently authenticated customer.         |
| `POST /api/v1/auth/verify-email`        | Verifies an email address using a verification token. |
| `POST /api/v1/auth/resend-verification` | Creates and sends a new verification email.           |
| `POST /api/v1/auth/forgot-password`     | Starts the password reset flow.                       |
| `POST /api/v1/auth/reset-password`      | Sets a new password using a valid reset token.        |




## Roles

SATI has four primary roles:


| Role       | Access                                                                                          |
| ---------- | ----------------------------------------------------------------------------------------------- |
| `CUSTOMER` | Own account, cart, addresses and orders.                                                        |
| `STAFF`    | Store operations: view products and categories, change availability, manage orders.            |
| `ADMIN`    | Everything STAFF can do, plus catalog editing, users, roles and store settings.                |
| `RIDER`    | Only their own assigned deliveries and delivery status updates.                                 |

All four roles work from day one. Roles are assigned through the admin API, never by editing code:

| Route                                   | What it does                                                                 |
| --------------------------------------- | ---------------------------------------------------------------------------- |
| `GET /api/v1/admin/users`               | Lists users, optionally filtered with `?role=`.                              |
| `POST /api/v1/admin/users`              | Creates a STAFF, RIDER or ADMIN account and emails a link to set a password. |
| `PATCH /api/v1/admin/users/:id/role`    | Changes a user's role. Takes effect immediately. You cannot change your own. |

Invite links last 72 hours and use the same single-use token as password reset.

Authorization is checked on the backend for every protected resource. A frontend role check is never treated as sufficient authorization.

## Products and categories

Products belong to categories and contain the information required to display and order them.

A product can be:

- Active or inactive
- Available or unavailable
- Assigned to a category
- Given a price
- Given one or more images
- Archived without deleting historical order data

The API does not trust product prices sent by the frontend.

When an order is created, the backend loads the current product prices from PostgreSQL and calculates the totals itself.

## Product endpoints


| Route                                          | What it does                  |
| ---------------------------------------------- | ----------------------------- |
| `GET /api/v1/products`                         | Lists available products.     |
| `GET /api/v1/products/:id`                     | Returns one product.          |
| `GET /api/v1/categories`                       | Lists product categories.     |
| `GET /api/v1/categories/:id/products`          | Lists products in a category. |
| `POST /api/v1/admin/products`                  | Creates a product.            |
| `PATCH /api/v1/admin/products/:id`             | Updates a product.            |
| `POST /api/v1/admin/products/:id/archive`      | Archives a product.           |
| `POST /api/v1/admin/products/:id/availability` | Changes product availability. |
| `POST /api/v1/admin/products/:id/images`       | Uploads a JPEG, PNG or WebP image (max 5MB, form field `image`). |
| `DELETE /api/v1/admin/products/:id/images/:imageId` | Removes a product image. |




Prices are integers in kobo (`price_kobo`) with `currency` set to `NGN`. List endpoints take `limit` (1-100, default 20) and `offset` and return `total`.

The public product list shows active, non-archived products in active categories, with an `available` flag so the frontend can mark sold-out items.

Images use a storage interface. `STORAGE_DRIVER=local` writes to `UPLOAD_DIR` and serves files from `/api/v1/files/`. The R2 driver is not implemented yet.

### Admin account and sample data

```bash
ADMIN_PASSWORD='choose-a-password' go run ./cmd/admin create -email you@example.com -name "Your Name"
go run ./cmd/admin promote -email existing@example.com
go run ./cmd/seed
```

`cmd/seed` adds sample categories and products in development only. If `ADMIN_PASSWORD` is not set, the admin command reads the password from stdin.

## Speed, caching and CDN

Speed is a core goal of the project.

- Public catalog responses carry `ETag` and `Cache-Control: public, max-age=60, stale-while-revalidate=300`. Browsers, phones and CDNs reuse them, and a repeat request answers `304 Not Modified` with no body.
- Every other API response is `Cache-Control: no-store`, so personal data is never kept by a shared cache. Responses always send `Vary: Origin`.
- Image files are never overwritten, so they are served with `Cache-Control: public, max-age=31536000, immutable`. Errors are never cached.
- Compression (`zstd`, `gzip`) is handled by the proxy. `deploy/Caddyfile` enables it locally and Cloudflare does it in production.

To put images behind a CDN, set `ASSET_BASE_URL` to the CDN address. Cloudflare in front of the API origin will then cache `/api/v1/files/*` at the edge. When object storage is added, point the CDN at the bucket's custom domain in the same way. Resizing and WebP conversion happen at the edge or in the frontend image component, so the API only stores originals.

Storing data on a customer's phone beyond the session cookie needs consent. The frontend shows an accept button, stores the choice locally, and only caches data on the device after it is accepted.

## Cart and checkout

The customer can add products to a cart and change quantities before checkout.

The frontend can maintain the temporary cart state, but the backend is responsible for validating the cart during checkout.

The checkout request contains product IDs and quantities rather than trusting a client-provided subtotal or total.

The backend:

1. Loads the requested products.
2. Checks that each product exists.
3. Checks product availability.
4. Loads current prices.
5. Calculates item totals.
6. Calculates the subtotal.
7. Calculates delivery fees when applicable.
8. Applies valid discounts or promotions.
9. Calculates the final total.
10. Creates the order and order items inside a transaction.



## Orders

Orders are independent from payments.

An order records what the customer purchased, the price at the time of purchase, the selected fulfillment method, and the current order state.

Order items store snapshots of the product name and unit price so historical orders do not change when a product is renamed or repriced.

### Order states

```text
PENDING
   |
   v
CONFIRMED
   |
   v
PREPARING
   |
   v
READY
   |
   +--------------------+
   |                    |
   v                    v
PICKUP              OUT_FOR_DELIVERY
   |                    |
   v                    v
COMPLETED            DELIVERED
```

Orders can also move to exceptional states such as:

```text
CANCELLED
FAILED
```

Payment state is stored separately from order state.

### Payment states

```text
UNPAID
PENDING
PAID
FAILED
REFUNDED
PARTIALLY_REFUNDED
```

This prevents payment status from being incorrectly inferred from the order status.

## Order endpoints


| Route                                   | What it does                                                         |
| --------------------------------------- | -------------------------------------------------------------------- |
| `POST /api/v1/orders`                   | Creates an order after validating the cart and checkout information. |
| `GET /api/v1/orders`                    | Lists the authenticated customer's orders.                           |
| `GET /api/v1/orders/:id`                | Returns an order owned by the authenticated customer.                |
| `POST /api/v1/orders/:id/cancel`        | Cancels an eligible order.                                           |
| `GET /api/v1/orders/:id/status`         | Returns the current order status.                                    |
| `GET /api/v1/admin/orders`              | Lists store orders for authorized staff.                             |
| `GET /api/v1/admin/orders/:id`          | Returns an order for staff management.                               |
| `PATCH /api/v1/admin/orders/:id/status` | Updates an order's preparation or fulfillment status.                |


Customers cannot access another customer's order by changing an order ID in a request. Another customer's order returns `404`, never `403`, so its existence is not revealed.

Checkout request:

```json
{
  "items": [{ "product_id": "...", "quantity": 2 }],
  "fulfillment": { "type": "PICKUP" },
  "payment_method": "ONLINE",
  "notes": "optional"
}
```

- `fulfillment.type` is `PICKUP` or `DELIVERY`. Delivery needs `address_id` and uses the flat `delivery_fee_kobo` from store settings until delivery zones exist.
- `payment_method` is one of `ONLINE`, `BANK_TRANSFER`, `PAY_ON_PICKUP` or `PAY_ON_DELIVERY`. Pickup orders can use the first three, delivery orders the first two plus `PAY_ON_DELIVERY`.
- Sending any price or total is rejected. The same product on several lines is merged. An order holds up to 50 lines and 50 of each product.
- Send an `Idempotency-Key` header to make retries safe. A repeated key returns the original order with `200` instead of creating another.
- The order reference is `order_number`, like `SATI-10042`. The UUID `id` is used in URLs.
- A customer can cancel only while the order is `PENDING`.
- A saved delivery address is copied into the order, so editing or deleting the address never changes order history.
- Money fields end in `_kobo` and are integers.

## Addresses

Customers can save delivery addresses for future orders.

An address can contain:

- Recipient name
- Phone number
- Address line
- Area
- City
- Additional delivery instructions

The backend validates that an address belongs to the authenticated customer before using it for an order.

Address endpoints require a signed-in customer and only ever touch that customer's own addresses (up to 10):

| Route                            | What it does                  |
| -------------------------------- | ----------------------------- |
| `GET /api/v1/addresses`          | Lists saved addresses.        |
| `POST /api/v1/addresses`         | Saves an address.             |
| `PATCH /api/v1/addresses/:id`    | Updates a saved address.      |
| `DELETE /api/v1/addresses/:id`   | Deletes a saved address.      |

### Payment methods and bank transfer

`GET /api/v1/checkout/options` is public and cached. It returns the delivery fee and the payment methods currently available for `PICKUP` and `DELIVERY`, so the checkout page never offers a method that is switched off.

Bank transfer orders include `payment_instructions` while they are `PENDING` and `UNPAID`:

```json
{
  "method": "BANK_TRANSFER",
  "bank_name": "...",
  "account_number": "...",
  "account_name": "...",
  "reference": "SATI-10042",
  "amount_kobo": 750000,
  "whatsapp_url": "https://wa.me/234...?text=..."
}
```

The customer pays to the account using the order number as the reference. The frontend can send them to `whatsapp_url`, which opens WhatsApp with a prefilled message. `whatsapp_url` is left out when no WhatsApp number is set.

Cash and bank transfer orders stay `PENDING` and `UNPAID` until staff confirm them and record the payment.

### Store settings

Everything below is changed through the admin API, with no code edits: `GET /api/v1/admin/settings` (STAFF and ADMIN) and `PATCH /api/v1/admin/settings` (ADMIN).

| Field | Purpose |
| --- | --- |
| `delivery_fee_kobo` | Flat delivery fee. |
| `online_enabled`, `bank_transfer_enabled`, `pay_on_pickup_enabled`, `pay_on_delivery_enabled` | Switch each payment method on or off. All are on by default. |
| `bank_name`, `bank_account_number`, `bank_account_name` | The store's bank account. Bank transfer is only offered once all three are set. |
| `whatsapp_number` | International number, digits only (a leading `+` and spaces are accepted). |
| `whatsapp_message` | Message template. `{order_number}` and `{total}` are replaced. |

Orders using a method that is switched off are rejected with `PAYMENT_METHOD_DISABLED`. Turn `online_enabled` off until Paystack is connected.

## Delivery and pickup

SATI supports both pickup and delivery.

### Pickup

```text
Order confirmed
      |
      v
Preparing
      |
      v
Ready for pickup
      |
      v
Customer collects
      |
      v
Completed
```



### Delivery

```text
Order confirmed
      |
      v
Preparing
      |
      v
Ready
      |
      v
Rider assigned
      |
      v
Picked up
      |
      v
Out for delivery
      |
      v
Delivered
```

The initial delivery system supports store-managed delivery with manual rider assignment from the admin dashboard.

### Zones and fees

Admins create delivery zones, each with a name and a fee in kobo. A customer picks a zone on the address (`zone_id`), and a delivery order uses that zone's fee. The price is read inside the order transaction, so a fee change never affects an order already placed. If the store has no active zones yet, the flat `delivery_fee_kobo` from settings is used instead. Once zones exist, a delivery address without a zone is rejected with `ADDRESS_ZONE_REQUIRED`.

`GET /api/v1/checkout/options` also returns the active zones, so checkout needs one request.

### Riders

- STAFF or ADMIN assigns a rider to a delivery order once it is `CONFIRMED`, until it goes out. Reassigning is allowed until the rider picks it up.
- The rider sees only their own jobs, with the address, the customer name and how much cash to collect.
- The rider drives the order: `PICKED_UP` moves the order to `OUT_FOR_DELIVERY` (it must be `READY`), and `DELIVERED` moves it to `DELIVERED`. Cash on delivery is recorded as collected by that rider.
- If staff move an order by hand, the delivery record follows. Sending an order out needs an assigned rider.
- A rider reports `FAILED` with a reason. The order stays `OUT_FOR_DELIVERY` and the delivery shows `FAILED`. Staff then either reassign it (the order returns to `READY` for another attempt) or fail or cancel the order with a reason.

## Delivery endpoints

| Route                                       | Who          | What it does                                           |
| ------------------------------------------- | ------------ | ------------------------------------------------------ |
| `GET /api/v1/delivery/zones`                | public       | Lists active zones with fees. Cached.                  |
| `GET /api/v1/admin/delivery/zones`          | STAFF, ADMIN | Lists all zones.                                       |
| `POST /api/v1/admin/delivery/zones`         | ADMIN        | Creates a zone.                                        |
| `PATCH /api/v1/admin/delivery/zones/:id`    | ADMIN        | Renames, reprices or switches a zone on or off.        |
| `GET /api/v1/admin/riders`                  | STAFF, ADMIN | Lists users with the RIDER role.                       |
| `POST /api/v1/admin/orders/:id/rider`       | STAFF, ADMIN | Assigns or reassigns a rider.                          |
| `GET /api/v1/admin/delivery`                | STAFF, ADMIN | Lists deliveries. Filters: `status`, `rider_id`.       |
| `GET /api/v1/rider/deliveries`              | RIDER        | The rider's active jobs. `?status=all` adds finished ones. |
| `PATCH /api/v1/rider/deliveries/:id/status` | RIDER        | Moves their own delivery to `PICKED_UP`, `DELIVERED` or `FAILED`. |

## Payments

SATI uses Paystack for customer payments.

Payment logic is isolated behind a provider interface so another payment provider can be added without rewriting the order system.

```go
type PaymentProvider interface {
    InitializePayment(...)
    VerifyPayment(...)
    HandleWebhook(...)
}
```

The payment flow is:

```text
Customer
   |
   v
Next.js
   |
   v
Go API
   |
   v
Create order/payment
   |
   v
Paystack
   |
   v
Customer pays
   |
   v
Paystack webhook
   |
   v
Go API
   |
   +--> Verify webhook
   |
   +--> Verify transaction
   |
   +--> Update payment
   |
   +--> Update order
   |
   +--> Notify customer
```

The frontend is never trusted to decide whether a payment succeeded.

The backend verifies the transaction with Paystack before marking the payment as successful.

## Payment endpoints


| Route                              | What it does                                         |
| ---------------------------------- | ---------------------------------------------------- |
| `POST /api/v1/payments/initialize` | Initializes a payment for an eligible order.         |
| `GET /api/v1/payments/:reference`  | Returns payment information for an authorized order. |
| `POST /api/v1/payments/verify`     | Verifies a payment with the payment provider.        |
| `POST /api/v1/webhooks/paystack`   | Receives and processes Paystack webhook events.      |
| `POST /api/v1/admin/payments/:id/refunded` | Records that a paid payment was refunded (ADMIN). |


Webhook processing is idempotent. Receiving the same event more than once must not create another payment or duplicate an order state transition.

Payment secrets are only available to the backend.

### How online payment works

1. The customer places an `ONLINE` order, then the frontend calls `POST /api/v1/payments/initialize` with `{ "order_id": "..." }`. The response has a `reference` and an `authorization_url`. Redirect the customer to that URL.
2. After paying, Paystack sends the customer back to `FRONTEND_URL/orders/:id`. The page calls `POST /api/v1/payments/verify` with the `reference` to show the result straight away. Paystack also calls `POST /api/v1/webhooks/paystack` on its own, so an order is confirmed even if the customer closes the browser.
3. The server never believes the browser or the webhook body. Both ask Paystack directly what happened to the reference, and the paid amount and currency must match the order exactly.
4. A successful payment sets the payment `PAID`, the order `PAID` and the order `CONFIRMED` in one transaction, then queues the receipt email.

Rules the code enforces:

- Every attempt gets its own reference (`SATI-10042-A1B2C3D4E5`), so a customer can retry after a failed or abandoned payment. An order allows up to 10 attempts, and only one payment can ever settle it.
- Webhooks are checked with the HMAC-SHA512 signature in `X-Paystack-Signature`. The secret is `PAYSTACK_WEBHOOK_SECRET`, or `PAYSTACK_SECRET_KEY` when that is not set. A bad signature gets `401` and changes nothing.
- Each event is stored in `payment_events` and handled once. Paystack can repeat an event, and a webhook and a verify call can arrive together, without ever confirming an order twice. If something fails, the webhook answers `500` so Paystack tries again.
- An unpaid online order is cancelled after `PAYMENT_EXPIRY_MINUTES` (default 30, `0` turns it off) with the reason "Payment not completed in time", and the customer is told.
- Money that arrives when the order cannot use it (it was cancelled or expired, a second attempt also paid, or the amount was wrong) is recorded and marked `needs_refund`. Cancelling an order that was already paid marks it the same way. Staff see these with `GET /api/v1/admin/payments?needs_refund=true`, refund the customer in the Paystack dashboard, then call `POST /api/v1/admin/payments/:id/refunded` (ADMIN) to record it.
- `ONLINE` is only offered at checkout when `PAYSTACK_SECRET_KEY` is set, so an order is never created that nobody can pay.
- Without a secret key the payment routes and the webhook are not registered at all.

To test locally, use Paystack test keys and expose the API with a tunnel, then set the webhook URL in the Paystack dashboard to `https://<your-tunnel>/api/v1/webhooks/paystack`.


## Database

PostgreSQL stores the application's durable state.

The main tables are:

```text
users
sessions
email_verification_tokens
password_reset_tokens

categories
products

addresses

orders
order_items

payments
payment_events

delivery_zones
delivery_orders

notifications
store_settings
audit_logs
```

Important constraints and indexes are defined at the database level.

Orders, payments and related state changes use transactions where atomicity is required.

The database is accessed through pgx and explicit SQL rather than an ORM.

## Redis

Redis is used only where temporary or high-speed state provides a clear benefit.

Current use cases include:

- Rate limiting
- Product/catalog caching
- Temporary locks
- Short-lived state
- Background job coordination

Redis is not the source of truth for orders, payments, customers or balances.

If Redis becomes unavailable, features that do not depend on it should continue operating normally.

## Notifications

SATI sends transactional emails through Resend and shows the same events inside the app.

An email is never sent during the request that causes it. The event is saved to an `email_jobs` table in the same database transaction as the order or account change, and a background worker sends it. The worker runs inside the API process (`EMAIL_WORKER_ENABLED=false` turns it off) and several instances can run at once without sending anything twice. If Resend is down the order is unaffected: sends are retried 5 times with growing delays (30 seconds, 2 minutes, 8 minutes, 32 minutes), and addresses Resend rejects are not retried. A failure to queue a notification is logged and never fails an order.

Sent jobs have their content wiped, because it can contain sign-in links, and old jobs are deleted after 14 days (30 for failed ones).

Without `RESEND_API_KEY` and `EMAIL_FROM` the emails are written to the log instead, with their text outside production. Links in emails use `FRONTEND_URL`: `/verify-email?token=`, `/reset-password?token=` (also used by staff invites), `/orders/:id`, and `/admin/orders/:id` for the store alert.

| Event | Email | In the app |
| --- | --- | --- |
| Registration | Verify your email | |
| Forgot password | Reset your password | |
| Staff invite | Set your password | |
| Order placed | Confirmation with items, total and bank details when relevant | yes |
| Order placed | Alert to `notification_email`, if set in settings | |
| Payment recorded | Payment received (one email, also says the order is confirmed when it was) | yes |
| Confirmed | Order confirmed | yes |
| Preparing | | yes |
| Ready | Ready for pickup (pickup orders only) | yes |
| Out for delivery | On its way | yes |
| Delivered or completed | Delivered or completed | yes |
| Cancelled or failed | The reason is included | yes |

In-app notifications are for signed-in users and are never cached:

| Route                                    | What it does                              |
| ---------------------------------------- | ----------------------------------------- |
| `GET /api/v1/notifications`              | Lists notifications. `?unread=true` filters. Includes `unread_count`. |
| `GET /api/v1/notifications/unread-count` | Cheap count for a bell icon.              |
| `POST /api/v1/notifications/:id/read`    | Marks one as read.                        |
| `POST /api/v1/notifications/read-all`    | Marks everything as read.                 |

## Admin

The admin API provides store operations for authorized staff.

Administrators can:

- Create and edit products
- Manage categories
- Change product availability
- View customers
- View orders
- Update order status
- Manage delivery zones
- Assign riders
- View payment status
- Manage store settings
- View basic operational statistics

Admin endpoints require an authenticated staff or admin session and the required role.

## Admin endpoints


| Route                                         | Who            | What it does                                                          |
| --------------------------------------------- | -------------- | --------------------------------------------------------------------- |
| `GET /api/v1/admin/dashboard`                 | STAFF, ADMIN   | Today's orders, revenue and the open order queue.                     |
| `GET /api/v1/admin/orders`                    | STAFF, ADMIN   | Lists orders. Filters: `status`, `payment_status`, `payment_method`, `fulfillment`, `q`, `from`, `to`. |
| `GET /api/v1/admin/orders/:id`                | STAFF, ADMIN   | Full order with customer, items and payments.                         |
| `PATCH /api/v1/admin/orders/:id/status`       | STAFF, ADMIN   | Moves an order to its next status.                                    |
| `POST /api/v1/admin/orders/:id/payment`       | STAFF, ADMIN   | Records a cash or bank transfer payment as received.                  |
| `GET /api/v1/admin/payments`                  | STAFF, ADMIN   | Lists payment records.                                                |
| `GET /api/v1/admin/customers`                 | ADMIN          | Lists customers with order counts and total spent.                    |
| `GET/POST /api/v1/admin/users`, `PATCH /api/v1/admin/users/:id/role` | ADMIN | Users and roles.                                       |
| `GET/PATCH /api/v1/admin/settings`            | STAFF read, ADMIN write | Delivery fee, payment methods, bank account, WhatsApp.       |
| `GET /api/v1/admin/products`, `GET /api/v1/admin/categories` | STAFF, ADMIN | Catalog for management.                                |

### Working orders

`PATCH /api/v1/admin/orders/:id/status` takes `{ "status": "...", "reason": "..." }` and enforces the transitions:

```text
PENDING -> CONFIRMED -> PREPARING -> READY -> COMPLETED          (pickup)
                                      READY -> OUT_FOR_DELIVERY -> DELIVERED   (delivery)
CANCELLED: from PENDING, CONFIRMED, PREPARING or READY
FAILED:    from PENDING or OUT_FOR_DELIVERY
```

- `CANCELLED` and `FAILED` need a `reason` (max 200 characters). The customer sees it as `status_reason`.
- `ONLINE` and `BANK_TRANSFER` orders cannot be confirmed until they are paid.
- Cash orders (`PAY_ON_PICKUP`, `PAY_ON_DELIVERY`) are recorded as paid automatically when they reach `COMPLETED` or `DELIVERED`, credited to the staff member who made the change.
- `POST /api/v1/admin/orders/:id/payment` records a payment seen by hand, with an optional `note`. For a pending bank transfer order it also confirms the order. A second payment on the same order returns `ALREADY_PAID`, even when two staff click at once.
- Every manual payment is stored in the `payments` table with who recorded it and when.

### Dashboard

"Today" starts at midnight in the store timezone (`STORE_TIMEZONE`, default `Africa/Lagos`). Pass `?tz=Europe/London` with the viewer's own timezone, for example from `Intl.DateTimeFormat().resolvedOptions().timeZone` in the browser, to see their own day instead. All timestamps in the API are UTC, so the frontend shows them in the customer's local time.

## Files and images

Product and store images are stored outside PostgreSQL.

The backend generates or manages the object storage references while PostgreSQL stores the metadata required by the application.

The database does not store large binary image files directly.

The frontend receives URLs or signed URLs where appropriate.

## API errors

API responses use a consistent error structure.

```json
{
  "error": {
    "code": "ORDER_NOT_FOUND",
    "message": "Order not found"
  }
}
```

Error codes are stable and can be consumed by the frontend.

The API does not expose internal database errors, stack traces or provider secrets to clients.

## Environment

See `.env.example`.

The main environment variables are:

```env
APP_ENV=development
PORT=8080

DATABASE_URL=postgres://...

REDIS_URL=redis://...

SESSION_SECRET=...

PAYSTACK_SECRET_KEY=...
PAYSTACK_WEBHOOK_SECRET=...

RESEND_API_KEY=...
EMAIL_FROM=...

R2_ACCOUNT_ID=...
R2_ACCESS_KEY_ID=...
R2_SECRET_ACCESS_KEY=...
R2_BUCKET=...
R2_PUBLIC_URL=...
```

Production secrets must never be committed to the repository.

The application should refuse to start in production when required secrets are missing.

## Rate limiting

Authentication, payment, webhook and other sensitive endpoints are rate limited.

Rate limits are applied per client IP and, where appropriate, per authenticated account.

Important routes include:

- Registration
- Login
- Password reset
- Email verification
- Payment initialization
- Payment verification
- Admin authentication
- Public API endpoints

The backend must correctly identify the client IP when running behind the configured reverse proxy.

## Security

The API follows several security rules:

- Passwords use Argon2id.
- Sessions use secure HttpOnly cookies.
- Cookies use `Secure` in production.
- SameSite and CSRF protection are configured for the deployment architecture.
- SQL queries are parameterized.
- User input is validated before reaching business logic.
- Authorization is checked server-side.
- Payment webhooks are verified.
- Payment secrets never reach the frontend.
- Session identifiers are never stored as plaintext in the database.
- Sensitive values are excluded from logs.
- Admin actions are recorded in audit logs.
- Rate limits protect sensitive endpoints.



## Audit logs

Important administrative and security actions are recorded.

Examples include:

- Product creation
- Product updates
- Product archival
- Order status changes
- Rider assignment
- Payment actions
- Store setting changes
- Administrative authentication events

Audit logs provide an operational history without exposing sensitive authentication credentials.

## Health

```text
GET /api/v1/health
```

The health endpoint checks that the API is running and that required infrastructure is available according to the configured health policy.

The deployment platform uses this endpoint to determine whether the service is ready to receive traffic.

## Scripts


| Command               | What it does                       |
| --------------------- | ---------------------------------- |
| `go run ./cmd/api`    | Starts the development API.        |
| `go build ./...`      | Builds all Go packages.            |
| `go test ./...`       | Runs the test suite.               |
| `go test -race ./...` | Runs tests with the race detector. |
| `go vet ./...`        | Runs Go's static analysis checks.  |
| `gofmt -w .`          | Formats Go source files.           |


Database migration commands depend on the migration tool selected for the project and are documented alongside the migration package.

## Testing

The backend tests the important business boundaries rather than only HTTP handlers.

Tests cover:

- Registration
- Login and logout
- Session expiration
- Remember Me
- Email verification
- Password reset
- Product availability
- Cart validation
- Order totals
- Delivery fees
- Order status transitions
- Customer authorization
- Admin authorization
- Payment initialization
- Payment verification
- Invalid payment webhooks
- Duplicate payment webhooks
- Order/payment consistency
- Rider assignment
- Delivery status transitions

Integration tests use a dedicated PostgreSQL test database. They are skipped unless `TEST_DATABASE_URL` is set:

```bash
createdb -O sati sati_test
TEST_DATABASE_URL="postgres://sati:sati@localhost:5432/sati_test?sslmode=disable" go test -p 1 ./...
```

The `-p 1` flag runs packages one at a time because they share the test database. The tests truncate the users table, so never point `TEST_DATABASE_URL` at the development database.

Payment provider calls should use test credentials or mocked provider implementations during automated tests.

## Project structure

```text
backend/
├── cmd/
│   └── api/
│       └── main.go
│
├── internal/
│   ├── auth/
│   ├── users/
│   ├── products/
│   ├── categories/
│   ├── cart/
│   ├── orders/
│   ├── payments/
│   ├── webhooks/
│   ├── delivery/
│   ├── notifications/
│   ├── email/
│   ├── store/
│   ├── admin/
│   ├── analytics/
│   ├── files/
│   └── audit/
│
├── db/
│   ├── migrations/
│   └── queries/
│
├── pkg/
│
├── .env.example
├── go.mod
└── README.md
```

Each module owns its business logic and exposes only the interfaces other modules actually need.

HTTP handlers should remain thin. Validation, database operations and business rules belong in the appropriate application modules.

## Deployment

The production API run             |
                            v
                     Reverse Proxy
                            |
                            v
                         Go API
                            |
             +--------------+--------------+
             |              |              |
             v              v              v
        PostgreSQL        Redis        External APIs
                                          |
                                   +------+------+
                                   |             |
                                Paystack       Resend
```

The frontend is deployed separately and communicates with the backend through the configured API origin.

The backend can run on Render or another VPS/container platform without changing the application architecture.

Production configuration must provide:

- PostgreSQL connection
- Redis connection
- Session secret
- Paystack credentials
- Resend credentials
- Object storage credentials
- Correct proxy configuration
- Production CORS configuration
- Secure cookie configuration

The application must not contain production secrets in source control.

## Development principles

The backend follows a few rules:

1. PostgreSQL is the source of truth.
2. Never trust prices or payment status from the frontend.
3. Orders and payments are separate concepts.
4. Payment webhooks must be idempotent.
5. Authentication and authorization are different concerns.
6. Business logic belongs in backend modules, not HTTP handlers.
7. Redis is optional infrastructure, not the primary database.
8. Historical orders must remain accurate after product changes.
9. Sensitive information must not appear in logs.
10. New infrastructure should only be introduced when the product actually needs it.


