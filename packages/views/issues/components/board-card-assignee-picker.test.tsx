import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Issue, IssueAssigneeType } from "@multica/core/types";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import { AppLink, NavigationProvider, type NavigationAdapter } from "../../navigation";
import {
  IssueSurfaceActionsProvider,
  type IssueSurfaceActions,
} from "../surface/actions-context";
import { BoardCardContent } from "./board-card";

// CLAWSTA-151 F6: the card's assignee picker needs the card itself to run the
// assignment preflight; without it the picker falls back to a plain transfer.
const { preflightMock, WIRE_AGENT } = vi.hoisted(() => ({
  preflightMock: vi.fn(),
  WIRE_AGENT: {
    id: "agent-wire",
    name: "Ops Bot",
    archived_at: null,
    visibility: "workspace",
    runtime_id: "runtime-1",
    runtime_bound: true,
    owner_id: null,
    permission_mode: "public_to",
    invocation_targets: [{ target_type: "workspace", target_id: "ws-1" }],
  },
}));

vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQuery: (options?: { queryKey?: readonly unknown[] }) => {
    const key = options?.queryKey ?? [];
    if (key[0] === "workspaces" && key[2] === "agents") {
      return { data: [WIRE_AGENT] };
    }
    return { data: [] };
  },
}));

vi.mock("@multica/core/permissions", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/permissions")>()),
  canAssignAgentToIssue: () => ({ allowed: true, message: "" }),
}));

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/core/properties", () => ({
  propertyListOptions: () => ({ queryKey: ["properties"] }),
}));

vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) =>
    selector({ user: { id: "viewer-1" } }),
}));

vi.mock("@multica/core/agents", () => ({
  isAgentRuntimeBound: () => true,
  useAgentPresenceDetail: () => ({ availability: "offline", workload: null }),
}));

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws-1", slug: "acme" }),
  useWorkspacePaths: () => ({
    memberDetail: (id: string) => `/acme/members/${id}`,
    agentDetail: (id: string) => `/acme/agents/${id}`,
    squadDetail: (id: string) => `/acme/squads/${id}`,
  }),
}));

const viewState = vi.hoisted(() => ({
  viewMode: "board",
  grouping: "status",
  swimlaneGrouping: "assignee",
  cardProperties: {
    priority: false,
    description: false,
    assignee: true,
    startDate: true,
    dueDate: true,
    project: false,
    childProgress: false,
    labels: false,
  },
  cardPropertyIds: [],
}));

vi.mock("@multica/core/issues/stores/view-store-context", () => ({
  useViewStore: (selector: (state: typeof viewState) => unknown) => selector(viewState),
}));

vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({
    getActorName: (type: string) => `Assigned ${type}`,
    getActorInitials: () => "AA",
    getActorAvatarUrl: () => null,
  }),
}));

vi.mock("../../i18n", () => ({
  useLocale: () => "en",
  useT: () => ({ t: () => "Translated" }),
  useTimeAgo: () => () => "now",
}));

vi.mock("../../agents/components/agent-profile-card", () => ({
  AgentProfileCard: () => null,
}));

vi.mock("../../agents/components/agent-live-peek-card", () => ({
  AgentLivePeekCard: () => null,
}));

vi.mock("../../members/member-profile-card", () => ({
  MemberProfileCard: () => null,
}));

vi.mock("../../squads/components/squad-profile-card", () => ({
  SquadProfileCard: () => null,
}));

vi.mock("./issue-agent-activity-indicator", () => ({
  IssueAgentActivityIndicator: () => null,
}));

const navigation: NavigationAdapter = {
  push: vi.fn(),
  replace: vi.fn(),
  back: vi.fn(),
  pathname: "/acme/issues",
  searchParams: new URLSearchParams(),
  hash: "",
  getShareableUrl: (path) => `https://app.example${path}`,
  getExternalShareUrl: (path) => `https://app.example${path}`,
};

const actions: IssueSurfaceActions = {
  isPending: false,
  createIssue: vi.fn(),
  updateIssue: vi.fn(),
  moveIssue: vi.fn(),
  batchUpdate: vi.fn().mockResolvedValue(undefined),
  batchDelete: vi.fn().mockResolvedValue(undefined),
};

