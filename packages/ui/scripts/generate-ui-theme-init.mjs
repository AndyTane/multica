import { readFileSync, writeFileSync } from "node:fs";
import { URL } from "node:url";

const source = readFileSync(new URL("../components/common/ui-theme-script.tsx", import.meta.url), "utf8");
const bootstrap = source.match(/UI_THEME_BOOTSTRAP = '([^']+)';/)?.[1];
if (!bootstrap) throw new Error("Canonical UI theme bootstrap was not found");
const target = new URL("../../../apps/desktop/src/renderer/public/ui-theme-init.js", import.meta.url);
writeFileSync(target, `/* global document, localStorage */ // Generated from packages/ui/components/common/ui-theme-script.tsx.\n${bootstrap}\n`);
