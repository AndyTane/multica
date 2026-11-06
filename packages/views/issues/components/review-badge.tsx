"use client";

import { useAuthStore } from "@multica/core/auth";
import { isAwaitingMe } from "@multica/core/issues";
import { useActorName } from "@multica/core/workspace/hooks";
import type { Issue } from "@multica/core/types";
import { Tooltip, TooltipTrigger, TooltipContent } from "@multica/ui/components/ui/tooltip";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";

export function ReviewBadge({ issue, actionOnly = false }: { issue: Issue; actionOnly?: boolean }) {
  if (issue.status !== "in_review" || !["user", "specialist"].includes(String(issue.metadata?.review_by))) return null;
  return <ReviewBadgeContent issue={issue} actionOnly={actionOnly} />;
}

function ReviewBadgeContent({ issue, actionOnly }: { issue: Issue; actionOnly: boolean }) {
  const { t } = useT("issues");
  const userId = useAuthStore((s) => s.user?.id ?? "");
  const { getActorName } = useActorName();
  const mine = isAwaitingMe(issue, userId);
  if (actionOnly) return mine && typeof issue.metadata.review_action === "string"
    ? <span className="mt-1 block truncate text-caption text-foreground">{issue.metadata.review_action}</span> : null;
  const specialist = issue.metadata.review_by === "specialist";
  const recipient = specialist ? issue.metadata.review_agent : issue.metadata.review_user;
  const name = typeof recipient === "string" ? getActorName(specialist ? "agent" : "member", recipient) : "";
  const label = specialist ? t(($) => $.awaiting.specialist) : mine ? t(($) => $.awaiting.you) : t(($) => $.awaiting.other, { name });
  const badge = <span className={cn("inline-flex max-w-40 shrink-0 items-center truncate rounded-sm px-1.5 py-0.5 text-micro", mine ? "bg-brand/10 text-brand" : "text-muted-foreground", specialist && "border border-border")}>{label}</span>;
  return specialist && name ? <Tooltip><TooltipTrigger render={badge} /><TooltipContent>{t(($) => $.awaiting.specialist_hint, { name })}</TooltipContent></Tooltip> : badge;
}
