"use client";

import { useAuthStore } from "@multica/core/auth";
import { awaitingKind, isAwaitingMe } from "@multica/core/issues";
import { useActorName } from "@multica/core/workspace/hooks";
import type { Issue } from "@multica/core/types";
import { Tooltip, TooltipTrigger, TooltipContent } from "@multica/ui/components/ui/tooltip";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";

/**
 * CLAWSTA-151 R4: a card can wait on a person in two statuses — in_review
 * (an approval, the R1 behaviour) and blocked (a reply or a manual step).
 *
 * The review mark is read through the same core helpers the "waiting on me"
 * list uses, so a card badge and its list row cannot disagree about who it
 * waits for: awaitingKind decides approve/reply/act from the status plus
 * review_kind, and isAwaitingMe applies the B9/B10 dispatcher rules. A
 * specialist mark stays a separate, read-only "awaiting review" badge.
 */
function isSpecialistMark(issue: Issue): boolean {
  return issue.status === "in_review" && issue.metadata?.review_by === "specialist";
}

function hasReviewMark(issue: Issue): boolean {
  return awaitingKind(issue) !== null || isSpecialistMark(issue);
}

export function ReviewBadge({ issue, actionOnly = false }: { issue: Issue; actionOnly?: boolean }) {
  if (!hasReviewMark(issue)) return null;
  return <ReviewBadgeContent issue={issue} actionOnly={actionOnly} />;
}

function ReviewBadgeContent({ issue, actionOnly }: { issue: Issue; actionOnly: boolean }) {
  const { t } = useT("issues");
  const userId = useAuthStore((s) => s.user?.id ?? "");
  const { getActorName } = useActorName();
  const kind = awaitingKind(issue);
  const specialist = isSpecialistMark(issue);
  const metadata = issue.metadata ?? {};
  const mine = !specialist && isAwaitingMe(issue, userId);
  if (actionOnly) return mine && typeof metadata.review_action === "string"
    ? <span className="mt-1 block truncate text-caption text-foreground">{metadata.review_action}</span> : null;
  // The receiver is the explicit review_user or — for a legacy card that never
  // recorded one — the dispatcher the server resolves (B9).
  const recipient = specialist
    ? metadata.review_agent
    : typeof metadata.review_user === "string"
      ? metadata.review_user
      : issue.dispatcher_user_id;
  const name = typeof recipient === "string" ? getActorName(specialist ? "agent" : "member", recipient) : "";
  // The approval and the "waiting on someone else" wording are unchanged from
  // R1; only the two blocked states for the current member get new copy.
  const label = specialist
    ? t(($) => $.awaiting.specialist)
    : mine
      ? kind === "reply"
        ? t(($) => $.review_badge_r4.reply_you)
        : kind === "act"
          ? t(($) => $.review_badge_r4.act_you)
          : t(($) => $.awaiting.you)
      : t(($) => $.awaiting.other, { name });
  const badge = <span className={cn("inline-flex max-w-40 shrink-0 items-center truncate rounded-sm px-1.5 py-0.5 text-micro", mine ? "bg-brand/10 text-brand" : "text-muted-foreground", specialist && "border border-border")}>{label}</span>;
  return specialist && name ? <Tooltip><TooltipTrigger render={badge} /><TooltipContent>{t(($) => $.awaiting.specialist_hint, { name })}</TooltipContent></Tooltip> : badge;
}
