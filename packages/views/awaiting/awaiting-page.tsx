"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { ApiError } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { awaitingIssuesOptions, ReviewFeedbackDeliveryError, reviewSince, reviewSnapshot, sendReviewFeedback, useReviewUpdate } from "@multica/core/issues";
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

export function AwaitingRow({ issue }: { issue: Issue }) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const peek = useIssuePeekActions();
  const update = useReviewUpdate(wsId);
  const { getActorName } = useActorName();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState<{ id: string; revision?: number } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [conflict, setConflict] = useState(false);
  const owner = issue.assignee_id && issue.assignee_type ? getActorName(issue.assignee_type, issue.assignee_id) : null;
  const action = typeof issue.metadata.review_action === "string" ? issue.metadata.review_action.trim() : "";
  const busy = sending || update.isPending;
  const failed = (err: unknown) => {
    const changed = err instanceof ApiError && err.status === 409;
    return t(($) => changed ? $.awaiting.changed : $.awaiting.failed);
  };

  async function approve() {
    setError(null);
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
            .catch((err: unknown) => { toast.error(failed(err)); });
        } },
      });
    } catch (err) { toast.error(failed(err)); }
  }

  async function reject() {
    setSending(true); setError(null);
    let feedback = sent;
    try {
      if (!feedback) {
        const comment = await sendReviewFeedback(issue, reason);
        feedback = { id: comment.id, revision: comment.issue_revision };
        setSent(feedback);
      }
      if (!feedback.revision) throw new Error("Missing comment issue revision");
      await update.mutateAsync({ id: issue.id, status: "in_progress", expected_revision: feedback.revision });
      setOpen(false);
    } catch (err) {
      if (err instanceof ReviewFeedbackDeliveryError) {
        setError(t(($) => err.isNote ? $.awaiting.note_not_delivered : $.awaiting.not_delivered, { name: owner ?? "" }));
      } else if (err instanceof ApiError && err.status === 409) {
        setConflict(true); setError(t(($) => $.awaiting.conflict));
        toast.error(t(($) => $.awaiting.changed));
      } else setError(t(($) => feedback ? $.awaiting.partial : $.awaiting.failed));
    } finally { setSending(false); }
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
    </button>
    <div className="flex shrink-0 items-center gap-2" onClick={(e) => e.stopPropagation()}>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger render={<Button variant="ghost" size="sm" disabled={busy} />}>{t(($) => $.awaiting.reject)}</PopoverTrigger>
        <PopoverContent align="end" className="w-80 space-y-3">
          <Textarea aria-label={t(($) => $.awaiting.feedback_label)} placeholder={t(($) => $.awaiting.reason)} value={reason} onChange={(e) => setReason(e.target.value)} disabled={!!sent || sending} required className="min-h-24" />
          <p className="text-caption text-muted-foreground">{owner ? t(($) => $.awaiting.recipient, { name: owner }) : t(($) => $.awaiting.unassigned)}</p>
          {error && <p role="alert" className="text-caption text-destructive">{error}</p>}
          <div className="flex justify-end"><Button size="sm" disabled={!reason.trim() || busy || conflict} aria-busy={busy} onClick={() => void reject()}>{busy && <Loader2 className="size-3.5 animate-spin" />}{sent ? t(($) => $.awaiting.retry_status) : t(($) => $.awaiting.reject)}</Button></div>
        </PopoverContent>
      </Popover>
      <Button variant="outline" size="sm" disabled={busy || !!sent || conflict} aria-busy={update.isPending} onClick={() => void approve()}><Check className="size-3.5" />{t(($) => $.awaiting.approve)}</Button>
    </div>
  </li>;
}

function AwaitingList() {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const userId = useAuthStore((s) => s.user?.id ?? "");
  const { data, isPending, error, refetch } = useQuery(awaitingIssuesOptions(wsId, userId));
  const peek = useIssuePeekActions();
  useEffect(() => { peek?.publishColumns([data?.map((issue) => issue.id) ?? []]); }, [data, peek]);
  return <div className={cn("mx-auto w-full max-w-4xl py-6", PAGE_GUTTER)}>
    {error ? <div role="alert" className="flex items-center gap-3 text-caption text-muted-foreground">{t(($) => $.awaiting.failed)}<Button variant="outline" size="sm" onClick={() => void refetch()}>{t(($) => $.awaiting.retry)}</Button></div>
      : isPending ? <div className="space-y-6"><Skeleton className="h-20 w-full" /><Skeleton className="h-20 w-full" /></div>
      : data.length === 0 ? <div className="flex flex-col items-center gap-3 py-24 text-muted-foreground"><Check className="size-8" strokeWidth={1.5} /><p className="text-body">{t(($) => $.awaiting.empty)}</p></div>
      : <ul aria-label={t(($) => $.awaiting.title)}>{data.map((issue) => <AwaitingRow key={issue.id} issue={issue} />)}</ul>}
  </div>;
}

export function AwaitingPage() {
  const { t } = useT("issues");
  return <div className="flex h-full min-h-0 flex-col"><PageHeader><h1 className="text-body font-medium">{t(($) => $.awaiting.title)}</h1></PageHeader><IssuePeekHost><div className="min-h-0 flex-1 overflow-y-auto"><AwaitingList /></div></IssuePeekHost></div>;
}
