"use client";

import { useMemo, useState } from "react";
import { Lock, UserMinus } from "lucide-react";
import { toast } from "sonner";
import type { Agent, Issue, IssueAssigneeType, UpdateIssueRequest } from "@multica/core/types";
import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import { api } from "@multica/core/api";
import { isAgentRuntimeBound } from "@multica/core/agents";
import { useIssueDraftStore } from "@multica/core/issues/stores/draft-store";
import { useModalStore } from "@multica/core/modals";
import { canAssignAgentToIssue } from "@multica/core/permissions";
import { useActorName } from "@multica/core/workspace/hooks";
import { useWorkspaceId } from "@multica/core/hooks";
import { memberListOptions, agentListOptions, squadListOptions, assigneeFrequencyOptions } from "@multica/core/workspace/queries";
import { Button } from "@multica/ui/components/ui/button";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { ActorAvatar } from "../../../common/actor-avatar";
import { DeferredPopup } from "../../../common/deferred-popup";
import {
  PropertyPicker,
  PickerItem,
  PickerSection,
  PickerEmpty,
  PICKER_TRIGGER_CLASS,
} from "./property-picker";
import { useT } from "../../../i18n";
import { matchesPinyin } from "../../../editor/extensions/pinyin-match";

/**
 * Legacy boolean shape kept around for callers (e.g. `use-issue-actions.ts`)
 * that haven't migrated to the new `canAssignAgentToIssue` Decision API yet.
 * Internally redirects to the canonical rule so behaviour stays in sync.
 */
export function canAssignAgent(
  agent: Agent,
  userId: string | undefined,
  memberRole: string | undefined,
): boolean {
  return canAssignAgentToIssue(agent, {
    userId: userId ?? null,
    role: memberRole === "owner" || memberRole === "admin" || memberRole === "member"
      ? memberRole
      : null,
  }).allowed;
}

/** The assignee change the user picked, held while the preflight explains it. */
interface PendingAssignment {
  assigneeType: "agent" | "squad";
  assigneeId: string;
  /** Member who dispatched the card; empty only if the server contradicts B13. */
  dispatcherUserId: string;
}

interface AssigneePickerProps {
  assigneeType: IssueAssigneeType | null;
  assigneeId: string | null;
  /**
   * `true` when a batch selection spans different assignees ("mixed"): no row
   * is checked, including the unassigned row. Distinct from `assigneeType` /
   * `assigneeId` both being `null`, which means every selected issue is
   * genuinely unassigned and the unassigned row should be checked.
   */
  mixed?: boolean;
  onUpdate: (updates: Partial<UpdateIssueRequest>) => void;
  /**
   * The card whose assignee is being changed. CLAWSTA-151 F6 needs it for the
   * `assignment-preview` preflight and for the one-click "create my own
   * sub-issue" recovery. Callers that assign without a single card in scope
   * (create dialog, batch edit) omit it and keep the plain update path.
   */
  issue?: Pick<Issue, "id" | "identifier" | "title" | "description">;
  trigger?: React.ReactNode;
  triggerRender?: React.ReactElement<Record<string, unknown>>;
  open?: boolean;
  onOpenChange?: (v: boolean) => void;
  align?: "start" | "center" | "end";
}

/**
 * Mounting the real picker subscribes to members/agents/squads/frequency
 * queries — multiplied per board card / list row that cost froze tab
 * switches. Uncontrolled callers that bring their own trigger content get a
 * deferred lookalike trigger instead; the picker mounts on first interaction.
 * The default trigger needs `getActorName` (a members/agents subscription
 * itself), so trigger-less callers stay eager.
 */
