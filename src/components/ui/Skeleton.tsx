// @ts-ignore: CSS module import for side effects
import "./ui.css";

type Props = { width?: string | number; height?: string | number; className?: string };

/** A grey shimmering placeholder shown while content loads. */
export default function Skeleton({ width = "100%", height = "1rem", className = "" }: Props) {
  return (
    <span
      className={`ui-skeleton ${className}`}
      style={{ width, height }}
      aria-hidden="true"
    />
  );
}
