"use client";

import { useReveal } from "@/lib/useReveal";
import type { Gallery as Content } from "@/lib/content/types";
// @ts-ignore: CSS module import for side effects
import "./landing-sections.css";
import Img from "@/components/ui/Img";

const arrow = (
  <span className="button__icon">
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M5 12h14M13 6l6 6-6 6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  </span>
);

export default function GalleryStrip({ content }: { content: Content }) {
  useReveal();
  const images = content.images.filter((g) => g.image.url);
  if (!images.length) return null;

  return (
    <section className="lp is-white" id="gallery">
      <div className="lp__inner">
        <div className="lp__head" data-reveal>
          <h2 className="lp__title">{content.heading}</h2>
          {content.instagram_url ? (
            <a href={content.instagram_url} className="button" target="_blank" rel="noopener noreferrer" data-magnetic>
              <span className="button__text">Follow on Instagram</span>
              {arrow}
              <span className="button__inner" />
            </a>
          ) : null}
        </div>
        <div className="gal__strip" data-reveal>
          {images.map((g, i) => (
            <figure className="gal__item" key={`${g.image.media_id}-${i}`}>
              <Img
                src={g.image.url}
                alt={g.image.alt || g.caption}
                width={g.image.width || undefined}
                height={g.image.height || undefined}
                loading="lazy"
              />
              {g.caption ? <figcaption className="gal__caption">{g.caption}</figcaption> : null}
            </figure>
          ))}
        </div>
      </div>
    </section>
  );
}
