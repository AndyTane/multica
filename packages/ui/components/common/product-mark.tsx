import { useId, type ComponentProps } from "react";
import { cn } from "../../lib/utils";

type ProductMarkProps = Omit<ComponentProps<"span">, "children"> & {
  name: string;
  variant?: "mark" | "wordmark" | "lockup";
  size?: 16 | 20 | 24 | 32;
};

// A squircle tile with a single knocked-out U. Drawn on a 24-unit grid; the
// stroke is optically compensated per size so the counter stays open at 16px
// and does not turn heavy at 32px. The tile inherits the foreground colour, so
// the U always shows the surface underneath.
const strokeBySize = { 16: 3.4, 20: 3.2, 24: 3, 32: 2.75 } as const;
const wordmarkBySize = { 16: 14, 20: 15, 24: 17, 32: 22 } as const;

export function ProductMark({
  name,
  variant = "mark",
  size = 24,
  className,
  style,
  ...props
}: ProductMarkProps) {
  const maskId = useId();
  return (
    <span
      role="img"
      aria-label={name}
      className={cn("inline-flex shrink-0 items-center", className)}
      style={{ lineHeight: 1, gap: Math.round(size * 0.4), ...style }}
      {...props}
    >
      {variant !== "wordmark" && (
        <svg aria-hidden="true" focusable="false" viewBox="0 0 24 24" width={size} height={size} className="shrink-0">
          <mask id={maskId}>
            <rect width="24" height="24" fill="white" />
            <path
              d="M8.25 6.75v5.5a3.75 3.75 0 0 0 7.5 0v-5.5"
              fill="none"
              stroke="black"
              strokeWidth={strokeBySize[size]}
              strokeLinecap="round"
            />
          </mask>
          <rect width="24" height="24" rx="7" fill="currentColor" mask={`url(#${maskId})`} />
        </svg>
      )}
      {variant !== "mark" && (
        <span
          aria-hidden="true"
          style={{
            fontFamily: "var(--font-inter), Inter Variable, sans-serif",
            fontWeight: 600,
            letterSpacing: "-0.02em",
            fontSize: wordmarkBySize[size],
            whiteSpace: "nowrap",
          }}
        >
          {name}
        </span>
      )}
    </span>
  );
}
