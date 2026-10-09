"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "./api/client";
import type { MenuItem } from "./store";

type ApiProduct = {
  id: string;
  category_id: string;
  name: string;
  description: string;
  price_kobo: number;
  available: boolean;
  images: { url: string; position: number }[];
};

type ApiCategory = { id: string; name: string; sort_order: number };

/** A menu item from the real catalog. Prices are in naira, the unit the cart already uses. */
export type CatalogItem = MenuItem & {
  description: string;
  categoryId: string;
  /** False when the product is sold out. It cannot be added to the bag. */
  available: boolean;
};

export type CatalogCategory = { id: string; name: string };

type Catalog = { items: CatalogItem[]; categories: CatalogCategory[] };

const PAGE = 100;
// The API answers with its own cache headers and ETag, so repeat requests are cheap.
// This only stops the landing page and /menu from asking twice in one visit.
const FRESH_MS = 30_000;

let cached: { at: number; data: Catalog } | null = null;
let inflight: Promise<Catalog> | null = null;

async function fetchCatalog(): Promise<Catalog> {
  const [cats, products] = await Promise.all([
    api.get<{ categories: ApiCategory[] }>("/api/v1/categories"),
    (async () => {
      const all: ApiProduct[] = [];
      for (let offset = 0; ; offset += PAGE) {
        const page = await api.get<{ products: ApiProduct[]; total: number }>(
          "/api/v1/products",
          { limit: PAGE, offset },
        );
        all.push(...page.products);
        if (all.length >= page.total || page.products.length === 0) return all;
      }
    })(),
  ]);

  const categories = [...cats.categories]
    .sort((a, b) => a.sort_order - b.sort_order)
    .map((c) => ({ id: c.id, name: c.name }));
  const names = new Map(categories.map((c) => [c.id, c.name]));

  const items = products.map<CatalogItem>((p) => ({
    id: p.id,
    name: p.name,
    description: p.description,
    price: p.price_kobo / 100,
    image: [...p.images].sort((a, b) => a.position - b.position)[0]?.url ?? "",
    category: names.get(p.category_id) ?? "",
    categoryId: p.category_id,
    available: p.available,
  }));
  return { items, categories };
}

function load(force = false): Promise<Catalog> {
  if (!force && cached && Date.now() - cached.at < FRESH_MS) {
    return Promise.resolve(cached.data);
  }
  inflight ??= fetchCatalog()
    .then((data) => {
      cached = { at: Date.now(), data };
      return data;
    })
    .finally(() => {
      inflight = null;
    });
  return inflight;
}

/** The menu from the API, with loading and error states for the page to show. */
export function useCatalog() {
  const [data, setData] = useState<Catalog | null>(cached?.data ?? null);
  const [failed, setFailed] = useState(false);

  const reload = useCallback(() => {
    setFailed(false);
    load(true).then(setData, () => setFailed(true));
  }, []);

  useEffect(() => {
    let live = true;
    load().then(
      (d) => live && setData(d),
      () => live && setFailed(true),
    );
    return () => {
      live = false;
    };
  }, []);

  return {
    items: data?.items ?? [],
    categories: data?.categories ?? [],
    status: data ? ("ready" as const) : failed ? ("error" as const) : ("loading" as const),
    reload,
  };
}

/** What goes into the bag: only the fields the cart stores. */
export const toCartItem = ({ id, name, price, image, category }: CatalogItem): MenuItem => ({
  id,
  name,
  price,
  image,
  category,
});
