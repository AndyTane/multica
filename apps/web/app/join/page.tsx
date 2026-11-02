"use client";

import { useEffect, useState, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { api, errorCode } from "@multica/core/api";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { Badge } from "@multica/ui/components/ui/badge";
import { useAuthStore } from "@multica/core/auth";
import { workspaceKeys } from "@multica/core/workspace/queries";
import type { ShareLinkInfo } from "@multica/core/types";

function JoinInner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const code = searchParams.get("code");
  const user = useAuthStore((s) => s.user);
  const refreshMe = useAuthStore((s) => s.refreshMe);
  const queryClient = useQueryClient();

  const [info, setInfo] = useState<ShareLinkInfo | null>(null);
  const [infoError, setInfoError] = useState<string | null>(null);
  const [joining, setJoining] = useState(false);
  const [joinError, setJoinError] = useState<string | null>(null);
  const [joinedWorkspace, setJoinedWorkspace] = useState<string | null>(null);
  const [syncError, setSyncError] = useState(false);

  // Load the share-link preview once. No auth needed: this endpoint only
  // exposes the workspace name/slug and the inviter.
  useEffect(() => {
    if (!code) {
      setInfoError("No invite code found. Please use a valid share link.");
      return;
    }
    let cancelled = false;
    api
      .getShareLinkInfo(code)
      .then((data) => {
        if (!cancelled) setInfo(data);
      })
      .catch(() => {
        if (!cancelled) setInfoError("This invite link is invalid or has expired.");
      });
    return () => {
      cancelled = true;
    };
  }, [code]);

  const syncAndEnter = async (workspace: string) => {
    setJoining(true);
    setSyncError(false);
    try {
      // The join also completes onboarding on the server. Publish that account
      // state before the workspace layout evaluates its onboarding guard.
      await Promise.all([
        refreshMe(),
        api.listWorkspaces().then((list) => {
          queryClient.setQueryData(workspaceKeys.list(), list);
        }),
      ]);
      router.push(`/${workspace}/issues`);
    } catch {
      // Membership is already saved; retry only these reads, never the join.
      setSyncError(true);
    } finally {
      setJoining(false);
    }
  };

  const handleJoin = async () => {
    if (!code) return;
    if (!user) {
      // Not logged in: send them to login, then return here to join.
      router.push(`/login?next=${encodeURIComponent(`/join?code=${code}`)}`);
      return;
    }
    setJoining(true);
    setJoinError(null);
    let workspace: string;
    try {
      const result = await api.joinByShareLink(code);
      workspace = result.workspace_slug || result.workspace_id;
    } catch (e) {
      const code = errorCode(e);
      const msg = e instanceof Error ? e.message : "";
      if (msg.includes("already a member") && info?.workspace_slug) {
        workspace = info.workspace_slug;
      } else {
        setJoining(false);
        if (code === "seat_capacity_full") {
          setJoinError("All purchased member seats are in use. Ask a workspace admin to add a seat before trying again.");
        } else if (code === "seat_capacity_unavailable") {
          setJoinError("Member capacity could not be verified. Please try again.");
        } else {
          setJoinError(msg || "Failed to join the workspace. The link may have expired.");
        }
        return;
      }
    }
    setJoinedWorkspace(workspace);
    await syncAndEnter(workspace);
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/30 p-4">
      <Card className="w-full max-w-md">
        <CardContent className="space-y-4 pt-6">
          {joinedWorkspace ? (
            <>
              <h1 className="text-title-lg font-semibold text-center">Joined!</h1>
              {syncError ? (
                <>
                  <p role="alert" className="text-center text-body text-destructive">
                    Your membership is saved, but your account could not be synced. Retry to enter the workspace.
                  </p>
                  <div className="flex justify-center">
                    <Button onClick={() => syncAndEnter(joinedWorkspace)} disabled={joining} aria-busy={joining}>
                      Retry sync
                    </Button>
                  </div>
                </>
              ) : (
                <p role="status" className="text-center text-muted-foreground">Redirecting to your workspace...</p>
              )}
            </>
          ) : infoError ? (
            <>
              <h1 className="text-title-lg font-semibold text-center">Oops</h1>
              <p className="text-center text-muted-foreground">{infoError}</p>
              <div className="flex justify-center pt-2">
                <Button variant="outline" onClick={() => router.push("/")}>
                  Go Home
                </Button>
              </div>
            </>
          ) : !info ? (
            <div className="py-6 text-center text-muted-foreground">Loading invite details...</div>
          ) : (
            <>
              <h1 className="text-title-lg font-semibold text-center">
                You&apos;re invited to {info.workspace_name}
              </h1>
              {info.creator_name && (
                <p className="text-center text-muted-foreground">
                  Invited by {info.creator_name}
                </p>
              )}
              <p className="flex items-center justify-center gap-2 text-center text-body text-muted-foreground">
                <span>You&apos;ll join this workspace as</span>
                <Badge variant="outline">
                  {info.role === "admin" ? "Administrator" : "Member"}
                </Badge>
              </p>
              {!user && (
                <p className="text-center text-body text-muted-foreground">
                  You&apos;ll need to log in to join this workspace.
                </p>
              )}
              {joinError && (
                <p className="text-center text-body text-destructive">{joinError}</p>
              )}
              <div className="flex justify-center gap-2 pt-2">
                <Button onClick={handleJoin} disabled={joining} aria-busy={joining}>
                  {joining ? "Joining..." : user ? "Join Workspace" : "Log In to Join"}
                </Button>
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export default function JoinPage() {
  return (
    <Suspense fallback={<div className="flex min-h-screen items-center justify-center">Loading...</div>}>
      <JoinInner />
    </Suspense>
  );
}
