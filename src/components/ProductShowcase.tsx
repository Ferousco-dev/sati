"use client";

import { useState } from "react";
import { useReveal } from "@/lib/useReveal";
import { useStore } from "@/lib/store";
import { MENU_ITEMS } from "@/lib/menu";
// @ts-ignore: CSS module import for side effects
import "./product-showcase.css";

// Each card adds its menu item to the bag.
const products = [
  { name: "Beast Fuel", image: "/images/products/beast-fuel.webp", menuId: "beast-fuel" },
  { name: "Chicken", image: "/images/products/chicken.webp", menuId: "chicken-classic" },
  { name: "Shawarma", image: "/images/products/shawarma.webp", menuId: "shawarma-double" },
  {
    name: "Chicken & Chips",
    image: "/images/products/yogurt-chicken-chips.webp",
    menuId: "yogurt-chicken-chips",
  },
  { name: "Cake", image: "/images/products/cake.webp", menuId: "cake-slice" },
];

export default function ProductShowcase() {
  const [active, setActive] = useState(2);
  const { add } = useStore();
  useReveal();

  const order = (menuId: string) => {
    const item = MENU_ITEMS.find((i) => i.id === menuId);
    if (item) add(item);
  };

  return (
    <section className="showcase" id="brand">
      <div className="showcase__list" data-reveal>
        {products.map((product, i) => (
          <article
            key={product.name}
            className={`card ${i === active ? "is-active" : ""}`}
            onMouseEnter={() => setActive(i)}
          >
            <div className="card__img-wrap">
              <div className="card__header">
                <h2>{product.name}</h2>
              </div>

              <div className="card__text" aria-hidden="true">
                {[0, 1, 2].map((row) => (
                  <div
                    key={row}
                    className={`card__text-row ${row % 2 === 0 ? "is-right" : "is-left"}`}
                  >
                    {Array.from({ length: 4 }).map((_, i) => (
                      <span key={i}>{product.name}</span>
                    ))}
                  </div>
                ))}
              </div>

              <img
                className="card__img"
                src={product.image}
                alt={product.name}
                loading="lazy"
              />

              <button
                type="button"
                className="card__tag"
                aria-label={`Order ${product.name}`}
                onClick={() => order(product.menuId)}
              >
                <span>Order Now</span>
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
          </article>
        ))}
      </div>

      <a href="/menu" className="showcase__marquee" aria-label="Order now">
        <div className="showcase__marquee-track">
          {Array.from({ length: 8 }).map((_, i) => (
            <span key={i} className="showcase__marquee-item">
              Order Now
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
                <path
                  d="M7.05 16.95 16.95 7.05M16.95 7.05H8.46M16.95 7.05v8.49"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
            </span>
          ))}
        </div>
      </a>
    </section>
  );
}
