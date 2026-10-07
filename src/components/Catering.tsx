"use client";

import { useReveal } from "@/lib/useReveal";
import { useParallax } from "@/lib/useParallax";
import { useSplitReveal } from "@/lib/useSplitReveal";
// @ts-ignore: CSS module import for side effects
import "./catering.css";

const wing = (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width="100%"
    viewBox="0 0 152 85"
    fill="none"
    className="wing-icon"
  >
    <path
      d="M130.93 80.9708C126.663 79.7842 121.514 77.5234 117.503 75.2465L130.085 57.0712C132.689 59.1317 136.427 61.3565 140.603 62.9319C138.487 69.1693 134.729 75.8718 130.93 80.9708ZM101.508 63.1483C97.8741 60.0577 94.1878 56.7465 90.3373 53.4915L112.787 33.7449C115.552 36.9438 117.559 40.4193 119.567 43.8346L101.512 63.1483H101.508ZM115.28 73.8915C110.996 71.2859 107.31 68.3035 103.403 64.9402L120.645 45.843C122.812 49.2624 125.092 52.786 128.398 55.7724L115.276 73.8915H115.28ZM88.3299 51.7036C84.2069 48.4486 79.9236 45.2497 74.6667 42.2632L102.157 24.9018C106.06 27.0183 108.608 29.0267 111.429 32.1173L88.3299 51.6996V51.7036ZM72.2265 40.6919C65.447 36.7874 57.9103 33.801 50.5378 31.2515L88.8748 18.0149C92.5611 19.9671 97.0086 21.9754 100.098 23.6551L72.2265 40.6878V40.6919ZM46.6311 29.7883C35.949 26.2086 25.5393 23.0618 16.9167 18.7204L76.6701 9.76909C79.7072 12.5912 82.9608 14.816 86.3225 16.66L46.6311 29.7883ZM13.2304 16.4916C6.72341 12.4789 1.79104 7.32381 0 0H69.9467C71.1407 3.14679 72.9277 5.74841 74.827 7.86498L13.2304 16.4916Z"
      fill="currentColor"
    ></path>
    <path
      d="M133.604 81.9575C139.662 83.7254 145.66 84.2345 151.755 84.2986C151.999 73.82 151.991 64.6923 151.991 64.6923C151.991 64.6923 147.98 64.9047 142.723 63.5498C140.6 70.0158 136.268 77.6683 133.6 81.9575H133.604Z"
      fill="currentColor"
    ></path>
    <path
      d="M140.33 60.3263C122.003 53.3271 122.925 38.6274 108.989 26.854C99.2845 18.6643 78.6255 14.4312 72.8757 0H141.18C143.295 0 144.377 0.753627 144.377 2.15666C144.377 5.12707 141.051 11.7414 141.051 25.1102C141.051 38.4791 145.218 48.9096 151.722 60.9316C152.25 61.9057 152.174 62.4909 150.179 62.4909C145.375 62.4909 140.33 60.3263 140.33 60.3263Z"
      fill="currentColor"
    ></path>
  </svg>
);

const arrow = (
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
);

const locations = [
  { image: "/images/locations/loc-1.webp", label: "Right on campus" },
  { image: "/images/locations/loc-2.webp", label: "Open late" },
  { image: "/images/locations/loc-3.webp", label: "Fuel up" },
];

export default function Catering() {
  useReveal();
  useParallax();
  useSplitReveal();

  return (
    <section className="catering" id="stores">
      <div className="catering__inner">
        <div className="catering__banner" data-reveal>
          <img
            className="catering__banner-img"
            src="/images/locations/catering.avif"
            alt="Catering spread"
            loading="lazy"
            data-parallax="-12"
          />
          <p className="catering__banner-note">
            Feed the mates
            <br />
            to the masses
          </p>
        </div>

        <div className="catering__copy" data-reveal>
          <h2 className="catering__title">
            {wing}
            <span>We&rsquo;ve got your catering covered</span>
          </h2>
          <div className="catering__right">
            <p className="catering__text">
              From study-week all-nighters to club socials and faculty events,
              our catering is built to feed any hungry crowd.
            </p>
            <a href="#catering" className="button is-light" data-magnetic>
              <span className="button__text">Learn more</span>
              {arrow}
              <span className="button__inner" />
            </a>
          </div>
        </div>

        <div className="catering__gallery">
          <figure className="gallery-card is-tall" data-reveal>
            <div className="gallery-card__img-wrap">
              <img src={locations[0].image} alt="" loading="lazy" />
            </div>
            <figcaption className="gallery-card__label">
              {locations[0].label}
            </figcaption>
          </figure>

          <div className="gallery-col" data-reveal>
            <figure className="gallery-card">
              <div className="gallery-card__img-wrap">
                <img src={locations[1].image} alt="" loading="lazy" />
              </div>
              <figcaption className="gallery-card__label">
                {locations[1].label}
              </figcaption>
            </figure>
            <figure className="gallery-card">
              <div className="gallery-card__img-wrap">
                <img src={locations[2].image} alt="" loading="lazy" />
              </div>
              <figcaption className="gallery-card__label">
                {locations[2].label}
              </figcaption>
            </figure>
          </div>

          <div className="gallery-card is-find" data-reveal>
            <div className="find-local">
              <h3 className="find-local__title" data-split>
                Find your local
              </h3>
              <a href="#stores" className="button is-light" data-magnetic>
                <span className="button__text">Find a spot near you</span>
                {arrow}
                <span className="button__inner" />
              </a>
            </div>
            <img
              className="find-local__map"
              src="/images/locations/map.png"
              alt=""
              loading="lazy"
            />
          </div>
        </div>
      </div>
    </section>
  );
}
