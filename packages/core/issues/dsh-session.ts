/** Accept the local web homepage only; launch-link credentials must never reach UI. */
export function normalizeDshWebUrl(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  try {
    const url = new URL(value);
    if (url.protocol !== "http:" || !["127.0.0.1", "localhost", "[::1]"].includes(url.hostname)
      || url.username || url.password || url.pathname !== "/" || url.search || url.hash) return undefined;
    return `${url.origin}/`;
  } catch {
    return undefined;
  }
}
