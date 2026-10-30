"use client";

import { useWorkspaceId } from "@multica/core/hooks";
import { useModalStore } from "@multica/core/modals";
import { Button } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { useT } from "../i18n";

/** Retains local rejection/recovery feedback without any subscription lookup. */
export function IssueLimitUpgradeDialog() {
  const { t } = useT("modals");
  const { t: common } = useT("common");
  const wsId = useWorkspaceId();
  const visible = useModalStore((s) => s.issueLimitRecoveryWorkspaceId === wsId);
  const reason = useModalStore((s) => s.issueLimitRecoveryReason);
  const dismiss = useModalStore((s) => s.dismissIssueLimitRecovery);
  return <Dialog open={visible} onOpenChange={(open) => { if (!open) dismiss(); }}>
    <DialogContent><DialogHeader>
      <DialogTitle>{reason === "autopilot_quota" ? t(($) => $.create_issue.autopilot_quota_limit.title) : t(($) => $.create_issue.issue_limit.title)}</DialogTitle>
      <DialogDescription>{reason === "autopilot_quota" ? t(($) => $.create_issue.autopilot_quota_limit.billing_disabled_description) : t(($) => $.create_issue.issue_limit.billing_disabled_description)}</DialogDescription>
    </DialogHeader><DialogFooter><Button onClick={dismiss}>{common(($) => $.cancel)}</Button></DialogFooter></DialogContent>
  </Dialog>;
}
