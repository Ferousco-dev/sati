"use client";

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from "react";
import { optionalStorage, useConsent } from "./consent";

export type MenuItem = {
  id: string;
  name: string;
  price: number;
  image: string;
  category: string;
  tags?: string[];
};

export type CartLine = MenuItem & { qty: number };

type State = {
  cart: CartLine[];
};

type Action =
  | { type: "add"; item: MenuItem }
  | { type: "remove"; id: string }
  | { type: "qty"; id: string; qty: number }
  | { type: "clearCart" }
  | { type: "reorder"; lines: CartLine[] }
  | { type: "hydrate"; state: State };

// Saved as "sati:v1", and only once the visitor has accepted optional storage.
const KEY = "v1";
const LEGACY_KEY = "orente:v1";
const empty: State = { cart: [] };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "hydrate":
      // Only the bag is kept. Older versions also saved names and orders here, which are dropped now.
      return { cart: Array.isArray(action.state?.cart) ? action.state.cart : [] };
    case "add": {
      const existing = state.cart.find((l) => l.id === action.item.id);
      const cart = existing
        ? state.cart.map((l) =>
            l.id === action.item.id ? { ...l, qty: l.qty + 1 } : l,
          )
        : [...state.cart, { ...action.item, qty: 1 }];
      return { ...state, cart };
    }
    case "remove":
      return { ...state, cart: state.cart.filter((l) => l.id !== action.id) };
    case "qty": {
      if (action.qty <= 0)
        return { ...state, cart: state.cart.filter((l) => l.id !== action.id) };
      return {
        ...state,
        cart: state.cart.map((l) =>
          l.id === action.id ? { ...l, qty: action.qty } : l,
        ),
      };
    }
    case "clearCart":
      return { ...state, cart: [] };
    case "reorder": {
      // Merge the order's lines into the current cart, summing quantities.
      let cart = state.cart.map((l) => ({ ...l }));
      for (const line of action.lines) {
        const existing = cart.find((l) => l.id === line.id);
        if (existing) existing.qty += line.qty;
        else cart = [...cart, { ...line }];
      }
      return { ...state, cart };
    }
    default:
      return state;
  }
}

type Ctx = {
  cart: CartLine[];
  count: number;
  total: number;
  add: (item: MenuItem) => void;
  addQuiet: (item: MenuItem) => void;
  remove: (id: string) => void;
  setQty: (id: string, qty: number) => void;
  clearCart: () => void;
  reorder: (lines: CartLine[]) => void;
  cartOpen: boolean;
  setCartOpen: (open: boolean) => void;
};

const StoreContext = createContext<Ctx | null>(null);

export function StoreProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(reducer, empty);
  const [cartOpen, setCartOpen] = useState(false);
  const [ready, setReady] = useState(false);
  const consent = useConsent();
  const hydrated = useRef(false);

  // The old demo stored its data under another name without asking. Remove it.
  useEffect(() => {
    try {
      localStorage.removeItem(LEGACY_KEY);
    } catch {}
  }, []);

  // Load a saved cart once, and only if the visitor already accepted storage.
  useEffect(() => {
    if (!consent.ready || hydrated.current) return;
    hydrated.current = true;
    try {
      const raw = optionalStorage.get(KEY);
      if (raw) dispatch({ type: "hydrate", state: JSON.parse(raw) });
    } catch {}
    setReady(true);
  }, [consent.ready]);

  // Without consent the cart lives in memory for this visit only.
  useEffect(() => {
    if (ready && consent.state === "accepted") {
      optionalStorage.set(KEY, JSON.stringify(state));
    }
  }, [state, ready, consent.state]);

  const total = useMemo(
    () => state.cart.reduce((sum, l) => sum + l.price * l.qty, 0),
    [state.cart],
  );
  const count = useMemo(
    () => state.cart.reduce((sum, l) => sum + l.qty, 0),
    [state.cart],
  );

  const value: Ctx = {
    cart: state.cart,
    count,
    total,
    add: (item) => {
      dispatch({ type: "add", item });
      setCartOpen(true);
    },
    addQuiet: (item) => dispatch({ type: "add", item }),
    remove: (id) => dispatch({ type: "remove", id }),
    setQty: (id, qty) => dispatch({ type: "qty", id, qty }),
    clearCart: () => dispatch({ type: "clearCart" }),
    reorder: (lines) => dispatch({ type: "reorder", lines }),
    cartOpen,
    setCartOpen,
  };

  return <StoreContext.Provider value={value}>{children}</StoreContext.Provider>;
}

export function useStore() {
  const ctx = useContext(StoreContext);
  if (!ctx) throw new Error("useStore must be used within StoreProvider");
  return ctx;
}
