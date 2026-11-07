import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useIssueDraftStore } from "@multica/core/issues/stores/draft-store";
import { useModalStore } from "@multica/core/modals";
import { renderWithI18n } from "../../../test/i18n";
import { AssigneePicker } from "./assignee-picker";

// CLAWSTA-151 F6: choosing an agent/squad runs the assignment-preview
// preflight and reacts to its THREE outcomes:
//   actor_can_assign=false        → still forbidden (no write, no dialog)
//   dispatcher_can_invoke=false   → warn + three buttons (create my sub-issue,
//                                   transfer anyway, cancel)
//   null                          → no prompt at all
// "Create my sub-issue" opens the normal create dialog pre-filled with the
// card's text, its parent, and the chosen owner.

const { previewMock, toastErrorMock } = vi.hoisted(() => ({
  previewMock: vi.fn(),
  toastErrorMock: vi.fn(),
}));

vi.mock("sonner", () => ({ toast: { error: toastErrorMock, success: vi.fn() } }));
vi.mock("@multica/core/api", () => ({
  api: { previewIssueAssignment: previewMock },
}));

const AGENTS = [
  { id: "agent-1", name: "Ops Bot", archived_at: null, visibility: "workspace" },
];
const SQUADS = [
  { id: "squad-1", name: "Platform Team", archived_at: null, leader_id: "agent-1" },
];
const MEMBERS = [{ user_id: "user-1", name: "Ada", role: "member" }];
const NAMES: Record<string, string> = {
  "agent:agent-1": "Ops Bot",
  "squad:squad-1": "Platform Team",
  "member:user-2": "Boss",
};

vi.mock("@tanstack/react-query", () => ({
  useQuery: ({ queryKey }: { queryKey: string[] }) => {
    if (queryKey[0] === "members") return { data: MEMBERS };
    if (queryKey[0] === "agents") return { data: AGENTS };
    if (queryKey[0] === "squads") return { data: SQUADS };
    return { data: [] };
  },
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace-1" }));
vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) =>
    selector({ user: { id: "user-1" } }),
}));
vi.mock("@multica/core/agents", () => ({ isAgentRuntimeBound: () => true }));
vi.mock("@multica/core/permissions", () => ({
  canAssignAgentToIssue: () => ({ allowed: true }),
}));
vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({
    getActorName: (type: string, id: string) => NAMES[`${type}:${id}`] ?? "Unknown",
  }),
}));
vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({ queryKey: ["members"] }),
  agentListOptions: () => ({ queryKey: ["agents"] }),
  squadListOptions: () => ({ queryKey: ["squads"] }),
  assigneeFrequencyOptions: () => ({ queryKey: ["frequency"] }),
}));
vi.mock("../../../common/actor-avatar", () => ({
  ActorAvatar: () => <span data-testid="actor-avatar" />,
}));

const ISSUE = {
  id: "issue-1",
  identifier: "CLAW-1",
  title: "Fix the build",
  description: "The build fails on Windows.",
};

function renderPicker(onUpdate: (updates: unknown) => void) {
  return renderWithI18n(
    <AssigneePicker
      assigneeType={null}
      assigneeId={null}
      onUpdate={onUpdate}
      issue={ISSUE}
      open
      onOpenChange={() => {}}
    />,
  );
}

const pickAgent = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(await screen.findByText("Ops Bot"));
};

beforeEach(() => {
  previewMock.mockReset();
  toastErrorMock.mockReset();
  useModalStore.getState().close();
  useIssueDraftStore.getState().clearDraft();
});
afterEach(cleanup);