export function AssigneePicker(props: AssigneePickerProps) {
  const hasDeferredTriggerContent =
    props.trigger !== undefined || props.triggerRender?.props.children != null;
  const canDefer =
    props.open === undefined &&
    props.onOpenChange === undefined &&
    hasDeferredTriggerContent;
  if (!canDefer) {
    return <AssigneePickerImpl {...props} />;
  }
  return (
    <DeferredPopup
      trigger={props.trigger}
      triggerRender={props.triggerRender}
      triggerClassName={PICKER_TRIGGER_CLASS}
    >
      {(open, onOpenChange) => (
        <AssigneePickerImpl {...props} open={open} onOpenChange={onOpenChange} />
      )}
    </DeferredPopup>
  );
}

function AssigneePickerImpl({
  assigneeType,
  assigneeId,
  mixed = false,
  onUpdate,
  issue,
  trigger: customTrigger,
  triggerRender,
  open: controlledOpen,
  onOpenChange: controlledOnOpenChange,
  align,
}: AssigneePickerProps) {
  const { t } = useT("issues");
  const [internalOpen, setInternalOpen] = useState(false);
  const open = controlledOpen ?? internalOpen;
  const setOpen = controlledOnOpenChange ?? setInternalOpen;
  const [filter, setFilter] = useState("");
  const [pending, setPending] = useState<PendingAssignment | null>(null);
  const [checking, setChecking] = useState(false);
  const user = useAuthStore((s) => s.user);
  const wsId = useWorkspaceId();
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: agents = [] } = useQuery(agentListOptions(wsId));
  const { data: squads = [] } = useQuery(squadListOptions(wsId));
  const { data: frequency = [] } = useQuery(assigneeFrequencyOptions(wsId));
  const { getActorName } = useActorName();

  const currentMember = members.find((m) => m.user_id === user?.id);
  const memberRole = currentMember?.role;

  // Build a lookup map from frequency data for sorting.
  const freqMap = useMemo(() => {
    const map = new Map<string, number>();
    for (const entry of frequency) {
      map.set(`${entry.assignee_type}:${entry.assignee_id}`, entry.frequency);
    }
    return map;
  }, [frequency]);

  const getFreq = (type: string, id: string) => freqMap.get(`${type}:${id}`) ?? 0;

  const query = filter.trim().toLowerCase();
  const filteredMembers = members
    .filter((m) => m.name.toLowerCase().includes(query) || matchesPinyin(m.name, query))
    .sort((a, b) => getFreq("member", b.user_id) - getFreq("member", a.user_id));
  const filteredAgents = agents
    .filter((a) => !a.archived_at && (a.name.toLowerCase().includes(query) || matchesPinyin(a.name, query)))
    .sort((a, b) => getFreq("agent", b.id) - getFreq("agent", a.id));
  const filteredSquads = squads
    .filter((s) => !s.archived_at && (s.name.toLowerCase().includes(query) || matchesPinyin(s.name, query)))
    .sort((a, b) => getFreq("squad", b.id) - getFreq("squad", a.id));
  const runnableAgentIds = new Set(
    agents
      .filter((agent) => !agent.archived_at && isAgentRuntimeBound(agent))
      .map((agent) => agent.id),
  );

  const isSelected = (type: string, id: string) =>
    assigneeType === type && assigneeId === id;

  const triggerLabel =
    assigneeType && assigneeId
      ? getActorName(assigneeType, assigneeId)
      : t(($) => $.pickers.assignee.trigger_unassigned);

  const applyAssignment = (type: "agent" | "squad", id: string) => {
    onUpdate({ assignee_type: type, assignee_id: id });
    setOpen(false);
  };

  // CLAWSTA-151 F6. The local permission check above still decides which rows
  // are reachable; this preflight adds the one thing the client cannot know —
  // whether the member who dispatched the card can still wake the candidate
  // after this run. It never authorizes the write: the update re-checks live.
  const requestAssignment = async (type: "agent" | "squad", id: string) => {
    if (checking) return;
    if (!issue) {
      applyAssignment(type, id);
      return;
    }
    setChecking(true);
    try {
      const preview = await api.previewIssueAssignment(issue.id, {
        assignee_type: type,
        assignee_id: id,
      });
      // Advisory only: a failed or older-server preview keeps the plain
      // transfer path instead of inventing a permission verdict.
      if (!preview) {
        applyAssignment(type, id);
        return;
      }
      if (!preview.actor_can_assign) {
        toast.error(t(($) => $.assign_preview.not_allowed, { name: getActorName(type, id) }));
        return;
      }
      if (preview.dispatcher_can_invoke === false) {
        setOpen(false);
        setPending({
          assigneeType: type,
          assigneeId: id,
          dispatcherUserId: preview.dispatcher_user_id ?? "",
        });
        return;
      }
      applyAssignment(type, id);
    } catch {
      applyAssignment(type, id);
    } finally {
      setChecking(false);
    }
  };

  // One-click recovery (B13): create a sub-issue of the SAME card, owned by the
  // chosen agent/squad, with the current text pre-filled. The original card's
  // assignee stays untouched, and the new card's dispatcher is this member.
  const createChildForPending = (assignment: PendingAssignment) => {
    if (!issue) return;
    useIssueDraftStore.getState().setManual({
      title: issue.title,
      description: issue.description ?? "",
      assigneeType: assignment.assigneeType,
      assigneeId: assignment.assigneeId,
    });
    useModalStore.getState().open("create-issue", {
      parent_issue_id: issue.id,
      parent_issue_identifier: issue.identifier,
    });
    setPending(null);
  };

  return (
    <>
      <PropertyPicker
        open={open}
        onOpenChange={(v: boolean) => {
          setOpen(v);
          if (!v) setFilter("");
        }}
        width="w-64"
        align={align}
        searchable
        searchPlaceholder={t(($) => $.pickers.assignee.search_placeholder)}
        onSearchChange={setFilter}
        triggerRender={triggerRender}
        trigger={
          customTrigger ? customTrigger : assigneeType && assigneeId ? (
            <>
              <ActorAvatar actorType={assigneeType} actorId={assigneeId} size="sm" enableHoverCard showStatusDot />
              <span className="truncate">{triggerLabel}</span>
            </>
          ) : (
            <span className="text-muted-foreground">{t(($) => $.pickers.assignee.trigger_unassigned)}</span>
          )
        }
      >
        {/* Unassigned — always the first row, search active or not. Every
            picker in the app puts the empty value there, so "clear this field"
            never moves. */}
        <PickerItem
          emptyValue
          selected={!mixed && !assigneeType && !assigneeId}
          onClick={() => {
            onUpdate({ assignee_type: null, assignee_id: null });
            setOpen(false);
          }}
        >
          <UserMinus className="h-3.5 w-3.5 text-muted-foreground" />
          <span className="text-muted-foreground">{t(($) => $.pickers.assignee.trigger_unassigned)}</span>
        </PickerItem>

        {/* Members */}
        {filteredMembers.length > 0 && (
          <PickerSection label={t(($) => $.pickers.assignee.members_group)}>
            {filteredMembers.map((m) => (
              <PickerItem
                key={m.user_id}
                selected={isSelected("member", m.user_id)}
                onClick={() => {
                  onUpdate({
                    assignee_type: "member",
                    assignee_id: m.user_id,
                  });
                  setOpen(false);
                }}
              >
                <ActorAvatar actorType="member" actorId={m.user_id} size="sm" />
                <span className="truncate">{m.name}</span>
              </PickerItem>
            ))}
          </PickerSection>
        )}

        {/* Agents */}
        {filteredAgents.length > 0 && (
          <PickerSection label={t(($) => $.pickers.assignee.agents_group)}>
            {filteredAgents.map((a) => {
              const decision = canAssignAgentToIssue(a, {
                userId: user?.id ?? null,
                role:
                  memberRole === "owner" ||
                  memberRole === "admin" ||
                  memberRole === "member"
                    ? memberRole
                    : null,
              });
              const runtimeBound = isAgentRuntimeBound(a);
              const allowed = decision.allowed && runtimeBound;
              return (
                <PickerItem
                  key={a.id}
                  selected={isSelected("agent", a.id)}
                  disabled={!allowed}
                  tooltip={
                    !decision.allowed
                      ? decision.message
                      : !runtimeBound
                        ? t(($) => $.pickers.assignee.agent_runtime_required)
                        : undefined
                  }
                  onClick={() => {
                    if (!allowed) return;
                    void requestAssignment("agent", a.id);
                  }}
                >
                  <ActorAvatar actorType="agent" actorId={a.id} size="sm" showStatusDot />
                  <span className={`truncate ${allowed ? "" : "text-muted-foreground"}`}>{a.name}</span>
                  {a.visibility === "private" && (
                    <Lock className="ml-auto h-3 w-3 text-muted-foreground" />
                  )}
                </PickerItem>
              );
            })}
          </PickerSection>
        )}

        {/* Squads — group ownership; assigning to a squad routes the issue to
            its leader agent on the backend. */}
        {filteredSquads.length > 0 && (
          <PickerSection label={t(($) => $.pickers.assignee.squads_group)}>
            {filteredSquads.map((s) => {
              const runtimeBound = runnableAgentIds.has(s.leader_id);
              return (
                <PickerItem
                  key={s.id}
                  selected={isSelected("squad", s.id)}
                  disabled={!runtimeBound}
                  tooltip={
                    runtimeBound
                      ? undefined
                      : t(($) => $.pickers.assignee.squad_runtime_required)
                  }
                  onClick={() => {
                    if (!runtimeBound) return;
                    void requestAssignment("squad", s.id);
                  }}
                >
                  <ActorAvatar actorType="squad" actorId={s.id} size="sm" />
                  <span className="truncate">{s.name}</span>
                </PickerItem>
              );
            })}
          </PickerSection>
        )}

        {filteredMembers.length === 0 &&
          filteredAgents.length === 0 &&
          filteredSquads.length === 0 &&
          filter && <PickerEmpty />}
      </PropertyPicker>

      {pending && (
        <AssignmentPreviewDialog
          dispatcherName={
            pending.dispatcherUserId
              ? getActorName("member", pending.dispatcherUserId)
              : ""
          }
          targetName={getActorName(pending.assigneeType, pending.assigneeId)}
          onCancel={() => setPending(null)}
          onTransferAnyway={() => {
            applyAssignment(pending.assigneeType, pending.assigneeId);
            setPending(null);
          }}
          onCreateChild={() => createChildForPending(pending)}
        />
      )}
    </>
  );
}

/**
 * CLAWSTA-151 F6: the dispatcher cannot wake the candidate after this run, so
 * later automatic follow-ups would fail. The default action is NOT to transfer
 * — it is to create a sub-issue of my own and hand that to the candidate
 * (the original card keeps its owner). "Transfer anyway" stays available for
 * the cases where the member knows better.
 */
function AssignmentPreviewDialog({
  dispatcherName,
  targetName,
  onCancel,
  onTransferAnyway,
  onCreateChild,
}: {
  dispatcherName: string;
  targetName: string;
  onCancel: () => void;
  onTransferAnyway: () => void;
  onCreateChild: () => void;
}) {
  const { t } = useT("issues");
  return (
    <AlertDialog
      open
      onOpenChange={(v) => {
        if (!v) onCancel();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t(($) => $.assign_preview.title, { dispatcher: dispatcherName, name: targetName })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t(($) => $.assign_preview.description)}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel onClick={onCancel}>
            {t(($) => $.assign_preview.cancel)}
          </AlertDialogCancel>
          <Button variant="secondary" onClick={onTransferAnyway}>
            {t(($) => $.assign_preview.transfer_anyway)}
          </Button>
          <Button onClick={onCreateChild}>
            {t(($) => $.assign_preview.create_child)}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
