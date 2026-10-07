"use client";

import { useEffect, useRef, useState } from "react";
import { gsap } from "gsap";
// @ts-ignore: CSS module import for side effects
import "./hero.css";

const slides = [
  "/images/hero/slide-1.webp",
  "/images/hero/slide-2.webp",
  "/images/hero/slide-3.webp",
  "/images/hero/slide-4.webp",
];

export default function Hero() {
  const heroRef = useRef<HTMLElement>(null);
  const [current, setCurrent] = useState(0);
  const [playing, setPlaying] = useState(true);

  useEffect(() => {
    if (!playing) return;
    const id = setInterval(() => {
      setCurrent((c) => (c + 1) % slides.length);
    }, 4500);
    return () => clearInterval(id);
  }, [playing]);

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
      <div className="hero__subtitle">Campus fresh.</div>

      <h1 className="hero__title">
        <span className="hero__title-mask">
          <span className="hero__title-line">
            Burgers, Shawarma<span className="hero__comma">,</span>
          </span>
        </span>
        <span className="hero__title-mask">
          <span className="hero__title-line">
            Made for the late-night grind.
          </span>
        </span>
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

        <a href="#menu" className="button hero__cta" data-magnetic>
          <span className="button__text">View Menu</span>
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

        <div className="hero__dots">
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
        <img className="hero__image" src="/images/hero-mobile.webp" alt="" />
        <div className="hero__shade" />
        <div className="hero__texture" />
        {overlay}
      </div>
    </header>
  );
}
