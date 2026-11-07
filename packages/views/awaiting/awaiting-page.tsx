"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Loader2, MessageSquare } from "lucide-react";
import { toast } from "sonner";
import { ApiError } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  awaitingIssuesOptions,
  awaitingKind,
  awaitingUnmarkedOptions,
  groupAwaiting,
  ReviewFeedbackDeliveryError,
  reviewSince,
  reviewSnapshot,
  sendReviewFeedback,
  useReviewUpdate,
} from "@multica/core/issues";
import { useActorName } from "@multica/core/workspace/hooks";
import type { Issue } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Popover, PopoverContent, PopoverTrigger } from "@multica/ui/components/ui/popover";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { cn } from "@multica/ui/lib/utils";
import { ActorAvatar } from "../common/actor-avatar";
import { PageHeader, PAGE_GUTTER } from "../layout/page-header";
import { IssuePeekHost } from "../issues/components/issue-peek";
import { PEEK_TARGET_ATTR, useIssuePeekActions } from "../issues/surface/peek-context";
import { useT } from "../i18n";

const NO_ISSUES: Issue[] = [];

function ReviewWait({ issue }: { issue: Issue }) {
  const { t } = useT("issues");
  const [, tick] = useState(0);
  useEffect(() => { const id = setInterval(() => tick((n) => n + 1), 60_000); return () => clearInterval(id); }, []);
  const minutes = Math.max(0, Math.floor((Date.now() - Date.parse(reviewSince(issue))) / 60000));
  const time = minutes < 1 ? t(($) => $.awaiting.just_now)
    : minutes < 60 ? t(($) => $.awaiting.minutes, { count: minutes })
    : minutes < 1440 ? t(($) => $.awaiting.hours, { count: Math.floor(minutes / 60) })
    : t(($) => $.awaiting.days, { count: Math.floor(minutes / 1440) });
  return <span className={cn("shrink-0", minutes > 4320 && "text-warning")} title={reviewSince(issue)}>{t(($) => $.awaiting.wait, { time })}</span>;
}

/**
 * The three ways a card hands work back: send a rejection, answer a question
 * (reply), or confirm an off-system action (act). All three follow the R2
 * send-back chain: one top-level member comment carries the words to the
 * deliverer, then the card returns to in_progress.
 */
type ReturnKind = "reject" | "reply" | "act";

function ReturnControl({ issue, kind, owner }: { issue: Issue; kind: ReturnKind; owner: string | null }) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const update = useReviewUpdate(wsId);
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState<{ id: string; revision?: number } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [conflict, setConflict] = useState(false);
  const action = typeof issue.metadata.review_action === "string" ? issue.metadata.review_action.trim() : "";
  const busy = sending || update.isPending;
  // "Act" may be confirmed with no words: the deliverer gets the default
  // sentence, which is still a real hand-back and still carries the action.
  const optional = kind === "act";
  const trigger = kind === "reject" ? t(($) => $.awaiting.reject)
    : kind === "reply" ? t(($) => $.awaiting_r4.reply)
    : t(($) => $.awaiting_r4.done);
  const label = kind === "reject" ? t(($) => $.awaiting.feedback_label)
    : kind === "reply" ? t(($) => $.awaiting_r4.reply_label)
    : t(($) => $.awaiting_r4.done_label);
  const placeholder = kind === "reject" ? t(($) => $.awaiting.reason)
    : kind === "reply" ? t(($) => $.awaiting_r4.reply_placeholder)
    : action ? t(($) => $.awaiting_r4.act_default, { action }) : t(($) => $.awaiting_r4.done);

  function content() {
    const trimmed = text.trim();
    if (trimmed || !optional) return trimmed;
    return action ? t(($) => $.awaiting_r4.act_default, { action }) : t(($) => $.awaiting_r4.done);
  }

  async function send() {
    setSending(true); setError(null);
    let feedback = sent;
    try {
      if (!feedback) {
        const comment = await sendReviewFeedback(issue, content());
        feedback = { id: comment.id, revision: comment.issue_revision };
        setSent(feedback);
      }
      if (!feedback.revision) throw new Error("Missing comment issue revision");
      await update.mutateAsync({ id: issue.id, status: "in_progress", expected_revision: feedback.revision });
      setOpen(false);
    } catch (err) {
      if (err instanceof ReviewFeedbackDeliveryError) {
        setError(kind === "reject"
          ? t(($) => err.isNote ? $.awaiting.note_not_delivered : $.awaiting.not_delivered, { name: owner ?? "" })
          : t(($) => err.isNote ? $.awaiting_r4.note_not_delivered : $.awaiting_r4.not_delivered, { name: owner ?? "" }));
      } else if (err instanceof ApiError && err.status === 409) {
        setConflict(true); setError(t(($) => $.awaiting.conflict));
        toast.error(t(($) => $.awaiting.changed));
      } else setError(t(($) => feedback ? $.awaiting.partial : $.awaiting.failed));
    } finally { setSending(false); }
  }

  return <Popover open={open} onOpenChange={setOpen}>
    <PopoverTrigger render={<Button variant={kind === "reject" ? "ghost" : "outline"} size="sm" disabled={busy} />}>
      {kind === "reply" && <MessageSquare className="size-3.5" />}{kind === "act" && <Check className="size-3.5" />}{trigger}
    </PopoverTrigger>
    <PopoverContent align="end" className="w-80 space-y-3">
      <Textarea aria-label={label} placeholder={placeholder} value={text} onChange={(e) => setText(e.target.value)} disabled={!!sent || sending} required={!optional} className="min-h-24" />
      <p className="text-caption text-muted-foreground">{owner ? t(($) => $.awaiting.recipient, { name: owner }) : t(($) => $.awaiting.unassigned)}</p>
      {error && <p role="alert" className="text-caption text-destructive">{error}</p>}
      <div className="flex justify-end"><Button size="sm" disabled={(!optional && !text.trim()) || busy || conflict} aria-busy={busy} onClick={() => void send()}>{busy && <Loader2 className="size-3.5 animate-spin" />}{sent ? t(($) => $.awaiting.retry_status) : trigger}</Button></div>
    </PopoverContent>
  </Popover>;
}

