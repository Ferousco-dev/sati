"use client";

import { useReveal } from "@/lib/useReveal";
import Stars from "@/components/Stars";
import type { Testimonials as Content } from "@/lib/content/types";
// @ts-ignore: CSS module import for side effects
import "./landing-sections.css";

export default function Testimonials({ content }: { content: Content }) {
  useReveal();
  if (!content.items.length) return null;

  return (
    <section className="lp is-soft" id="reviews">
      <div className="lp__inner">
        <div className="lp__head" data-reveal>
          <h2 className="lp__title">{content.heading}</h2>
        </div>
        <div className="tst__grid" data-reveal>
          {content.items.map((t, i) => (
            <figure className="tst__card" key={`${t.name}-${i}`}>
              <Stars rating={t.rating} />
              <blockquote className="tst__quote">&ldquo;{t.quote}&rdquo;</blockquote>
              <figcaption className="tst__who">
                {t.avatar?.url ? (
                  <img className="tst__avatar" src={t.avatar.url} alt="" loading="lazy" />
                ) : (
                  <span className="tst__avatar is-initial" aria-hidden="true">
                    {t.name.charAt(0).toUpperCase()}
                  </span>
                )}
                <span>
                  <span className="tst__name">{t.name}</span>
                  {t.role ? <span className="tst__role">{t.role}</span> : null}
                </span>
              </figcaption>
            </figure>
          ))}
        </div>
      </div>
    </section>
  );
}
