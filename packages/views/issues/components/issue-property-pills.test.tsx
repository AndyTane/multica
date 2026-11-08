import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Issue } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { IssuePropertyPills } from "./issue-property-pills";

// CLAWSTA-151 F6: the board side-peek property row is a single-card assignee
// entry point too, so it must hand the card to the transfer preflight.

const { previewMock } = vi.hoisted(() => ({ previewMock: vi.fn() }));

const MEMBERS = [{ user_id: "me", name: "Me", role: "admin" }];
const AGENTS = [
  {
    id: "agent-1",
    name: "Ops Bot",
    archived_at: null,
    visibility: "workspace",
    runtime_id: "runtime-1",
    runtime_bound: true,
    owner_id: null,
    permission_mode: "public_to",
    invocation_targets: [{ target_type: "workspace", target_id: "ws-1" }],
  },
];

vi.mock("@tanstack/react-query", () => ({
  useQuery: ({ queryKey }: { queryKey: string[] }) => {
    if (queryKey[0] === "members") return { data: MEMBERS };
    if (queryKey[0] === "agents") return { data: AGENTS };
    return { data: [] };
  },
}));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));

vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) =>
    selector({ user: { id: "me" } }),
}));

vi.mock("@multica/core/api", () => ({
  api: { previewIssueAssignment: previewMock },
}));

vi.mock("@multica/core/permissions", () => ({
  canAssignAgentToIssue: () => ({ allowed: true, message: "" }),
}));

vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({ getActorName: () => "Ops Bot" }),
}));

vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({ queryKey: ["members"] }),
  agentListOptions: () => ({ queryKey: ["agents"] }),
  squadListOptions: () => ({ queryKey: ["squads"] }),
  assigneeFrequencyOptions: () => ({ queryKey: ["frequency"] }),
}));

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

vi.mock("../../common/actor-avatar", () => ({
  ActorAvatar: () => <span data-testid="actor-avatar" />,
}));

vi.mock("../../projects/components/project-picker", () => ({
  ProjectPicker: () => null,
}));

// Only the assignee field is under test; the sibling pickers need a lot of
// unrelated context and are stubbed so the pill row can render.
vi.mock("./pickers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./pickers")>();
  return {
    ...actual,
    StatusPicker: () => null,
    PriorityPicker: () => null,
    DueDatePicker: () => null,
    StartDatePicker: () => null,
    LabelPicker: () => null,
  };
});

const ISSUE = {
  id: "issue-pill",
  workspace_id: "ws-1",
  number: 9,
  identifier: "CLAW-9",
  title: "Side peek card",
  description: null,
  status: "in_progress",
  priority: "none",
  assignee_type: null,
  assignee_id: null,
  creator_type: "member",
  creator_id: "me",
  parent_issue_id: null,
  project_id: null,
  position: 1,
  stage: null,
  start_date: null,
  due_date: null,
  metadata: {},
  properties: {},
  labels: [],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
} as Issue;

afterEach(cleanup);

describe("IssuePropertyPills assignee preflight", () => {
  it("preflights an assignee change with the card from the side-peek pills", async () => {
    previewMock.mockReset();
    previewMock.mockResolvedValue({
      actor_can_assign: true,
      dispatcher_user_id: null,
      dispatcher_can_invoke: null,
    });

    renderWithI18n(<IssuePropertyPills issue={ISSUE} onUpdate={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Unassigned" }));
    fireEvent.click(await screen.findByText("Ops Bot"));

    await waitFor(() => {
      expect(previewMock).toHaveBeenCalledWith("issue-pill", {
        assignee_type: "agent",
        assignee_id: "agent-1",
      });
    });
  });
});
