"use client";

import { useMemo } from "react";
import { useOptionalNavigation } from "./context";

/**
 * Origin used for navigation, or `null` when the platform can't
 * name one (server render, desktop before runtime config lands, or a component
 * mounted outside a NavigationProvider).
 *
 * For absolute content-link recognition use `useAppOrigins`, which also accepts
 * the public frontend used by copied links. This singular API stays compatible.
 */
export function useAppOrigin(): string | null {
  const navigation = useOptionalNavigation();
  const getShareableUrl = navigation?.getShareableUrl;
  return useMemo(() => {
    if (!getShareableUrl) return null;
    try {
      return new URL(getShareableUrl("/")).origin;
    } catch {
      return null;
    }
  }, [getShareableUrl]);
}

/** Both entry points belong to this deployment; neither changes navigation. */
export function useAppOrigins(): readonly string[] {
  const navigation = useOptionalNavigation();
  const getShareableUrl = navigation?.getShareableUrl;
  const getExternalShareUrl = navigation?.getExternalShareUrl;
  return useMemo(() => {
    const origins = new Set<string>();
    for (const getUrl of [getShareableUrl, getExternalShareUrl]) {
      if (!getUrl) continue;
      try {
        const url = new URL(getUrl("/"));
        if (url.protocol === "http:" || url.protocol === "https:") {
          origins.add(url.origin);
        }
      } catch {
        // SSR and an unconfigured desktop return relative paths.
      }
    }
    return [...origins];
  }, [getShareableUrl, getExternalShareUrl]);
}
