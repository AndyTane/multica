// Render the checked-in ProductMark geometry with the existing Chromium toolchain.
import { chromium } from "@playwright/test";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve, dirname } from "node:path";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const source = await readFile(resolve(root, "packages/ui/components/common/product-mark.tsx"), "utf8");
const masters = Object.fromEntries([...source.matchAll(/(16|24|32): \{ path: "([^"]+)", weight: ([\d.]+)/g)].map((m) => [m[1], { path: m[2], weight: m[3] }]));
function svg(size, adaptive = false) {
  const optical = size <= 32 ? size : 32;
  const master = masters[optical];
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${optical + 8} ${optical + 8}">${adaptive ? '<style>:root{color:#161616}@media(prefers-color-scheme:dark){:root{color:#f5f5f5}}</style>' : `<rect width="100%" height="100%" rx="${(optical + 8) / 5}" fill="#161616"/>`}<g transform="translate(4 4)" fill="none" stroke="${adaptive ? 'currentColor' : '#f5f5f5'}" stroke-width="${master.weight}" stroke-linejoin="round"><path d="${master.path}"/></g></svg>`;
}
await writeFile(resolve(root, "apps/web/public/favicon.svg"), svg(32, true));
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
} finally { await browser.close(); }
for (const size of [16,24,32,48,64,128,256,512]) await writeFile(resolve(root, `apps/desktop/build/icons/${size}x${size}.png`), pngs.get(size));
for (const f of ["apps/desktop/build/icon.png", "apps/desktop/resources/icon.png"]) await writeFile(resolve(root, f), pngs.get(1024));
for (const [file, size] of [["icon-192.png",192],["icon-512.png",512],["icon-maskable-512.png",512],["apple-touch-icon.png",180]]) await writeFile(resolve(root, `apps/web/public/icons/${file}`), pngs.get(size));
const icon = pngs.get(256);
const ico = Buffer.alloc(22); ico.writeUInt16LE(1,2); ico.writeUInt16LE(1,4); ico.writeUInt16LE(1,10); ico.writeUInt16LE(32,12); ico.writeUInt32LE(icon.length,14); ico.writeUInt32LE(22,18);
await writeFile(resolve(root,"apps/desktop/build/icon.ico"), Buffer.concat([ico,icon]));
const chunks = [["ic07",128],["ic08",256],["ic09",512],["ic10",1024]].map(([tag,size]) => { const data=pngs.get(size), header=Buffer.alloc(8); header.write(tag);header.writeUInt32BE(data.length+8,4);return Buffer.concat([header,data]); });
const header=Buffer.alloc(8);header.write("icns");header.writeUInt32BE(8+chunks.reduce((sum,x)=>sum+x.length,0),4);
await writeFile(resolve(root,"apps/desktop/build/icon.icns"), Buffer.concat([header,...chunks]));
console.log("Generated Web/PWA and Desktop PNG, ICO and ICNS icons from ProductMark.");