export function AwaitingRow({ issue }: { issue: Issue }) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const peek = useIssuePeekActions();
  const update = useReviewUpdate(wsId);
  const { getActorName } = useActorName();
  // A card the server marked as unroutable only explains itself; it must not
  // offer a status write that would fake a resolution (B10).
  const problem = issue.awaiting_problem;
  const kind = problem ? null : awaitingKind(issue);
  const busy = update.isPending;
  const owner = issue.assignee_id && issue.assignee_type ? getActorName(issue.assignee_type, issue.assignee_id) : null;
  const action = typeof issue.metadata.review_action === "string" ? issue.metadata.review_action.trim() : "";

  async function approve() {
    try {
      if (!issue.revision) throw new Error("Missing issue revision");
      const snapshot = reviewSnapshot(issue);
      const approved = await update.mutateAsync({ id: issue.id, status: "done", expected_revision: issue.revision });
      const expires = Date.now() + 5000;
      toast.success(t(($) => $.awaiting.approved, { identifier: issue.identifier }), {
        duration: 5000,
        action: { label: t(($) => $.awaiting.undo), onClick: () => {
          if (Date.now() > expires) return;
          void update.mutateAsync({ id: issue.id, status: "in_review", expected_revision: approved.revision, review_metadata: snapshot })
            .catch((err: unknown) => { toast.error(err instanceof ApiError && err.status === 409 ? t(($) => $.awaiting.changed) : t(($) => $.awaiting.failed)); });
        } },
      });
    } catch (err) { toast.error(err instanceof ApiError && err.status === 409 ? t(($) => $.awaiting.changed) : t(($) => $.awaiting.failed)); }
  }

  return <li className="flex items-center gap-4 border-b border-border/60 py-5" {...{ [PEEK_TARGET_ATTR]: issue.id }}>
    <button type="button" className="min-w-0 flex-1 rounded-md text-left outline-none focus-visible:ring-2 focus-visible:ring-ring" onClick={() => peek?.open(issue.id)}>
      <p className={cn("line-clamp-2 text-body font-medium leading-relaxed", !action && "text-muted-foreground")}>{action || t(($) => $.awaiting.missing_action)}</p>
      <span className="mt-2 flex min-w-0 items-center gap-x-2 text-caption text-muted-foreground">
        <span className="shrink-0">{issue.identifier}</span><span aria-hidden>·</span>
        <span className="min-w-0 truncate">{issue.title}</span>
        {owner && <><span aria-hidden>·</span><span className="inline-flex max-w-36 shrink-0 items-center gap-1.5 truncate"><ActorAvatar actorType={issue.assignee_type!} actorId={issue.assignee_id!} size="sm" profileLink={false} /><span className="truncate">{owner}</span></span></>}
        <span aria-hidden>·</span><ReviewWait issue={issue} />
      </span>
      {problem && <p className="mt-1 text-caption text-destructive">{problem}</p>}
    </button>
    <div className="flex shrink-0 items-center gap-2" onClick={(e) => e.stopPropagation()}>
      {kind === "approve" && <><ReturnControl issue={issue} kind="reject" owner={owner} /><Button variant="outline" size="sm" disabled={busy} aria-busy={busy} onClick={() => void approve()}><Check className="size-3.5" />{t(($) => $.awaiting.approve)}</Button></>}
      {kind === "reply" && <ReturnControl issue={issue} kind="reply" owner={owner} />}
      {kind === "act" && <ReturnControl issue={issue} kind="act" owner={owner} />}
    </div>
  </li>;
}