function makeIssue(assigneeType: IssueAssigneeType): Issue {
  return {
    id: `issue-${assigneeType}`,
    workspace_id: "ws-1",
    number: 6082,
    identifier: "MUL-6082",
    title: "Fix Board assignee interaction",
    description: null,
    status: "todo",
    priority: "none",
    assignee_type: assigneeType,
    assignee_id: `${assigneeType}-1`,
    creator_type: "member",
    creator_id: "member-1",
    parent_issue_id: null,
    project_id: null,
    position: 1,
    stage: null,
    start_date: "2026-08-12",
    due_date: "2026-08-13",
    metadata: {},
    properties: {},
    labels: [],
    created_at: "2026-08-12T00:00:00Z",
    updated_at: "2026-08-12T00:00:00Z",
  };
}

describe("BoardCardContent assignee picker", () => {
  beforeEach(() => {
    preflightMock.mockReset();
    setApiInstance({ previewIssueAssignment: preflightMock } as unknown as ApiClient);
  });

  it("hands the card to the picker so the transfer preflight can check it", async () => {
    const issue = makeIssue("member");
    const { container } = render(
      <NavigationProvider value={navigation}>
        <IssueSurfaceActionsProvider actions={actions}>
          <AppLink href={`/acme/issues/${issue.id}`}>
            <BoardCardContent issue={issue} editable />
          </AppLink>
        </IssueSurfaceActionsProvider>
      </NavigationProvider>,
    );

    fireEvent.click(container.querySelector('[data-slot="avatar"]')!);
    fireEvent.click(await screen.findByText("Ops Bot"));

    await waitFor(() =>
      expect(preflightMock).toHaveBeenCalledWith(issue.id, {
        assignee_type: "agent",
        assignee_id: "agent-wire",
      }),
    );
  });

  it.each<IssueAssigneeType>(["member", "agent", "squad"])(
    "opens the picker from an avatar-only %s assignee without navigating the card",
    (assigneeType) => {
      const issue = makeIssue(assigneeType);
      const { container } = render(
        <NavigationProvider value={navigation}>
          <IssueSurfaceActionsProvider actions={actions}>
            <AppLink href={`/acme/issues/${issue.id}`}>
              <BoardCardContent issue={issue} editable />
            </AppLink>
          </IssueSurfaceActionsProvider>
        </NavigationProvider>,
      );
      const avatar = container.querySelector('[data-slot="avatar"]');

      expect(avatar).not.toBeNull();
      expect(avatar!.closest('[role="link"]')).toBeNull();
      expect(fireEvent.click(avatar!)).toBe(false);
      expect(screen.getByRole("textbox")).toBeInTheDocument();
      expect(navigation.push).not.toHaveBeenCalled();
    },
  );

  it("does not repeat the assignee inside an assignee-grouped board", () => {
    viewState.grouping = "assignee";
    const issue = makeIssue("member");
    const { container } = render(
      <NavigationProvider value={navigation}>
        <IssueSurfaceActionsProvider actions={actions}>
          <BoardCardContent issue={issue} editable />
        </IssueSurfaceActionsProvider>
      </NavigationProvider>,
    );

    expect(container.querySelector('[data-slot="avatar"]')).toBeNull();
    viewState.grouping = "status";
  });

  it("does not repeat the assignee inside an assignee-grouped swimlane", () => {
    viewState.viewMode = "swimlane";
    const issue = makeIssue("member");
    const { container } = render(
      <NavigationProvider value={navigation}>
        <IssueSurfaceActionsProvider actions={actions}>
          <BoardCardContent issue={issue} editable />
        </IssueSurfaceActionsProvider>
      </NavigationProvider>,
    );

    expect(container.querySelector('[data-slot="avatar"]')).toBeNull();
    viewState.viewMode = "board";
  });

  it("keeps empty priority and assignee fields visually silent", () => {
    viewState.cardProperties.priority = true;
    const issue = {
      ...makeIssue("member"),
      assignee_type: null,
      assignee_id: null,
      start_date: null,
      due_date: null,
    };
    const { container } = render(
      <NavigationProvider value={navigation}>
        <IssueSurfaceActionsProvider actions={actions}>
          <BoardCardContent issue={issue} editable />
        </IssueSurfaceActionsProvider>
      </NavigationProvider>,
    );

    expect(container.querySelector("svg")).toBeNull();
    expect(screen.queryByText("Translated")).not.toBeInTheDocument();
    viewState.cardProperties.priority = false;
  });
});
