/** Validated operator-configured frontend origin; empty keeps current-origin sharing. */
export function normalizePublicAppUrl(value: unknown): string {
  if (typeof value !== "string" || /[\\\r\n\t?#]/.test(value)) return "";
  try {
    const raw = value.trim();
    // URL accepts missing slashes and an empty port; neither is our contract.
    if (!/^https?:\/\//i.test(raw) || raw.replace(/\/$/, "").endsWith(":")) return "";
    const url = new URL(raw);
    if (url.username || url.password || url.pathname !== "/" || url.port === "0") return "";
    const host = url.hostname.toLowerCase().replace(/\.$/, "");
    if (
      host === "localhost" || host.endsWith(".localhost") ||
      /^127\./.test(host) || host === "0.0.0.0" || host === "[::]" || host === "[::1]" ||
      /^\[::ffff:7f[0-9a-f]{2}:/.test(host) || host === "[::ffff:0:0]"
    ) return "";
    if (!host.startsWith("[") && (host.length > 253 || host.split(".").some(
      (label) => label.length > 63 || !/^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(label),
    ))) return "";
    return url.origin;
  } catch {
    return "";
  }
}
