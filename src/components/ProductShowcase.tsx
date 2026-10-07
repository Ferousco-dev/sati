"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useReveal } from "@/lib/useReveal";
import { fallbackShowcase } from "@/lib/fallbacks";
import type { Showcase as ShowcaseContent } from "@/lib/content/types";
import { useStore } from "@/lib/store";
import { MENU_ITEMS } from "@/lib/menu";
// @ts-ignore: CSS module import for side effects
import "./product-showcase.css";
import Img from "@/components/ui/Img";

export default function ProductShowcase({ content }: { content: ShowcaseContent }) {
  const products = content.items.length
    ? content.items.map((i) => ({ name: i.name, image: i.image.url ?? "", productId: i.product_id }))
    : fallbackShowcase.map((i) => ({ name: i.name, image: i.image, productId: i.product_id }));
  const [active, setActive] = useState(Math.min(2, products.length - 1));
  const { add } = useStore();
  const router = useRouter();
  useReveal();

  // A card adds its product to the bag, or opens the menu when no product is linked.
  const order = (productId: string) => {
    const item = MENU_ITEMS.find((i) => i.id === productId);
    if (item) add(item);
    else router.push("/menu");
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

              <Img
                className="card__img"
                src={product.image}
                alt={product.name}
                loading="lazy"
              />

              <button
                type="button"
                className="card__tag"
                aria-label={`Order ${product.name}`}
                onClick={() => order(product.productId)}
              >
                <span>{content.order_label}</span>
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

      <a href={content.marquee_url || "/menu"} className="showcase__marquee" aria-label={content.marquee_text}>
        <div className="showcase__marquee-track">
          {Array.from({ length: 8 }).map((_, i) => (
            <span key={i} className="showcase__marquee-item">
              {content.marquee_text}
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
