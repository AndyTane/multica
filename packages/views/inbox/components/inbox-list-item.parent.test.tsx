import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { WorkspaceSlugProvider } from "@multica/core/paths";
import { buildIssueStatusCatalog } from "@multica/core/issue-statuses";
import type { InboxItem } from "@multica/core/types";
import { NavigationProvider } from "../../navigation";
import type { NavigationAdapter } from "../../navigation";
import { InboxListItem } from "./inbox-list-item";

// CLAWSTA-151 F5: a notification about a sub-issue says which card it belongs
// to. The parent projection is optional (B14): an older server omits it and
// the row must keep rendering exactly as before, with no empty line.

vi.mock("@multica/core/issue-statuses/hooks", () => ({
  useIssueStatuses: () => buildIssueStatusCatalog(undefined),
}));
vi.mock("../../issues/components", () => ({
  StatusIcon: () => <span data-testid="status-icon" />,
}));
vi.mock("../../issues/components/issue-agent-activity-indicator", () => ({
  IssueAgentActivityIndicator: () => <span data-testid="issue-agent-activity" />,
}));
vi.mock("../../common/actor-avatar", () => ({
  ActorAvatar: () => <span data-testid="actor-avatar" />,
}));
vi.mock("./inbox-detail-label", () => ({
  InboxDetailLabel: () => null,
  useTypeLabels: () => ({}),
}));

// The real selectors are exercised against a tiny dictionary: the parent line
// is the only copy under test, and interpolation is reproduced so the assert
// reads like the shipped sentence.
const MESSAGES = {
  list: {
    time: { just_now: "just now", minutes: "{{count}}m", hours: "{{count}}h", days: "{{count}}d" },
    archive_tooltip: "Archive",
    unarchive_tooltip: "Unarchive",
  },
  parent_line: { text: "In {{identifier}} · {{title}}" },
};

vi.mock("../../i18n", () => ({
  useT: () => ({
    t: (selector: (v: unknown) => string, options?: Record<string, unknown>) =>
      selector(MESSAGES).replace(/\{\{(\w+)\}\}/g, (_, key: string) =>
        String(options?.[key] ?? ""),
      ),
  }),
}));

function item(overrides: Partial<InboxItem> = {}): InboxItem {
  return {
    id: "inbox-1",
    workspace_id: "workspace-1",
    recipient_type: "member",
    recipient_id: "member-1",
    actor_type: "member",
    actor_id: "member-2",
    type: "new_comment",
    severity: "info",
    issue_id: "child-1",
    title: "Child issue",
    body: null,
    issue_status: null,
    read: true,
    archived: false,
    created_at: "2026-06-15T08:00:00Z",
    details: null,
    ...overrides,
  };
}

function adapter(): NavigationAdapter {
  return {
    push: vi.fn(),
    replace: vi.fn(),
    back: vi.fn(),
    pathname: "/",
    searchParams: new URLSearchParams(),
    hash: "",
    getShareableUrl: (p) => p,
    getExternalShareUrl: (p) => p,
  };
}

function renderRow(row: InboxItem) {
  return render(
    <WorkspaceSlugProvider slug="acme">
      <NavigationProvider value={adapter()}>
        <InboxListItem
          item={row}
          view="inbox"
          isSelected={false}
          onClick={vi.fn()}
          onAction={vi.fn()}
        />
      </NavigationProvider>
    </WorkspaceSlugProvider>,
  );
}

describe("InboxListItem parent line", () => {
  it("shows which card the sub-issue belongs to when the server sent the parent projection", () => {
    renderRow(
      item({
        parent_issue_id: "parent-1",
        parent_issue_identifier: "CLAWSTA-143",
        parent_issue_title: "Run the long build",
      }),
    );

    expect(screen.getByText("In CLAWSTA-143 · Run the long build")).toBeTruthy();
  });

  it("renders no parent line when the fields are missing (older server)", () => {
    const { container } = renderRow(item());

    expect(container.textContent).not.toContain("In CLAWSTA-143");
    expect(screen.queryByText(/ · /)).toBeNull();
  });

  it("does not infer a parent line from the issue id alone", () => {
    const { container } = renderRow(
      item({ parent_issue_id: "parent-1", parent_issue_title: "Run the long build" }),
    );

    // B14 keys the line off the identifier (the current workspace prefix +
    // number). An id without an identifier is not renderable copy.
    expect(container.textContent).not.toContain("Run the long build");
  });
});
