"use client";

import { useEffect } from "react";
import { gsap } from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";

gsap.registerPlugin(ScrollTrigger);

export function useSplitReveal() {
  useEffect(() => {
    const ctx = gsap.context(() => {
      gsap.utils.toArray<HTMLElement>("[data-split]").forEach((el) => {
        if (el.dataset.splitDone) return;
        el.dataset.splitDone = "true";

        const words = el.textContent?.trim().split(/\s+/) ?? [];
        el.textContent = "";

        const lines = words.map((word) => {
          const mask = document.createElement("span");
          mask.className = "split-mask";
          const inner = document.createElement("span");
          inner.className = "split-word";
          inner.textContent = word;
          mask.appendChild(inner);
          el.appendChild(mask);
          el.appendChild(document.createTextNode(" "));
          return inner;
        });

        gsap.from(lines, {
          yPercent: 115,
          duration: 1,
          ease: "power4.out",
          stagger: 0.08,
          scrollTrigger: { trigger: el, start: "top 88%" },
        });
      });
    });

    return () => ctx.revert();
  }, []);
}
