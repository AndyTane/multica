// Docs-site path prefix for a UI language. English docs are prefix-less;
// every other docs locale lives under `/docs/<lang>/`.
export function docsLocalePrefix(language?: string): string {
  if (language?.startsWith("zh")) return "/zh";
  if (language?.startsWith("ja")) return "/ja";
  if (language?.startsWith("ko")) return "/ko";
  if (language?.startsWith("fr")) return "/fr";
  return "";
}

/** Same-origin on web; the configured web app origin in the Desktop shell. */
export function docsHref(topic = "", language?: string, appUrl?: string): string {
  let base = appUrl;
  if (base === undefined && typeof window !== "undefined") {
    const bridge = (window as Window & { desktopAPI?: { runtimeConfig?: { ok: boolean; config?: { appUrl: string } } } }).desktopAPI;
    if (bridge?.runtimeConfig?.ok) base = bridge.runtimeConfig.config?.appUrl;
  }
  return `${base?.replace(/\/+$/, "") ?? ""}/docs${docsLocalePrefix(language)}${topic ? `/${topic.replace(/^\/+/, "")}` : ""}`;
}
