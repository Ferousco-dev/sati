"use client";

import { useEffect } from "react";
import { gsap } from "gsap";

export function useMagnetic() {
  useEffect(() => {
    const els = gsap.utils.toArray<HTMLElement>("[data-magnetic]");
    const cleanups: Array<() => void> = [];

    els.forEach((el) => {
      const move = (e: MouseEvent) => {
        const rect = el.getBoundingClientRect();
        const x = e.clientX - rect.left - rect.width / 2;
        const y = e.clientY - rect.top - rect.height / 2;
        gsap.to(el, { x: x * 0.3, y: y * 0.4, duration: 0.6, ease: "power3.out" });
      };
      const reset = () => {
        gsap.to(el, { x: 0, y: 0, duration: 0.6, ease: "elastic.out(1, 0.4)" });
      };

      el.addEventListener("mousemove", move);
      el.addEventListener("mouseleave", reset);
      cleanups.push(() => {
        el.removeEventListener("mousemove", move);
        el.removeEventListener("mouseleave", reset);
      });
    });

    return () => cleanups.forEach((fn) => fn());
  }, []);
}
