// @vitest-environment node
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// Independently check the optional theme's painted colours; the upstream
// contrast guard continues to check the unmodified upstream design tokens.
const repoRoot = resolve(process.cwd(), "../..");
const backgroundTokens = [
  "--app-shell", "--page-canvas", "--background", "--surface",
  "--surface-raised", "--surface-hover", "--surface-selected",
  "--card", "--popover", "--muted", "--secondary", "--accent",
  "--sidebar", "--sidebar-accent",
];

type Rgb = [number, number, number];

function readBlock(source: string, selector: string): Map<string, string> {
  const start = source.indexOf(`${selector} {`);
  if (start < 0) throw new Error(`${selector} block not found`);
  const end = source.indexOf("\n}", start);
  if (end < 0) throw new Error(`${selector} block is unterminated`);

  const declarations = new Map<string, string>();
  for (const match of source.slice(start, end).matchAll(/(--[\w-]+):\s*([^;]+);/g)) {
    const [, name, value] = match;
    if (name && value) declarations.set(name, value.trim());
  }
  return declarations;
}

/** Follows `--card: var(--surface)` style indirection to a literal colour. */
function resolveToken(declarations: Map<string, string>, name: string): string {
  let current = name;

  for (let hops = 0; hops < 8; hops++) {
    const value = declarations.get(current);
    if (value === undefined) {
      throw new Error(`${current} is not declared (while resolving ${name})`);
    }
    const alias = /^var\((--[\w-]+)\)$/.exec(value)?.[1];
    if (!alias) return value;
    current = alias;
  }
  throw new Error(`${name} did not resolve to a literal colour`);
}

const clamp01 = (x: number) => Math.min(1, Math.max(0, x));
const encodeSrgb = (c: number) =>
  c <= 0.0031308 ? 12.92 * c : 1.055 * Math.pow(c, 1 / 2.4) - 0.055;
const decodeSrgb = (c: number) =>
  c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);

/**
 * Quantises to 8-bit on purpose: contrast is judged on what the display
 * actually paints, not on the unrounded float behind it.
 */
function oklchToRgb(value: string, underlying?: Rgb): Rgb {
  const match = /^oklch\(\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)(?:\s*\/\s*([\d.]+)(%)?)?\s*\)$/.exec(value);
  if (!match?.[1] || !match[2] || !match[3]) {
    throw new Error(`expected an oklch() colour, got "${value}"`);
  }

  const lightness = Number(match[1]);
  const chroma = Number(match[2]);
  const hue = (Number(match[3]) * Math.PI) / 180;
  const a = chroma * Math.cos(hue);
  const b = chroma * Math.sin(hue);

  const l = (lightness + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (lightness - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (lightness - 0.0894841775 * a - 1.291485548 * b) ** 3;

  const channel = (linear: number) =>
    Math.round(clamp01(encodeSrgb(clamp01(linear))) * 255);

  const rgb: Rgb = [
    channel(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s),
    channel(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s),
    channel(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s),
  ];
  const alpha = match[4] === undefined ? 1 : Number(match[4]) / (match[5] ? 100 : 1);
  if (alpha === 1) return rgb;
  if (!underlying) throw new Error(`translucent colour needs an underlying surface: ${value}`);
  return rgb.map((channel, i) =>
    Math.round(channel * alpha + underlying[i]! * (1 - alpha)),
  ) as Rgb;
}

// Selected blue tints appear on both the canvas and nested content surfaces.
// Judge their painted result; comparing the uncomposited blue is meaningless.
function paintedBackgrounds(declarations: Map<string, string>, token: string): Rgb[] {
  const value = resolveToken(declarations, token);
  if (!value.includes("/")) return [oklchToRgb(value)];
  return ["--app-shell", "--page-canvas", "--surface", "--surface-raised", "--surface-hover"]
    .map((base) => oklchToRgb(value, oklchToRgb(resolveToken(declarations, base))));
}

function relativeLuminance([r, g, b]: Rgb): number {
  return (
    0.2126 * decodeSrgb(r / 255) +
    0.7152 * decodeSrgb(g / 255) +
    0.0722 * decodeSrgb(b / 255)
  );
}

function contrastRatio(foreground: Rgb, background: Rgb): number {
  const a = relativeLuminance(foreground);
  const b = relativeLuminance(background);
  const [lighter, darker] = a > b ? [a, b] : [b, a];
  return (lighter + 0.05) / (darker + 0.05);
}

function expectTonePasses(
  declarations: Map<string, string>,
  tone: string,
  floor: number,
) {
  const foreground = oklchToRgb(resolveToken(declarations, tone));

  for (const token of backgroundTokens) {
    for (const background of paintedBackgrounds(declarations, token)) {
      const ratio = contrastRatio(foreground, background);

      expect(
        Number(ratio.toFixed(2)),
        `${tone} on ${token} is ${ratio.toFixed(2)}:1`,
      ).toBeGreaterThanOrEqual(floor);
    }
  }
}


const themeCss = readFileSync(
  resolve(repoRoot, "packages/ui/styles/themes/uworkers.css"), "utf8",
);

describe.each([":root", ".dark"])("UWorkers theme %s", (selector) => {
  const declarations = readBlock(themeCss, selector);

  it.each([
    "--foreground", "--surface-foreground", "--card-foreground",
    "--popover-foreground", "--secondary-foreground", "--accent-foreground",
    "--sidebar-foreground", "--muted-foreground", "--primary", "--brand",
    "--success", "--warning", "--info", "--destructive",
  ])("keeps %s readable on every content surface", (tone) => {
    expectTonePasses(declarations, tone, 4.5);
  });

  it.each(["--primary", "--brand", "--sidebar-primary"])(
    "keeps %s button labels readable", (tone) => {
      const fill = oklchToRgb(resolveToken(declarations, tone));
      const label = oklchToRgb(resolveToken(declarations, `${tone}-foreground`));
      expect(contrastRatio(label, fill)).toBeGreaterThanOrEqual(4.5);
    },
  );

  it.each(["--success", "--warning", "--info", "--destructive"])(
    "keeps %s text readable on its 10 percent tint", (tone) => {
      const value = resolveToken(declarations, tone);
      const text = oklchToRgb(value);
      for (const base of ["--page-canvas", "--surface", "--surface-raised"]) {
        const background = oklchToRgb(
          value.replace(")", " / 10%)"),
          oklchToRgb(resolveToken(declarations, base)),
        );
        expect(contrastRatio(text, background)).toBeGreaterThanOrEqual(4.5);
      }
    },
  );

  it.each([
    "--faint-foreground", "--chart-1", "--chart-2", "--chart-3",
    "--chart-4", "--chart-5",
  ])("keeps %s non-text marks distinguishable", (tone) => {
    expectTonePasses(declarations, tone, 3);
  });
});
