"use client";

import { useLayoutEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { cn } from "@multica/ui/lib/utils";
import { Dialog, DialogContent } from "@multica/ui/components/ui/dialog";
import type { IssueAssigneeType } from "@multica/core/types";
import {
  useCreateModeStore,
  type CreateMode,
} from "@multica/core/issues/stores/create-mode-store";
import { AgentCreatePanel } from "./quick-create-issue";
import { ManualCreatePanel, manualDialogContentClass } from "./create-issue";
import { sourceContextPreviewOptions } from "@multica/core/issues/queries";
import {
  useIssueDraftStore,
  type IssueCreateManual,
} from "@multica/core/issues/stores/draft-store";
import { useWorkspaceId } from "@multica/core/hooks";

/**
 * Text a caller wants the manual form to start with, carried on the modal
 * store as "prefill" (CLAWSTA-151 B13 one-click child issue). It is only the
 * card's own text and the chosen owner: review marks, the dispatcher and
 * attachments never travel with it.
 */
export interface CreateIssuePrefill {
  title?: string;
  description?: string;
  assigneeType?: IssueAssigneeType;
  assigneeId?: string;
}

const ASSIGNEE_TYPES: readonly IssueAssigneeType[] = ["member", "agent", "squad"];

function readPrefill(
  data?: Record<string, unknown> | null,
): CreateIssuePrefill | null {
  const raw = data?.prefill;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return null;
  const source = raw as Record<string, unknown>;
  const prefill: CreateIssuePrefill = {};
  if (typeof source.title === "string") prefill.title = source.title;
  if (typeof source.description === "string") prefill.description = source.description;
  if (
    typeof source.assigneeType === "string" &&
    ASSIGNEE_TYPES.includes(source.assigneeType as IssueAssigneeType)
  ) {
    prefill.assigneeType = source.assigneeType as IssueAssigneeType;
  }
  if (typeof source.assigneeId === "string") prefill.assigneeId = source.assigneeId;
  return prefill;
}

/**
 * Shell that owns the single `<Dialog>` AND `<DialogContent>` for the
 * create-issue flow. Mode switching unmounts/mounts only the inner panel
 * body — the Portal, Backdrop, and Popup all stay in the DOM, so Base UI
 * never replays the open animation. That's what makes the switch feel
 * instant; an earlier version put `<DialogContent>` inside each panel and
 * the close→open animation cycle still fired on every toggle.
 *
 * `initialMode` comes from the modal registry (`quick-create-issue` →
 * agent, `create-issue` → manual). Subsequent switches are local state
 * only and never round-trip through the modal store.
 *
 * Carry payload: when a panel switches mode it can hand a payload up via
 * `onSwitchMode`; the shell stores it as the next panel's `data` so seeding
 * works exactly like a fresh open.
 *
 * Manual-mode `isExpanded` is lifted up because it drives `DialogContent`'s
 * className — the className lives here in the shell since the Popup is here,
 * but the toggle for that state lives in the manual panel body.
 */
export function CreateIssueDialog({
  onClose,
  initialMode,
  data,
}: {
  onClose: () => void;
  initialMode: CreateMode;
  data?: Record<string, unknown> | null;
}) {
  const anchorCommentId = typeof data?.anchor_comment_id === "string"
    ? data.anchor_comment_id
    : null;
  const prefill = useMemo(() => readPrefill(data), [data]);
  if (anchorCommentId) {
    return (
      <SourceContextCreateIssueDialog
        onClose={onClose}
        initialMode={initialMode}
        data={data}
        anchorCommentId={anchorCommentId}
      />
    );
  }
  if (prefill) {
    return <PrefillCreateIssueDialog onClose={onClose} data={data} prefill={prefill} />;
  }
  return <CreateIssueDialogBody onClose={onClose} initialMode={initialMode} data={data} />;
}

/**
 * A caller-provided prefill runs in its own isolated draft, exactly like the
 * source-context create: the user's real create draft is saved, the manual form
 * starts on the prefill, and closing restores the draft untouched. The form is
 * forced to manual mode because the prefill is text for the manual editor — an
 * isolated draft otherwise starts on the agent form.
 */
function PrefillCreateIssueDialog({
  onClose,
  data,
  prefill,
}: {
  onClose: () => void;
  data?: Record<string, unknown> | null;
  prefill: CreateIssuePrefill;
}) {
  const [draftReady, setDraftReady] = useState(false);

  useLayoutEffect(() => {
    const store = useIssueDraftStore.getState();
    store.beginIsolatedDraft();
    store.setActiveMode("manual");
    const manual: Partial<IssueCreateManual> = {};
    if (prefill.title !== undefined) manual.title = prefill.title;
    if (prefill.description !== undefined) manual.description = prefill.description;
    if (prefill.assigneeType !== undefined) manual.assigneeType = prefill.assigneeType;
    if (prefill.assigneeId !== undefined) manual.assigneeId = prefill.assigneeId;
    store.setManual(manual);
    setDraftReady(true);
    return () => useIssueDraftStore.getState().endIsolatedDraft();
  }, [prefill]);

  if (!draftReady) return null;

  return <CreateIssueDialogBody onClose={onClose} initialMode="manual" data={data} />;
}

function SourceContextCreateIssueDialog({
  onClose,
  initialMode,
  data,
  anchorCommentId,
}: {
  onClose: () => void;
  initialMode: CreateMode;
  data?: Record<string, unknown> | null;
  anchorCommentId: string;
}) {
  const wsId = useWorkspaceId();
  const [draftReady, setDraftReady] = useState(false);
  const [sourceContextExpanded, setSourceContextExpanded] = useState(false);
  const sourceQuery = useQuery(sourceContextPreviewOptions(wsId, anchorCommentId));

  useLayoutEffect(() => {
    useIssueDraftStore.getState().beginIsolatedDraft();
    setDraftReady(true);
    return () => useIssueDraftStore.getState().endIsolatedDraft();
  }, []);

  if (!draftReady) return null;

  return (
    <CreateIssueDialogBody
      onClose={onClose}
      initialMode={initialMode}
      data={data}
      sourceContextExpanded={sourceContextExpanded}
      sourceContextData={{
        anchor_comment_id: anchorCommentId,
        source_context_preview: sourceQuery.isError || sourceQuery.isFetching
          ? undefined
          : sourceQuery.data,
        source_context_loading: sourceQuery.isLoading || sourceQuery.isFetching,
        source_context_failed: sourceQuery.isError,
        source_context_error: sourceQuery.error,
        source_context_refetch: sourceQuery.refetch,
        source_context_expanded: sourceContextExpanded,
        source_context_on_expanded_change: setSourceContextExpanded,
      }}
    />
  );
}

function CreateIssueDialogBody({
  onClose,
  initialMode,
  data,
  sourceContextData,
  sourceContextExpanded = false,
}: {
  onClose: () => void;
  initialMode: CreateMode;
  data?: Record<string, unknown> | null;
  sourceContextData?: Record<string, unknown>;
  sourceContextExpanded?: boolean;
}) {
  const setLastMode = useCreateModeStore((s) => s.setLastMode);
  const [mode, setMode] = useState<CreateMode>(initialMode);
  const [panelData, setPanelData] = useState(data ?? null);
  const [isExpanded, setIsExpanded] = useState(false);
  const effectiveData = sourceContextData
    ? { ...(panelData ?? {}), ...sourceContextData }
    : panelData;

  const switchTo = (next: CreateMode) => (carry?: Record<string, unknown> | null) => {
    setLastMode(next);
    setPanelData(carry ?? null);
    setMode(next);
  };

  const className =
    mode === "agent"
      ? cn(
          "p-0 gap-0 flex flex-col overflow-hidden",
          "!top-1/2 !left-1/2 !-translate-x-1/2 !-translate-y-1/2",
          // Smooth size transition when switching modes — the manual mode
          // uses the same easing.
          "!transition-all !duration-300 !ease-out",
          // Phone gutter. The widths below are `!important` so they beat
          // DialogContent's own sizing — which also made them beat its
          // `max-w-[calc(100%-2rem)]` safety margin, leaving the card flush
          // against both screen edges on a 430px viewport (MUL-6236). Restore
          // the margin here and let the `sm:` widths take over above 640px.
          "!w-full !max-w-[calc(100vw-1.5rem)]",
          // Source-context create needs numeric collapsed/expanded endpoints
          // so its preview transition can interpolate the height. Ordinary
          // quick create has no expanding preview and keeps its original
          // content-driven height, capped for mobile browser chrome.
          isExpanded
            ? "!h-5/6 sm:!max-w-4xl"
            : sourceContextData
              ? sourceContextExpanded
                ? "!h-5/6 sm:!max-w-2xl"
                : "!h-96 sm:!max-w-xl"
              : "!max-h-[80dvh] sm:!max-w-xl",
        )
      : cn(
          manualDialogContentClass(isExpanded),
          sourceContextExpanded && "!h-5/6",
        );

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent
        finalFocus={false}
        showCloseButton={false}
        className={className}
      >
        {mode === "agent" ? (
          <AgentCreatePanel
            onClose={onClose}
            onSwitchMode={switchTo("manual")}
            data={effectiveData}
            isExpanded={isExpanded}
            setIsExpanded={setIsExpanded}
          />
        ) : (
          <ManualCreatePanel
            onClose={onClose}
            onSwitchMode={switchTo("agent")}
            data={effectiveData}
            isExpanded={isExpanded}
            setIsExpanded={setIsExpanded}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
