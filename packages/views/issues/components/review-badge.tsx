"use client";

import { useAuthStore } from "@multica/core/auth";
import { useActorName } from "@multica/core/workspace/hooks";
import type { Issue } from "@multica/core/types";
import { Tooltip, TooltipTrigger, TooltipContent } from "@multica/ui/components/ui/tooltip";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";

/**
 * CLAWSTA-151 R4: a card can wait on a person in two statuses — `in_review`
 * (an approval, the R1 behaviour) and `blocked` (a reply or a manual step).
 * `review_kind` and the dispatcher fields ship with the R4 backend; they are
 * read straight off `issue.metadata` (a flat KV map) so this component does
 * not depend on the core Issue field definition owned by a parallel R4b card.
 * Integration can fold the same read into `isAwaitingMe`.
 */
function reviewKind(metadata: Issue["metadata"]): "approve" | "reply" | "act" | null {
  const kind = metadata.review_kind;
  if (kind === "approve" || kind === "reply" || kind === "act") return kind;
  return null;
}

function awaitingMe(issue: Issue, meUserId: string): boolean {
  if (!meUserId) return false;
  if (issue.status !== "in_review" && issue.status !== "blocked") return false;
  if (issue.metadata.review_by !== "user") return false;
  const reviewUser = issue.metadata.review_user;
  return reviewUser === undefined || reviewUser === meUserId;
}

/**
 * Which badge this card earns, or null when it carries no usable review mark.
 * B7 only allows `in_review` + user (approve) / specialist, and
 * `blocked` + user + reply/act. A blocked card with anything else is an
 * invalid combination — render nothing instead of guessing (B10).
 */
function badgeKind(issue: Issue): "approve" | "specialist" | "reply" | "act" | null {
  const reviewBy = issue.metadata.review_by;
  if (issue.status === "in_review") {
    if (reviewBy === "specialist") return "specialist";
    if (reviewBy === "user") return "approve";
    return null;
  }
  if (issue.status === "blocked") {
    const kind = reviewKind(issue.metadata);
    if (reviewBy === "user" && (kind === "reply" || kind === "act")) return kind;
    return null;
  }
  return null;
}

export function ReviewBadge({ issue, actionOnly = false }: { issue: Issue; actionOnly?: boolean }) {
  if (!badgeKind(issue)) return null;
  return <ReviewBadgeContent issue={issue} actionOnly={actionOnly} />;
}

function ReviewBadgeContent({ issue, actionOnly }: { issue: Issue; actionOnly: boolean }) {
  const { t } = useT("issues");
  const userId = useAuthStore((s) => s.user?.id ?? "");
  const { getActorName } = useActorName();
  const kind = badgeKind(issue);
  const mine = awaitingMe(issue, userId);
  if (actionOnly) return mine && typeof issue.metadata.review_action === "string"
    ? <span className="mt-1 block truncate text-caption text-foreground">{issue.metadata.review_action}</span> : null;
  const specialist = kind === "specialist";
  const recipient = specialist ? issue.metadata.review_agent : issue.metadata.review_user;
  const name = typeof recipient === "string" ? getActorName(specialist ? "agent" : "member", recipient) : "";
  // The approval and the "waiting on someone else" wording are unchanged from
  // R1; only the two blocked states for the current member get new copy.
  const label = specialist
    ? t(($) => $.awaiting.specialist)
    : !mine
      ? t(($) => $.awaiting.other, { name })
      : kind === "reply"
        ? t(($) => $.review_badge_r4.reply_you)
        : kind === "act"
          ? t(($) => $.review_badge_r4.act_you)
          : t(($) => $.awaiting.you);
  const badge = <span className={cn("inline-flex max-w-40 shrink-0 items-center truncate rounded-sm px-1.5 py-0.5 text-micro", mine ? "bg-brand/10 text-brand" : "text-muted-foreground", specialist && "border border-border")}>{label}</span>;
  return specialist && name ? <Tooltip><TooltipTrigger render={badge} /><TooltipContent>{t(($) => $.awaiting.specialist_hint, { name })}</TooltipContent></Tooltip> : badge;
}