describe("AssigneePicker assignment preflight", () => {
  it("still forbids the transfer when the caller cannot assign the candidate", async () => {
    const user = userEvent.setup();
    const onUpdate = vi.fn();
    previewMock.mockResolvedValue({
      actor_can_assign: false,
      dispatcher_user_id: "user-2",
      dispatcher_can_invoke: true,
      reason: "not_assignable",
    });
    renderPicker(onUpdate);

    await pickAgent(user);

    await waitFor(() => expect(previewMock).toHaveBeenCalledWith("issue-1", {
      assignee_type: "agent",
      assignee_id: "agent-1",
    }));
    expect(onUpdate).not.toHaveBeenCalled();
    expect(screen.queryByText("Transfer anyway")).toBeNull();
    expect(toastErrorMock).toHaveBeenCalledWith("You can't transfer this task to Ops Bot.");
  });

  it("warns and offers the three actions when the dispatcher cannot wake the candidate", async () => {
    const user = userEvent.setup();
    const onUpdate = vi.fn();
    previewMock.mockResolvedValue({
      actor_can_assign: true,
      dispatcher_user_id: "user-2",
      dispatcher_can_invoke: false,
    });
    renderPicker(onUpdate);

    await pickAgent(user);

    expect(await screen.findByText("Boss can't wake Ops Bot")).toBeTruthy();
    expect(screen.getByText("Later automatic follow-ups on this task will fail.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Create my own sub-issue for it" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Transfer anyway" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeTruthy();
    expect(onUpdate).not.toHaveBeenCalled();

    // "Transfer anyway" still applies the picked assignee.
    await user.click(screen.getByRole("button", { name: "Transfer anyway" }));
    expect(onUpdate).toHaveBeenCalledWith({ assignee_type: "agent", assignee_id: "agent-1" });
  });

  it("closes the warning without changing the assignee on Cancel", async () => {
    const user = userEvent.setup();
    const onUpdate = vi.fn();
    previewMock.mockResolvedValue({
      actor_can_assign: true,
      dispatcher_user_id: "user-2",
      dispatcher_can_invoke: false,
    });
    renderPicker(onUpdate);

    await pickAgent(user);
    await screen.findByText("Boss can't wake Ops Bot");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByText("Boss can't wake Ops Bot")).toBeNull();
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("creates a pre-filled sub-issue of the current card for the candidate", async () => {
    const user = userEvent.setup();
    const onUpdate = vi.fn();
    previewMock.mockResolvedValue({
      actor_can_assign: true,
      dispatcher_user_id: "user-2",
      dispatcher_can_invoke: false,
    });
    renderPicker(onUpdate);

    await pickAgent(user);
    await user.click(await screen.findByRole("button", { name: "Create my own sub-issue for it" }));

    // The create dialog opens on the current card with its parent chip and the
    // chosen owner; the original card's assignee is untouched.
    expect(useModalStore.getState().modal).toBe("create-issue");
    expect(useModalStore.getState().data).toEqual({
      parent_issue_id: "issue-1",
      parent_issue_identifier: "CLAW-1",
    });
    expect(useIssueDraftStore.getState().draft.manual).toMatchObject({
      title: "Fix the build",
      description: "The build fails on Windows.",
      assigneeType: "agent",
      assigneeId: "agent-1",
    });
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("does not prompt when there is no dispatcher (null is not a permission failure)", async () => {
    const user = userEvent.setup();
    const onUpdate = vi.fn();
    previewMock.mockResolvedValue({
      actor_can_assign: true,
      dispatcher_user_id: null,
      dispatcher_can_invoke: null,
    });
    renderPicker(onUpdate);

    await pickAgent(user);

    await waitFor(() => expect(onUpdate).toHaveBeenCalledWith({
      assignee_type: "agent",
      assignee_id: "agent-1",
    }));
    expect(screen.queryByText(/can't wake/)).toBeNull();
  });

  it("preflights squads by their leader too", async () => {
    const user = userEvent.setup();
    const onUpdate = vi.fn();
    previewMock.mockResolvedValue({
      actor_can_assign: true,
      dispatcher_user_id: "user-2",
      dispatcher_can_invoke: false,
    });
    renderPicker(onUpdate);

    await user.click(await screen.findByText("Platform Team"));

    await waitFor(() => expect(previewMock).toHaveBeenCalledWith("issue-1", {
      assignee_type: "squad",
      assignee_id: "squad-1",
    }));
    expect(await screen.findByText("Boss can't wake Platform Team")).toBeTruthy();
    expect(onUpdate).not.toHaveBeenCalled();
  });
});
