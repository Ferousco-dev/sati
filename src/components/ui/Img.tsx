"use client";

import { useEffect, useRef, useState } from "react";
// @ts-ignore: CSS module import for side effects
import "./ui.css";

type Props = React.ImgHTMLAttributes<HTMLImageElement>;

/**
 * An image that shows a shimmering placeholder until it has loaded, so pictures
 * never pop in or leave a blank hole. Use it instead of a plain img.
 */
export default function Img({ className = "", onLoad, onError, alt = "", ...rest }: Props) {
  const ref = useRef<HTMLImageElement>(null);
  const [state, setState] = useState<"loading" | "done">("loading");

  // A picture that is already cached finishes before React attaches onLoad.
  useEffect(() => {
    if (ref.current?.complete) setState("done");
  }, []);

  return (
    <img
      {...rest}
      ref={ref}
      alt={alt}
      className={`${className} ui-img ${state === "loading" ? "is-loading" : ""}`.trim()}
      onLoad={(e) => {
        setState("done");
        onLoad?.(e);
      }}
      onError={(e) => {
        setState("done");
        onError?.(e);
      }}
    />
  );
}
