import type { ComponentProps } from "react";
import { cn } from "../../lib/utils";

type ProductMarkProps = Omit<ComponentProps<"span">, "children"> & {
  name: string;
  variant?: "mark" | "wordmark" | "lockup";
  size?: 16 | 24 | 32;
};

// Pixel-aligned U and W stems. The small master opens the counters; the large
// master tightens the joins. Each size is drawn separately, never scaled down
// from the application icon. All surfaces inherit the surrounding foreground.
const masters = {
  16: { path: "M1.5 4v6a2.5 2.5 0 0 0 5 0V4M8.5 4l1.3 8h1.1l1.1-4 1.1 4h1.1l1-8", weight: 1.5 },
  24: { path: "M2.5 5.5v9a3.5 3.5 0 0 0 7 0v-9M12 5.5l2 13h1.5l2-7 2 7H21l2-13", weight: 2 },
  32: { path: "M3 7v12a5 5 0 0 0 10 0V7M16 7l2.5 18h2l3-10 3 10h2L30.5 7", weight: 2.5 },
};

export function ProductMark({
  name,
  variant = "mark",
  size = 24,
  className,
  style,
  ...props
}: ProductMarkProps) {
  return (
    <span
      role="img"
      aria-label={name}
      className={cn("inline-flex shrink-0 items-center gap-2", className)}
      style={{ lineHeight: 1, ...style }}
      {...props}
    >
      {variant !== "wordmark" && (
        <svg aria-hidden="true" focusable="false" viewBox={`0 0 ${size} ${size}`} width={size} height={size} className="shrink-0" fill="none" stroke="currentColor" strokeWidth={masters[size].weight} strokeLinejoin="round">
          <path d={masters[size].path} />
        </svg>
      )}
      {variant !== "mark" && (
        <span aria-hidden="true" style={{ fontFamily: "var(--font-inter), Inter Variable, sans-serif", fontWeight: 600, letterSpacing: "-0.035em", fontSize: size * 0.75, whiteSpace: "nowrap" }}>
          {name}
        </span>
      )}
    </span>
  );
}
