"use client";

import { useEffect } from "react";
import { gsap } from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";

gsap.registerPlugin(ScrollTrigger);

export function useReveal() {
  useEffect(() => {
    const els = gsap.utils
      .toArray<HTMLElement>("[data-reveal]")
      .filter((el) => !el.dataset.revealed);

    els.forEach((el) => {
      el.dataset.revealed = "true";
      gsap.fromTo(
        el,
        { y: 50, opacity: 0 },
        {
          y: 0,
          opacity: 1,
          duration: 1.5,
          ease: "power4.out",
          immediateRender: false,
          scrollTrigger: {
            trigger: el,
            start: "top 90%",
          },
        },
      );
    });

    ScrollTrigger.refresh();
  }, []);
}