/**
 * The list body, data in: three groups in the order reply -> act -> approve,
 * oldest wait first, empty groups hidden, plus the unmarked footer that
 * expands in place. Kept separate from the queries so it renders in tests and
 * so the account/mine/unmarked windows stay one obvious composition.
 */
export function AwaitingSections({ mine, unmarked }: { mine: Issue[]; unmarked: Issue[] }) {
  const { t } = useT("issues");
  const groups = groupAwaiting(mine);
  const [expanded, setExpanded] = useState(false);
  return <div className="space-y-8">
    {groups.length === 0
      ? <div className="flex flex-col items-center gap-3 py-24 text-muted-foreground"><Check className="size-8" strokeWidth={1.5} /><p className="text-body">{t(($) => $.awaiting.empty)}</p></div>
      : groups.map((group) => {
        const title = group.kind === "reply" ? t(($) => $.awaiting_r4.group_reply)
          : group.kind === "act" ? t(($) => $.awaiting_r4.group_act)
          : t(($) => $.awaiting_r4.group_approve);
        const id = `awaiting-group-${group.kind}`;
        return <section key={group.kind} aria-labelledby={id}>
          <h2 id={id} className="flex items-baseline gap-2 pb-1 text-caption font-medium text-muted-foreground">{title}<span className="tabular-nums">{group.issues.length}</span></h2>
          <ul aria-labelledby={id}>{group.issues.map((issue) => <AwaitingRow key={issue.id} issue={issue} />)}</ul>
        </section>;
      })}
    {unmarked.length > 0 && <section className="border-t border-border/60 pt-4">
      <button type="button" aria-expanded={expanded} onClick={() => setExpanded((open) => !open)} className="rounded-md text-left text-caption text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring">
        {t(($) => $.awaiting_r4.unmarked, { count: unmarked.length })}
      </button>
      {expanded && <ul className="mt-1">{unmarked.map((issue) => <AwaitingRow key={issue.id} issue={issue} />)}</ul>}
    </section>}
  </div>;
}

function AwaitingList() {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const userId = useAuthStore((s) => s.user?.id ?? "");
  const mine = useQuery(awaitingIssuesOptions(wsId, userId));
  const unmarked = useQuery(awaitingUnmarkedOptions(wsId, userId));
  const peek = useIssuePeekActions();
  const mineIssues = mine.data ?? NO_ISSUES;
  useEffect(() => { peek?.publishColumns([mineIssues.map((issue) => issue.id)]); }, [mineIssues, peek]);
  return <div className={cn("mx-auto w-full max-w-4xl py-6", PAGE_GUTTER)}>
    {mine.error ? <div role="alert" className="flex items-center gap-3 text-caption text-muted-foreground">{t(($) => $.awaiting.failed)}<Button variant="outline" size="sm" onClick={() => void mine.refetch()}>{t(($) => $.awaiting.retry)}</Button></div>
      : mine.isPending ? <div className="space-y-6"><Skeleton className="h-20 w-full" /><Skeleton className="h-20 w-full" /></div>
      : <AwaitingSections mine={mineIssues} unmarked={unmarked.data ?? NO_ISSUES} />}
  </div>;
}

export function AwaitingPage() {
  const { t } = useT("issues");
  return <div className="flex h-full min-h-0 flex-col"><PageHeader><h1 className="text-body font-medium">{t(($) => $.awaiting.title)}</h1></PageHeader><IssuePeekHost><div className="min-h-0 flex-1 overflow-y-auto"><AwaitingList /></div></IssuePeekHost></div>;
}
