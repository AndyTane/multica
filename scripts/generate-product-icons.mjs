// Render the checked-in ProductMark geometry with the existing Chromium toolchain.
import { chromium } from "@playwright/test";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve, dirname } from "node:path";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const source = await readFile(resolve(root, "packages/ui/components/common/product-mark.tsx"), "utf8");
const glyph = source.match(/d="(M[^"]+)"/)[1];
const strokes = Object.fromEntries([...source.match(/strokeBySize = \{([^}]+)\}/)[1].matchAll(/(\d+): ([\d.]+)/g)].map((m) => [Number(m[1]), m[2]]));
const stroke = (size) => strokes[Object.keys(strokes).map(Number).find((key) => size <= key) ?? 32];
// adaptive: the in-app tile with the U knocked out, following the browser theme.
// maskable: a full-bleed field with the U kept inside the 80% safe zone.
function svg(size, mode = "app") {
  const u = (color) => `<path d="${glyph}" fill="none" stroke="${color}" stroke-width="${stroke(size)}" stroke-linecap="round"/>`;
  if (mode === "adaptive") return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><style>:root{color:#161616}@media(prefers-color-scheme:dark){:root{color:#f5f5f5}}</style><mask id="u"><rect width="24" height="24" fill="white"/>${u("black")}</mask><rect width="24" height="24" rx="7" fill="currentColor" mask="url(#u)"/></svg>`;
  if (mode === "maskable") return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" fill="#161616"/><g transform="translate(12 12) scale(0.72) translate(-12 -12)">${u("#f5f5f5")}</g></svg>`;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="7" fill="#161616"/>${u("#f5f5f5")}</svg>`;
}
await writeFile(resolve(root, "apps/web/public/favicon.svg"), svg(32, "adaptive"));
await writeFile(resolve(root, "apps/web/public/icons/icon.svg"), svg(32));
await mkdir(resolve(root, "apps/desktop/build/icons"), { recursive: true });
const browser = await chromium.launch({ channel: "msedge", headless: true });
const pngs = new Map();
try {
  const page = await browser.newPage({ deviceScaleFactor: 1 });
  for (const size of [16,24,32,48,64,128,180,192,256,512,1024]) {
    await page.setViewportSize({ width: size, height: size });
    await page.setContent(`<style>*{margin:0}svg{display:block;width:100vw;height:100vh}</style>${svg(size)}`);
    pngs.set(size, await page.screenshot({ omitBackground: true }));
  }
  await page.setViewportSize({ width: 512, height: 512 });
  await page.setContent(`<style>*{margin:0}svg{display:block;width:100vw;height:100vh}</style>${svg(512, "maskable")}`);
  pngs.set("maskable", await page.screenshot());
} finally { await browser.close(); }
for (const size of [16,24,32,48,64,128,256,512]) await writeFile(resolve(root, `apps/desktop/build/icons/${size}x${size}.png`), pngs.get(size));
for (const f of ["apps/desktop/build/icon.png", "apps/desktop/resources/icon.png"]) await writeFile(resolve(root, f), pngs.get(1024));
for (const [file, size] of [["icon-192.png",192],["icon-512.png",512],["icon-maskable-512.png","maskable"],["apple-touch-icon.png",180]]) await writeFile(resolve(root, `apps/web/public/icons/${file}`), pngs.get(size));
const icon = pngs.get(256);
const ico = Buffer.alloc(22); ico.writeUInt16LE(1,2); ico.writeUInt16LE(1,4); ico.writeUInt16LE(1,10); ico.writeUInt16LE(32,12); ico.writeUInt32LE(icon.length,14); ico.writeUInt32LE(22,18);
await writeFile(resolve(root,"apps/desktop/build/icon.ico"), Buffer.concat([ico,icon]));
const chunks = [["ic07",128],["ic08",256],["ic09",512],["ic10",1024]].map(([tag,size]) => { const data=pngs.get(size), header=Buffer.alloc(8); header.write(tag);header.writeUInt32BE(data.length+8,4);return Buffer.concat([header,data]); });
const header=Buffer.alloc(8);header.write("icns");header.writeUInt32BE(8+chunks.reduce((sum,x)=>sum+x.length,0),4);
await writeFile(resolve(root,"apps/desktop/build/icon.icns"), Buffer.concat([header,...chunks]));
console.log("Generated Web/PWA and Desktop PNG, ICO and ICNS icons from ProductMark.");
