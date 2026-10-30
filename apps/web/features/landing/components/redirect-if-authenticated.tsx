"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceList } from "@multica/core/workspace";
import { resolvePostAuthDestination, useHasOnboarded } from "@multica/core/paths";

/**
 * Client-side fallback redirect for authenticated visitors on the landing page.
 *
 * The proxy normally sends root visits to the recent workspace or login.
 * This component also resolves a session that becomes available after a
 * public page has mounted. Every deployment uses the same destination rules.
 *
 * Renders nothing. Uses `router.replace` so the landing page never enters
 * browser history for authenticated users.
 */
export function RedirectIfAuthenticated() {
  const router = useRouter();
  const user = useAuthStore((s) => s.user);
  const isLoading = useAuthStore((s) => s.isLoading);
  const hasOnboarded = useHasOnboarded();

  const { workspaces, ready } = useWorkspaceList({ enabled: !!user });

  useEffect(() => {
    if (isLoading || !user || !ready) return;
    router.replace(resolvePostAuthDestination(workspaces, hasOnboarded));
  }, [isLoading, user, ready, workspaces, hasOnboarded, router]);

  return null;
}
