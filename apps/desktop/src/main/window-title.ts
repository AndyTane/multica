import type { BrowserWindow } from "electron";

const PRODUCT_NAME = "U Workers";

const TITLE_SUFFIX = ` — ${PRODUCT_NAME}`;

/**
 * OS window title for a renderer page title, e.g. `Issues` → `Issues — U Workers`.
 *
 * Renderer code keeps writing bare page names to `document.title` (several
 * writers, including shared views); the suffix is applied once here so they
 * cannot disagree. An empty title, or one that already is or ends with the
 * product name, stays as is.
 */
export function formatWindowTitle(pageTitle: string | null | undefined): string {
  const trimmed = pageTitle?.trim();
  if (!trimmed || trimmed === PRODUCT_NAME) return PRODUCT_NAME;
  if (trimmed.endsWith(TITLE_SUFFIX)) return trimmed;
  return `${trimmed}${TITLE_SUFFIX}`;
}

export function installWindowTitle(window: BrowserWindow): void {
  window.setTitle(formatWindowTitle(window.getTitle()));
  window.on("page-title-updated", (event, title) => {
    event.preventDefault();
    window.setTitle(formatWindowTitle(title));
  });
}
