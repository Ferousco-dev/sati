"use client";

import { useMemo, useState } from "react";
import { useReveal } from "@/lib/useReveal";
import { useStore } from "@/lib/store";
import { naira } from "@/lib/menu";
import { useCatalog, toCartItem, type CatalogItem } from "@/lib/catalog";
import { fallbackMenuSlides } from "@/lib/fallbacks";
import type { MenuSection as MenuContent } from "@/lib/content/types";
// @ts-ignore: CSS module import for side effects
import "./our-menu.css";
// @ts-ignore: CSS module import for side effects
import "@/app/order/order.css";
// @ts-ignore: CSS module import for side effects
import "@/components/styles/auth.css";
import Img from "@/components/ui/Img";
import Modal from "@/components/ui/Modal";
import Skeleton from "@/components/ui/Skeleton";
import EmptyState from "@/components/ui/EmptyState";


const labels = ["", "", "", "", "Fun. fast. loud"];

// The link to the full menu is only for the landing page, the menu page already is the full menu.
export default function OurMenu({
  content,
  showFullMenuLink = true,
  showSearch = false,
}: {
  content: MenuContent;
  showFullMenuLink?: boolean;
  // Search and price filter, used on the menu page only.
  showSearch?: boolean;
}) {
  const strip = content.slide_images.length
    ? content.slide_images.map((i) => i.url ?? "")
    : fallbackMenuSlides;
  const { items: catalog, categories, status, reload } = useCatalog();
  const FILTERS = [{ id: "all", name: content.all_label }, ...categories];
  useReveal();
  const { add } = useStore();
  const [filter, setFilter] = useState("all");
  const [query, setQuery] = useState("");
  const [minPrice, setMinPrice] = useState("");
  const [maxPrice, setMaxPrice] = useState("");
  const [selected, setSelected] = useState<CatalogItem | null>(null);

  const items = useMemo(() => {
    const byCategory =
      filter === "all"
        ? catalog
        : catalog.filter((item) => item.categoryId === filter);
    if (!showSearch) return byCategory;

    const text = query.trim().toLowerCase();
    const min = minPrice === "" ? 0 : Number(minPrice);
    const max = maxPrice === "" ? Infinity : Number(maxPrice);
    return byCategory.filter(
      (item) =>
        (!text || item.name.toLowerCase().includes(text)) &&
        item.price >= min &&
        item.price <= max,
    );
  }, [catalog, filter, showSearch, query, minPrice, maxPrice]);

  return (
    <section className="menu" id="menu">
      <div className="menu__labels" aria-hidden="true">
        {labels.map((label, i) => (
          <span key={i} className="menu__label">
            {label}
          </span>
        ))}
      </div>

      <div className="menu__mask" data-reveal>
        <div className="menu__slider" aria-hidden="true">
          <div className="menu__slider-track">
            {[...strip, ...strip, ...strip].map((src, i) => (
              <Img
                key={i}
                className="menu__slider-img"
                src={src}
                alt=""
                loading="lazy"
              />
            ))}
          </div>
        </div>
        <p className="menu__script">
          {content.script_lines.map((line, i) => (
            <span key={i}>
              {i > 0 ? <br /> : null}
              {line}
            </span>
          ))}
        </p>
      </div>

      <div className="menu__grid">
        <div className="menu__intro" data-reveal>
          <h3 className="menu__intro-title">
            {content.intro_lines.map((line, i) => (
              <span key={i}>
                {i > 0 ? <br /> : null}
                {line}
              </span>
            ))}
          </h3>
          {showFullMenuLink ? (
            <a href="/menu" className="button menu__button" data-magnetic>
              <span className="button__text">{content.intro_button}</span>
              <span className="button__icon">
                <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path
                    d="M5 12h14M13 6l6 6-6 6"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  />
                </svg>
              </span>
              <span className="button__inner" />
            </a>
          ) : null}
        </div>

        <div className="menu__col">
        {showSearch ? (
          <div className="menu__search">
            <input
              type="search"
              className="menu__search-input"
              placeholder="Search the menu"
              aria-label="Search the menu"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <input
              type="number"
              inputMode="numeric"
              min={0}
              className="menu__price-input"
              placeholder="Min ₦"
              aria-label="Minimum price"
              value={minPrice}
              onChange={(e) => setMinPrice(e.target.value)}
            />
            <input
              type="number"
              inputMode="numeric"
              min={0}
              className="menu__price-input"
              placeholder="Max ₦"
              aria-label="Maximum price"
              value={maxPrice}
              onChange={(e) => setMaxPrice(e.target.value)}
            />
          </div>
        ) : null}
        <div className="menu__filters" role="tablist" aria-label="Menu categories">
          {FILTERS.map((f) => (
            <button
              key={f.id}
              type="button"
              role="tab"
              aria-selected={filter === f.id}
              className={`menu__filter ${filter === f.id ? "is-active" : ""}`}
              onClick={() => setFilter(f.id)}
            >
              {f.name}
            </button>
          ))}
        </div>

        {status === "error" ? (
          <EmptyState
            title="We couldn't load the menu"
            text="Check your connection and try again."
            action={
              <button type="button" className="order-form__submit" onClick={reload}>
                Try again
              </button>
            }
          />
        ) : null}

        {status === "ready" && catalog.length === 0 ? (
          <EmptyState title="The menu is empty right now" text="Please check back soon." />
        ) : null}

        {status === "ready" && catalog.length > 0 && showSearch && items.length === 0 ? (
          <p className="menu__no-results">No items match your search.</p>
        ) : null}

        <div className="menu__cards">
          {status === "loading"
            ? Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="menu-card" aria-hidden="true">
                  <Skeleton height="auto" className="menu-card__skeleton" />
                  <Skeleton width="70%" height="1.4rem" className="menu-card__skeleton-line" />
                </div>
              ))
            : null}
          {items.map((item) => (
            <article key={item.id} className="menu-card">
              <div className="menu-card__img-wrap">
                {item.image ? (
                  <Img
                    className="menu-card__img"
                    src={item.image}
                    alt={item.name}
                    loading="lazy"
                  />
                ) : (
                  <div className="menu-card__img menu-card__img--empty" aria-hidden="true" />
                )}

                {!item.available && (
                  <div className="menu-card__tags">
                    <span className="menu-card__tag">Sold out</span>
                  </div>
                )}

                <div className="menu-card__preview">
                  <span className="menu-card__price">{naira(item.price)}</span>
                  <button
                    type="button"
                    className="menu-card__order"
                    onClick={() => add(toCartItem(item))}
                    disabled={!item.available}
                    aria-label={`Add ${item.name} to bag`}
                  >
                    {item.available ? content.add_label : "Sold out"}
                    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                      <path
                        d="M7.05 16.95 16.95 7.05M16.95 7.05H8.46M16.95 7.05v8.49"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      />
                    </svg>
                  </button>
                </div>
              </div>
              <div className="menu-card__row">
                <h3 className="menu-card__title">
                  <button
                    type="button"
                    className="menu-card__open"
                    onClick={() => setSelected(item)}
                  >
                    {item.name}
                  </button>
                </h3>
                <span className="menu-card__tag-price">{naira(item.price)}</span>
              </div>
              <span className="menu-card__link">{item.category}</span>
            </article>
          ))}
        </div>
        </div>
      </div>

      <Modal
        open={selected !== null}
        title={selected?.name ?? ""}
        onClose={() => setSelected(null)}
        actions={
          selected ? (
            <button
              type="button"
              className="order-form__submit"
              disabled={!selected.available}
              onClick={() => {
                add(toCartItem(selected));
                setSelected(null);
              }}
            >
              {selected.available ? `${content.add_label} · ${naira(selected.price)}` : "Sold out"}
            </button>
          ) : null
        }
      >
        {selected ? (
          <>
            {selected.image ? (
              <Img className="menu-detail__img" src={selected.image} alt={selected.name} />
            ) : null}
            <p className="menu-detail__meta">
              {selected.category} · {naira(selected.price)}
            </p>
            {selected.description ? <p>{selected.description}</p> : null}
            {!selected.available ? (
              <p className="menu-detail__sold">This item is sold out right now.</p>
            ) : null}
          </>
        ) : null}
      </Modal>
    </section>
  );
}
