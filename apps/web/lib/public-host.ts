// Public metadata is opt-in; request hosts and API addresses are not public URLs.
export function resolvePublicAppUrl(
  env: Record<string, string | undefined>,
): string | undefined {
  const value = env.NEXT_PUBLIC_APP_URL?.trim();
  if (!value) return undefined;
  try {
    const url = new URL(value);
    if (url.protocol !== "http:" && url.protocol !== "https:") return undefined;
    if (url.username || url.password) return undefined;
    url.search = "";
    url.hash = "";
    return url.toString().replace(/\/+$/, "");
  } catch {
    return undefined;
  }
}
