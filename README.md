# SATI Yoghurt frontend

The SATI Yoghurt frontend is the Next.js website that customers use to browse the menu, build a cart, place an order and follow it. The admin dashboard will live in the same app later.

This repository is the `frontend` branch of the SATI Yoghurt remote. The API is the sibling `backend` clone.

## Current state

The site is a working front-end demo. It is not connected to the backend yet.

- The menu is a static list in `src/lib/menu.ts`.
- The cart, the "account" (a name and an email, with no password) and past orders are kept in the browser's `localStorage` under the key `orente:v1`.
- Order tracking is simulated. An order moves from placed to delivered by comparing the time it was placed with the clock (`src/lib/order-status.ts`), with a two minute timeline.
- Copy, colours and images still carry the earlier "SATI-L Yoghurt & confectionery " branding and need to be replaced with SATI Yoghurt.
- The fonts `Vanguard` and `Instrument Sans` are named in `src/app/tokens.css` but are not loaded yet, so the browser falls back to Arial. Only `Permanent Marker` is loaded, from Google Fonts in `src/app/layout.tsx`.

The plan for replacing the mock data with the real API is described under [Connecting to the backend](#connecting-to-the-backend).

## Stack

Next.js 15 with the App Router, React 19, TypeScript in strict mode, GSAP with ScrollTrigger for animation, and Lenis for smooth scrolling. Styling is plain CSS: shared design tokens in `src/app/tokens.css` and one stylesheet next to each component. There is no CSS framework.

There are no tests and no linter configured yet.

## Setup

You need Node.js 18.18 or newer (the project is developed on Node 22) and npm.

```bash
git clone --branch frontend --single-branch https://github.com/Ferousco-dev/sati.git frontend
cd frontend

npm install
npm run dev
```

The development server listens on `http://localhost:3000`.

To try the production build:

```bash
npm run build
npm run start
```

## Scripts

| Command         | What it does                           |
| --------------- | -------------------------------------- |
| `npm run dev`   | Starts the development server.         |
| `npm run build` | Creates the production build.          |
| `npm run start` | Serves the production build.           |

## Pages

| Route      | What it is                                                                 |
| ---------- | -------------------------------------------------------------------------- |
| `/`        | Landing page: hero, product showcase, catering, menu and footer.           |
| `/order`   | Checkout. Asks for a name and email, then places the order and plays a success animation. |
| `/account` | Order history with a live tracker for each order and a reorder button.     |

The cart is a drawer that opens from any page.

## Project structure

```text
frontend/
├── public/
│   └── images/
│       ├── hero/
│       ├── locations/
│       ├── menu/
│       └── products/
│
├── src/
│   ├── app/
│   │   ├── layout.tsx        Root layout, fonts, providers
│   │   ├── page.tsx          Landing page
│   │   ├── globals.css
│   │   ├── tokens.css        Colours, spacing and fonts
│   │   ├── order/            Checkout page
│   │   └── account/          Order history page
│   │
│   ├── components/           One component and one stylesheet per section
│   │   ├── Navbar, Hero, ProductShowcase, Catering, OurMenu, Footer
│   │   ├── CartDrawer, OrderSuccess, OrderTracker
│   │   └── SmoothScroll, Magnetic
│   │
│   └── lib/
│       ├── store.tsx         Cart, account and orders state
│       ├── menu.ts           Static menu and the naira formatter
│       ├── order-status.ts   Simulated order timeline
│       └── use*.ts           Animation hooks (reveal, parallax, split text, magnetic)
│
├── next.config.ts
├── tsconfig.json
├── package.json
└── README.md
```

Imports use the `@/` alias for `src/`.

## State

`StoreProvider` in `src/lib/store.tsx` wraps the whole app. It holds the cart, the account and the orders in a reducer and saves them to `localStorage` after the first load, so a refresh keeps the cart.

Prices are whole naira numbers and are formatted with `naira()` from `src/lib/menu.ts`.

## Connecting to the backend

The backend is a Go API under `/api/v1`. Its README lists every route. When the mock data is replaced, these rules from the backend apply:

- **Sessions use cookies.** Login sets an HttpOnly session cookie, so the token is never stored in `localStorage`. Every request to the API must send cookies (`credentials: "include"` with `fetch`).
- **Same origin in development.** The backend's `deploy/Caddyfile` runs a proxy on `http://localhost:8000` that sends `/api/*` to the API and everything else to this app. Browse the site through port 8000 and the cookie works without any extra setup. If you use port 3000 directly, the API must list that origin in `CORS_ALLOWED_ORIGINS` (it already allows `http://localhost:3000`).
- **Never send prices.** Checkout sends product IDs and quantities only. The server calculates the totals, and `price_kobo` values from the API are integers in kobo. Divide by 100 for naira.
- **The menu comes from the API.** `GET /api/v1/products` and `GET /api/v1/categories` replace `src/lib/menu.ts`. Image URLs come back ready to use.
- **Real accounts.** Register, login, logout and `GET /api/v1/auth/me` replace the name and email "account". Verification is optional and never blocks ordering.
- **Real order status.** `GET /api/v1/orders/:id/status` replaces the time-based simulation in `order-status.ts`. Statuses are `PENDING`, `CONFIRMED`, `PREPARING`, `READY`, `OUT_FOR_DELIVERY`, `DELIVERED`, `COMPLETED`, `CANCELLED` and `FAILED`. Payment status is separate.
- **Checkout options.** `GET /api/v1/checkout/options` returns which payment methods are available for pickup and delivery, plus the delivery zones and fees. Bank transfer orders include the account details and a WhatsApp link to show after the order is placed.
- **Retries are safe.** Send an `Idempotency-Key` header when placing an order so a double click or a retry cannot create two orders.
- **Errors have stable codes.** Every error looks like `{"error":{"code":"...","message":"..."}}`. Show the message and branch on the code.

## Speed

Speed is a main goal of the project.

- Public catalog responses from the API are cacheable, so repeat visits can be served from the browser cache.
- Optional data may only be stored on a customer's phone after they accept it. The site needs a button to accept this, and nothing optional is stored before it is pressed. The session cookie does not need consent.
- Images are served through a CDN in production.
- Use `next/image` for new images so they are resized and served as modern formats.

## Environment

There are no environment variables yet. When the API is connected the app will read one:

```env
NEXT_PUBLIC_API_URL=
```

Leave it empty to call the API on the same origin through the proxy. `.env*` files are ignored by git, so nothing secret is ever committed. Anything starting with `NEXT_PUBLIC_` is visible in the browser, so never put a secret in it.

## Deployment

The frontend is deployed separately from the backend. The system design recommends Vercel. The browser reaches the API through the configured proxy or API origin. Production needs the final site address added to the backend's `FRONTEND_URL` and `CORS_ALLOWED_ORIGINS`.

## Development principles

1. The backend is the source of truth for prices, payment status and order status.
2. Never store the session in `localStorage`.
3. Keep pages fast: small bundles, optimised images, cached reads.
4. One component per file, with its stylesheet beside it.
5. Keep logic in `src/lib` and out of the components.
