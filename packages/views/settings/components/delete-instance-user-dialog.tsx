"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { api } from "@multica/core/api";
import type { InstanceAdminUser } from "@multica/core/api/schemas";
import {
  instanceAdminKeys,
  instanceUserDeletePreviewOptions,
} from "@multica/core/instance-admin/queries";
import { useAccountErrorText, useT } from "../../i18n";

// Codes this client knows how to label. A code from a newer server is simply
// not rendered: the dialog still refuses to confirm because can_delete comes
// from the server, so an unknown blocker is never silently ignored.
const REMOVE_CODES = [
  "registration_invite",
  "password",
  "personal_access_token",
  "task_token",
  "membership",
  "workspace_invitation",
  "identity_binding",
  "notification_preference",
  "pinned_item",
  "feedback",
  "client_usage",
  "issue_subscription",
  "composio_connection",
  "seat_capacity",
  "chat_pin",
  "autopilot_collaborator",
  "autopilot_subscriber",
  "issue_view_preference",
  "issue_view",
  "account",
] as const;

const BLOCKER_CODES = [
  "workspace_owner",
  "runtime_owner",
  "agent_owner",
  "channel_installation",
  "lark_installation",
  "share_link_creator",
  "source_context_capture",
  "last_admin",
  "cannot_delete_self",
] as const;

type RemoveCode = (typeof REMOVE_CODES)[number];
type BlockerCode = (typeof BLOCKER_CODES)[number];

interface DeleteInstanceUserDialogProps {
  target: InstanceAdminUser | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Confirmation built on the delete preview: the same dependency decision the
 * server enforces inside its delete transaction is rendered here first.
 *
 * Confirmation stays disabled while the preview is loading, failed, malformed
 * or reports a blocker. The preview is never treated as an authorization — the
 * DELETE request re-checks everything and a conflict just refreshes this view.
 */
export function DeleteInstanceUserDialog({
  target,
  open,
  onOpenChange,
}: DeleteInstanceUserDialogProps) {
  const { t } = useT("settings");
  const errorText = useAccountErrorText();
  const qc = useQueryClient();
  const preview = useQuery({
    ...instanceUserDeletePreviewOptions(target?.id ?? ""),
    enabled: open && !!target,
  });
  const removeMutation = useMutation({
    mutationFn: (userId: string) => api.deleteInstanceUser(userId),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: instanceAdminKeys.users() });
      onOpenChange(false);
    },
    onError: () => {
      // A conflict means the account changed between preview and confirm.
      // Re-read before the operator can try again.
      void preview.refetch();
    },
  });

  const blockers = preview.data?.blockers ?? [];
  const canDelete =
    preview.isSuccess &&
    !preview.isError &&
    preview.data?.can_delete === true &&
    blockers.length === 0;

  const labelForRemove = (code: string) => {
    const key = REMOVE_CODES.find((candidate) => candidate === code);
    return key ? t(($) => $.instance_admin.remove[key as RemoveCode]) : null;
  };
  const labelForBlocker = (code: string) => {
    const key = BLOCKER_CODES.find((candidate) => candidate === code);
    return key ? t(($) => $.instance_admin.blocker[key as BlockerCode]) : null;
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t(($) => $.instance_admin.delete_title, {
              name: target?.name || target?.email || "",
            })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t(($) => $.instance_admin.delete_description)}
          </AlertDialogDescription>
        </AlertDialogHeader>

        {preview.isLoading ? (
          <p className="text-body text-muted-foreground">
            {t(($) => $.instance_admin.loading)}
          </p>
        ) : null}

        {preview.isError ? (
          <div className="space-y-2">
            <p role="alert" className="text-body text-destructive">
              {errorText(preview.error)}
            </p>
            <p className="text-caption text-muted-foreground">
              {t(($) => $.instance_admin.delete_preview_failed)}
            </p>
            <AlertDialogAction
              variant="outline"
              size="sm"
              type="button"
              onClick={() => void preview.refetch()}
            >
              {t(($) => $.instance_admin.delete_retry)}
            </AlertDialogAction>
          </div>
        ) : null}

        {preview.isSuccess && preview.data ? (
          <div className="space-y-4">
            {blockers.length > 0 ? (
              <div className="space-y-2">
                <p className="text-body font-medium">
                  {t(($) => $.instance_admin.delete_blockers)}
                </p>
                <ul className="space-y-1 text-body text-destructive">
                  {blockers.map((blocker) => {
                    const label = labelForBlocker(blocker.code);
                    if (!label) return null;
                    return (
                      <li key={blocker.code}>
                        {label}
                        {blocker.count > 1 ? ` (${blocker.count})` : null}
                      </li>
                    );
                  })}
                </ul>
                <p className="text-caption text-muted-foreground">
                  {t(($) => $.instance_admin.blocked_hint)}
                </p>
              </div>
            ) : null}

            {preview.data.removes.length > 0 ? (
              <div className="space-y-2">
                <p className="text-body font-medium">
                  {t(($) => $.instance_admin.delete_removes)}
                </p>
                <ul className="space-y-1 text-body text-muted-foreground">
                  {preview.data.removes.map((code) => {
                    const label = labelForRemove(code);
                    if (!label) return null;
                    return <li key={code}>{label}</li>;
                  })}
                </ul>
              </div>
            ) : null}
          </div>
        ) : null}

        {removeMutation.isError ? (
          <p role="alert" className="text-body text-destructive">
            {errorText(removeMutation.error)}
          </p>
        ) : null}

        <AlertDialogFooter>
          <AlertDialogCancel type="button" disabled={removeMutation.isPending}>
            {t(($) => $.instance_admin.done)}
          </AlertDialogCancel>
          <AlertDialogAction
            type="button"
            variant="destructive"
            disabled={!canDelete || removeMutation.isPending}
            onClick={() => {
              if (!target || !canDelete) return;
              removeMutation.mutate(target.id);
            }}
          >
            {t(($) => $.instance_admin.delete_confirm)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
