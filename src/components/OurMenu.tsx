"use client";

import { useMemo, useState } from "react";
import { useReveal } from "@/lib/useReveal";
import { useStore } from "@/lib/store";
import { MENU, MENU_ITEMS, naira } from "@/lib/menu";
// @ts-ignore: CSS module import for side effects
import "./our-menu.css";

const FILTERS = [{ id: "all", name: "All" }, ...MENU.map((c) => ({ id: c.id, name: c.name }))];

const strip = [
  "/images/products/pizza.webp",
  "/images/products/beast-fuel.webp",
  "/images/products/chicken.webp",
  "/images/products/shawarma.webp",
  "/images/products/yogurt-chicken-chips.webp",
  "/images/products/cake.webp",
];

const labels = ["", "", "", "", "Fun. fast. loud"];

// Map each item id to its category id, so the filter tabs can match items.
const ITEM_CATEGORY: Record<string, string> = Object.fromEntries(
  MENU.flatMap((c) => c.items.map((item) => [item.id, c.id])),
);

export default function OurMenu() {
  useReveal();
  const { add } = useStore();
  const [filter, setFilter] = useState("all");

  const items = useMemo(
    () =>
      filter === "all"
        ? MENU_ITEMS
        : MENU_ITEMS.filter((item) => ITEM_CATEGORY[item.id] === filter),
    [filter],
  );

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
              <img
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
          Sweet treats
          <br />
          &amp; healthy sips
        </p>
      </div>

      <div className="menu__grid">
        <div className="menu__intro" data-reveal>
          <h3 className="menu__intro-title">
            Fresh <span className="menu__amp">&amp;</span> sweet
            <br />
            for every occasion
          </h3>
          <a href="/menu" className="button menu__button" data-magnetic>
            <span className="button__text">See the full menu</span>
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
        </div>

        <div className="menu__col">
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

        <div className="menu__cards">
          {items.map((item) => (
            <article key={item.id} className="menu-card">
              <div className="menu-card__img-wrap">
                <img
                  className="menu-card__img"
                  src={item.image}
                  alt={item.name}
                  loading="lazy"
                />

                {item.tags && item.tags.length > 0 && (
                  <div className="menu-card__tags">
                    {item.tags.map((tag) => (
                      <span
                        key={tag}
                        className={`menu-card__tag tag--${tag.toLowerCase().replace(/[^a-z]/g, "")}`}
                      >
                        {tag}
                      </span>
                    ))}
                  </div>
                )}

                <div className="menu-card__preview">
                  <span className="menu-card__price">{naira(item.price)}</span>
                  <button
                    type="button"
                    className="menu-card__order"
                    onClick={() => add(item)}
                    aria-label={`Add ${item.name} to bag`}
                  >
                    Add to bag
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
                <h3 className="menu-card__title">{item.name}</h3>
                <span className="menu-card__tag-price">{naira(item.price)}</span>
              </div>
              <span className="menu-card__link">{item.category}</span>
            </article>
          ))}
        </div>
        </div>
      </div>
    </section>
  );
}
