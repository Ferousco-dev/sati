"use client";

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useState,
} from "react";

export type MenuItem = {
  id: string;
  name: string;
  price: number;
  image: string;
  category: string;
  tags?: string[];
};

export type CartLine = MenuItem & { qty: number };

export type Order = {
  id: string;
  placedAt: number;
  lines: CartLine[];
  total: number;
};

export type Account = { name: string; email: string };

type State = {
  cart: CartLine[];
  account: Account | null;
  orders: Order[];
};

type Action =
  | { type: "add"; item: MenuItem }
  | { type: "remove"; id: string }
  | { type: "qty"; id: string; qty: number }
  | { type: "clearCart" }
  | { type: "setAccount"; account: Account }
  | { type: "signOut" }
  | { type: "placeOrder"; order: Order }
  | { type: "reorder"; lines: CartLine[] }
  | { type: "hydrate"; state: State };

const KEY = "orente:v1";
const empty: State = { cart: [], account: null, orders: [] };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "hydrate":
      return action.state;
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
    case "setAccount":
      return { ...state, account: action.account };
    case "signOut":
      return { ...state, account: null };
    case "placeOrder":
      return { ...state, orders: [action.order, ...state.orders], cart: [] };
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
  account: Account | null;
  orders: Order[];
  count: number;
  total: number;
  add: (item: MenuItem) => void;
  addQuiet: (item: MenuItem) => void;
  remove: (id: string) => void;
  setQty: (id: string, qty: number) => void;
  clearCart: () => void;
  setAccount: (account: Account) => void;
  signOut: () => void;
  placeOrder: () => Order | null;
  reorder: (lines: CartLine[]) => void;
  cartOpen: boolean;
  setCartOpen: (open: boolean) => void;
};

const StoreContext = createContext<Ctx | null>(null);

export function StoreProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(reducer, empty);
  const [cartOpen, setCartOpen] = useState(false);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    try {
      const raw = localStorage.getItem(KEY);
      if (raw) dispatch({ type: "hydrate", state: JSON.parse(raw) });
    } catch {}
    setReady(true);
  }, []);

  useEffect(() => {
    if (ready) localStorage.setItem(KEY, JSON.stringify(state));
  }, [state, ready]);

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
    account: state.account,
    orders: state.orders,
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
    setAccount: (account) => dispatch({ type: "setAccount", account }),
    signOut: () => dispatch({ type: "signOut" }),
    placeOrder: () => {
      if (!state.cart.length) return null;
      const order: Order = {
        id: Math.random().toString(36).slice(2, 8).toUpperCase(),
        placedAt: Date.now(),
        lines: state.cart,
        total,
      };
      dispatch({ type: "placeOrder", order });
      return order;
    },
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
