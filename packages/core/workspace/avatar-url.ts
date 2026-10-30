import { api } from "../api";

export function resolvePublicFileUrlWithBase(rawUrl: string | null | undefined, baseUrl: string): string | null {
  if (!rawUrl) return null;
  try {
    const host = new URL(rawUrl, baseUrl || "http://local.invalid").hostname.toLowerCase();
    if (["gravatar.com", "libravatar.org"].some(domain => host === domain || host.endsWith("." + domain))) return null;
  } catch {
    return null;
  }
  if (!rawUrl.startsWith("/")) return rawUrl;
  const trimmedBaseUrl = baseUrl.replace(/\/+$/, "");
  return `${trimmedBaseUrl}${rawUrl}`;
}

export function resolvePublicFileUrl(rawUrl: string | null | undefined): string | null {
  if (!rawUrl) return null;
  return resolvePublicFileUrlWithBase(rawUrl, api.getBaseUrl?.() ?? "");
}
