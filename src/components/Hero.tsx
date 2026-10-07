"use client";

import { useEffect, useRef, useState } from "react";
import { gsap } from "gsap";
// @ts-ignore: CSS module import for side effects
import "./hero.css";
import type { Hero as HeroContent } from "@/lib/content/types";
import Img from "@/components/ui/Img";

// Used until pictures are chosen for the hero in the admin.
const fallbackSlides = [
  "/images/hero/slide-1.webp",
  "/images/hero/slide-2.webp",
  "/images/hero/slide-3.webp",
  "/images/hero/slide-4.webp",
];

export default function Hero({ content }: { content: HeroContent }) {
  const slides = content.images.length
    ? content.images.map((i) => i.url).filter((u): u is string => Boolean(u))
    : fallbackSlides;
  const mobileImage = content.mobile_image?.url ?? "/images/hero-mobile.webp";
  const heroRef = useRef<HTMLElement>(null);
  const [current, setCurrent] = useState(0);
  const [playing, setPlaying] = useState(true);

  useEffect(() => {
    if (!playing || slides.length < 2) return;
    const id = setInterval(() => {
      setCurrent((c) => (c + 1) % slides.length);
    }, 4500);
    return () => clearInterval(id);
  }, [playing, slides.length]);

  useEffect(() => {
    const ctx = gsap.context(() => {
      const mm = gsap.matchMedia();
      const intro = (scope: string) => () => {
        gsap
          .timeline({ defaults: { ease: "power4.out" } })
          .from(`${scope} .hero__subtitle`, {
            y: 40,
            autoAlpha: 0,
            duration: 1,
          })
          .from(
            `${scope} .hero__title-line`,
            { yPercent: 100, duration: 1.5, stagger: 0.2 },
            "-=0.6",
          )
          .from(
            `${scope} .hero__cta, ${scope} .hero__control`,
            { y: 24, autoAlpha: 0, duration: 1 },
            "-=0.8",
          );
      };
      mm.add("(min-width: 768px)", intro(".is-desktop"));
      mm.add("(max-width: 767px)", intro(".is-mobile"));
    }, heroRef);

    return () => ctx.revert();
  }, []);

  const overlay = (
    <>
      {content.tagline ? <div className="hero__subtitle">{content.tagline}</div> : null}

      <h1 className="hero__title">
        {content.title_lines.map((line) => (
          <span className="hero__title-mask" key={line}>
            <span className="hero__title-line">{line}</span>
          </span>
        ))}
      </h1>
    </>
  );

  return (
    <header className="hero" ref={heroRef}>
      <div className="hero__media is-desktop">
        <div className="hero__slides">
          {slides.map((src, i) => (
            <div
              key={src}
              className={`hero__slide ${i === current ? "is-current" : ""}`}
              style={{ backgroundImage: `url(${src})` }}
            />
          ))}
        </div>

        <div className="hero__shade" />
        <div className="hero__texture" />
        {overlay}

        <a href={content.cta_url || "/menu"} className="button hero__cta" data-magnetic>
          <span className="button__text">{content.cta_label || "View menu"}</span>
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

        <div className="hero__dots" hidden={slides.length < 2}>
          {slides.map((src, i) => (
            <button
              key={src}
              type="button"
              className={`hero__dot ${i === current ? "is-current" : ""}`}
              aria-label={`Go to slide ${i + 1}`}
              onClick={() => setCurrent(i)}
            />
          ))}
        </div>

        <button
          type="button"
          className="hero__control"
          hidden={slides.length < 2}
          aria-label={playing ? "Pause slideshow" : "Play slideshow"}
          onClick={() => setPlaying((v) => !v)}
        >
          {playing ? (
            <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <rect x="6" y="5" width="4" height="14" rx="1" />
              <rect x="14" y="5" width="4" height="14" rx="1" />
            </svg>
          ) : (
            <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <path d="M8 5v14l11-7z" />
            </svg>
          )}
        </button>
      </div>

      <div className="hero__media is-mobile">
        <Img className="hero__image" src={mobileImage} alt="" />
        <div className="hero__shade" />
        <div className="hero__texture" />
        {overlay}
      </div>
    </header>
  );
}
