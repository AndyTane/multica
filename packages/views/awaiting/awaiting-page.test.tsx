import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, api } from "@multica/core/api";
import type { Issue } from "@multica/core/types";
import en from "../locales/en/issues.json";
import { AwaitingRow, AwaitingSections } from "./awaiting-page";

const { toastSuccess, toastError, peekOpen } = vi.hoisted(() => ({ toastSuccess: vi.fn(), toastError: vi.fn(), peekOpen: vi.fn() }));
vi.mock("sonner", () => ({ toast: { success: toastSuccess, error: toastError } }));
vi.mock("@multica/core/api", async (original) => ({ ...await original<typeof import("@multica/core/api")>(), api: { updateIssue: vi.fn(), createComment: vi.fn(), previewCommentTriggers: vi.fn() } }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws" }));
vi.mock("@multica/core/workspace/hooks", () => ({ useActorName: () => ({ getActorName: () => "Deliverer" }) }));
vi.mock("../common/actor-avatar", () => ({ ActorAvatar: () => null }));
vi.mock("../issues/components/issue-peek", () => ({ IssuePeekHost: ({ children }: { children: React.ReactNode }) => children }));
vi.mock("../issues/surface/peek-context", () => ({ PEEK_TARGET_ATTR: "data-peek-target", useIssuePeekActions: () => ({ open: peekOpen }) }));
vi.mock("../i18n", () => ({ useT: () => ({ t: (selector: (resources: typeof en) => string, args: Record<string, string | number> = {}) => selector(en).replace(/{{(.*?)}}/g, (_, key: string) => String(args[key] ?? "")) }) }));

const issue: Issue = { id: "i1", workspace_id: "ws", number: 143, identifier: "CLAWSTA-143", title: "Runtime timeout", description: null, status: "in_review", priority: "none", assignee_type: "agent", assignee_id: "agent1", creator_type: "member", creator_id: "me", parent_issue_id: null, project_id: null, position: 0, stage: null, start_date: null, due_date: null, created_at: "2026-11-01T00:00:00Z", updated_at: "2026-11-01T00:00:00Z", metadata: { review_by: "user", review_action: "Run a 40 minute build", review_user: "me" }, properties: {}, revision: 7, dispatcher_user_id: "me", dispatcher_active: true };

function makeIssue(patch: Partial<Issue>): Issue { return { ...issue, ...patch }; }

function show(target: Issue = issue) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><ul><AwaitingRow issue={target} /></ul></QueryClientProvider>);
  return qc;
}

function showSections(mine: Issue[], unmarked: Issue[]) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><AwaitingSections mine={mine} unmarked={unmarked} /></QueryClientProvider>);
}

beforeEach(() => { vi.resetAllMocks(); vi.mocked(api.previewCommentTriggers).mockResolvedValue({ agents: [{ id: "agent1", name: "Deliverer", source: "issue_assignee", reason: "" }] }); });
afterEach(cleanup);

describe("review actions", () => {
  it("opens peek from the action sentence, approves with revision, and atomically undoes", async () => {
    vi.mocked(api.updateIssue).mockResolvedValue({ ...issue, status: "done", revision: 8, metadata: {} });
    show();
    fireEvent.click(screen.getByRole("button", { name: /Run a 40 minute build/ })); expect(peekOpen).toHaveBeenCalledWith("i1");
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(api.updateIssue).toHaveBeenNthCalledWith(1, "i1", { status: "done", expected_revision: 7 });
    expect(peekOpen).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(api.createComment).not.toHaveBeenCalled();
    expect(api.previewCommentTriggers).not.toHaveBeenCalled();
    const options = toastSuccess.mock.calls[0]![1];
    expect(options.duration).toBe(5000);
    await act(() => options.action.onClick());
    await waitFor(() => expect(api.updateIssue).toHaveBeenCalledTimes(2));
    expect(api.updateIssue).toHaveBeenNthCalledWith(2, "i1", { status: "in_review", expected_revision: 8, review_metadata: issue.metadata });
  });

  it("keeps the row on approval failure and reports undo conflicts", async () => {
    vi.mocked(api.updateIssue).mockRejectedValueOnce(new Error("offline"));
    show(); fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(screen.getByText("Run a 40 minute build")).toBeTruthy();
    vi.mocked(api.updateIssue).mockResolvedValueOnce({ ...issue, revision: 8 });
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    vi.mocked(api.updateIssue).mockRejectedValueOnce(new ApiError("Conflict", 409, "Conflict"));
    await act(() => toastSuccess.mock.calls[0]![1].action.onClick());
    await waitFor(() => expect(toastError).toHaveBeenLastCalledWith("This task has changed"));
  });

  it("sends one top-level comment and retries only the failed status write", async () => {
    vi.mocked(api.createComment).mockResolvedValue({ id: "c1", issue_revision: 8 } as never);
    vi.mocked(api.updateIssue).mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce({ ...issue, status: "in_progress", revision: 9 });
    show(); fireEvent.click(screen.getByRole("button", { name: "Feedback" }));
    const textbox = await screen.findByRole("textbox", { name: "Feedback" });
    expect(textbox).toHaveAttribute("placeholder", en.awaiting.reason);
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
    fireEvent.change(textbox, { target: { value: "Fix the build" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByText("Feedback sent, status not updated");
    fireEvent.click(screen.getByRole("button", { name: "Retry status update" }));
    await waitFor(() => expect(api.updateIssue).toHaveBeenCalledTimes(2));
    expect(api.createComment).toHaveBeenCalledTimes(1);
    expect(api.previewCommentTriggers).toHaveBeenCalledTimes(1);
    expect(api.createComment).toHaveBeenCalledWith("i1", "Fix the build");
    expect(api.updateIssue).toHaveBeenLastCalledWith("i1", { status: "in_progress", expected_revision: 8 });
  });

  it.each([
    ["/note Needs work", "Feedback starting with /note is only recorded and will not reach Deliverer"],
    ["Needs work", "This feedback did not reach Deliverer. The status has not changed."],
  ])("keeps undeliverable feedback editable and prevents a status write: %s", async (content, message) => {
    vi.mocked(api.previewCommentTriggers).mockResolvedValue({ agents: [] });
    show(); fireEvent.click(screen.getByRole("button", { name: "Feedback" }));
    const textbox = await screen.findByRole("textbox", { name: "Feedback" });
    fireEvent.change(textbox, { target: { value: content } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(message);
    expect(textbox).toHaveValue(content);
    expect(textbox).toBeEnabled();
    expect(api.createComment).not.toHaveBeenCalled();
    expect(api.updateIssue).not.toHaveBeenCalled();
    fireEvent.change(textbox, { target: { value: "Fixed feedback" } });
    vi.mocked(api.previewCommentTriggers).mockResolvedValue({ agents: [{ id: "agent1", name: "Deliverer", source: "issue_assignee", reason: "" }] });
    vi.mocked(api.createComment).mockResolvedValue({ id: "c1", issue_revision: 8 } as never);
    vi.mocked(api.updateIssue).mockResolvedValue({ ...issue, status: "in_progress" });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(api.updateIssue).toHaveBeenCalledTimes(1));
    expect(api.createComment).toHaveBeenCalledExactlyOnceWith("i1", "Fixed feedback");
  });

  it("keeps feedback on comment failure and prevents retry over a concurrent status change", async () => {
    vi.mocked(api.createComment).mockRejectedValueOnce(new Error("offline"));
    show(); fireEvent.click(screen.getByRole("button", { name: "Feedback" }));
    const textbox = await screen.findByRole("textbox");
    fireEvent.change(textbox, { target: { value: "Needs work" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByRole("alert");
    expect((textbox as HTMLTextAreaElement).value).toBe("Needs work"); expect(api.updateIssue).not.toHaveBeenCalled();
    vi.mocked(api.createComment).mockResolvedValueOnce({ id: "c1", issue_revision: 8 } as never);
    vi.mocked(api.updateIssue).mockRejectedValueOnce(new ApiError("Conflict", 409, "Conflict"));
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByText("This task has changed. Open it to continue.");
    expect(toastError).toHaveBeenLastCalledWith("This task has changed");
    expect((screen.getByRole("button", { name: "Retry status update" }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("blocked kinds", () => {
  it("confirms an off-system action with the default sentence when left empty", async () => {
    const act = makeIssue({ status: "blocked", metadata: { review_by: "user", review_kind: "act", review_action: "Install v0.2.17", review_user: "me" } });
    vi.mocked(api.createComment).mockResolvedValue({ id: "c1", issue_revision: 8 } as never);
    vi.mocked(api.updateIssue).mockResolvedValue({ ...act, status: "in_progress", revision: 9 });
    show(act);
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    const textbox = await screen.findByRole("textbox", { name: "What you did" });
    expect(textbox).toHaveAttribute("placeholder", "Done: Install v0.2.17");
    fireEvent.click(screen.getAllByRole("button", { name: "Done" }).at(-1)!);
    await waitFor(() => expect(api.updateIssue).toHaveBeenCalledTimes(1));
    expect(api.createComment).toHaveBeenCalledExactlyOnceWith("i1", "Done: Install v0.2.17");
    expect(api.updateIssue).toHaveBeenCalledWith("i1", { status: "in_progress", expected_revision: 8 });
  });

  it("answers a question with a real comment and retries only the failed status write", async () => {
    const reply = makeIssue({ status: "blocked", metadata: { review_by: "user", review_kind: "reply", review_action: "Which branch?", review_user: "me" } });
    vi.mocked(api.createComment).mockResolvedValue({ id: "c1", issue_revision: 8 } as never);
    vi.mocked(api.updateIssue).mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce({ ...reply, status: "in_progress", revision: 9 });
    show(reply);
    fireEvent.click(screen.getByRole("button", { name: "Reply" }));
    const textbox = await screen.findByRole("textbox", { name: "Your reply" });
    expect(screen.getAllByRole("button", { name: "Reply" }).at(-1)).toBeDisabled();
    fireEvent.change(textbox, { target: { value: "Use release/v0.2.17" } });
    fireEvent.click(screen.getAllByRole("button", { name: "Reply" }).at(-1)!);
    await screen.findByText("Feedback sent, status not updated");
    fireEvent.click(screen.getByRole("button", { name: "Retry status update" }));
    await waitFor(() => expect(api.updateIssue).toHaveBeenCalledTimes(2));
    expect(api.createComment).toHaveBeenCalledExactlyOnceWith("i1", "Use release/v0.2.17");
    expect(api.updateIssue).toHaveBeenLastCalledWith("i1", { status: "in_progress", expected_revision: 8 });
  });
});

describe("awaiting sections", () => {
  it("removes the last row border in each group and keeps the footer separator", () => {
    const reply = makeIssue({ id: "r1", status: "blocked", metadata: { review_by: "user", review_kind: "reply" } });
    const approve = makeIssue({ id: "a1" });
    const second = makeIssue({ id: "a2" });
    const unmarked = makeIssue({ id: "u1", status: "blocked", metadata: {} });
    showSections([reply, approve, second], [unmarked]);
    for (const list of screen.getAllByRole("list")) {
      expect(list.lastElementChild).toHaveClass("last:border-b-0");
    }
    expect(screen.getAllByRole("listitem").at(-2)).toHaveClass("border-b");
    expect(screen.getByRole("button", { name: "1 more are waiting, but no one is marked" }).closest("section")).toHaveClass("border-t");
  });

  it("prints one heading per non-empty group with its count and leaves the footer out at zero", () => {
    const approve = makeIssue({ id: "a1", identifier: "CLAWSTA-1", metadata: { review_by: "user", review_action: "Approve the build", review_user: "me", review_since: "2026-10-01T00:00:00Z" } });
    const reply = makeIssue({ id: "r1", identifier: "CLAWSTA-2", status: "blocked", metadata: { review_by: "user", review_kind: "reply", review_action: "Which branch?", review_user: "me", review_since: "2026-10-02T00:00:00Z" } });
    showSections([approve, reply], []);
    expect(screen.getByRole("heading", { name: /Replies/ })).toHaveTextContent("1");
    expect(screen.getByRole("heading", { name: /Approvals/ })).toHaveTextContent("1");
    expect(screen.queryByRole("heading", { name: /Actions/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /more are waiting/ })).toBeNull();
    // A non-empty list never shows the empty state, and the empty group is not
    // printed at all.
    expect(screen.queryByText("Nothing waiting on you")).toBeNull();
  });

  it("expands the unmarked footer in place and shows the reason without action buttons", () => {
    const problem = makeIssue({ id: "p1", identifier: "CLAWSTA-3", status: "blocked", metadata: { review_by: "user" }, awaiting_problem: "dispatcher_inactive" });
    showSections([], [problem]);
    const footer = screen.getByRole("button", { name: "1 more are waiting, but no one is marked" });
    expect(footer).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(footer);
    expect(footer).toHaveAttribute("aria-expanded", "true");
    // The server's stable code is localized before it reaches the row.
    expect(screen.getByText("The person who assigned this card is no longer in the workspace.")).toBeTruthy();
    expect(screen.queryByText("dispatcher_inactive")).toBeNull();
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();
  });

  it("explains each unroutable code in words and never prints the code", () => {
    const problems = [
      ["no_dispatcher", "No one can act on this card: the person who assigned it has left the workspace."],
      ["dispatcher_inactive", "The person who assigned this card is no longer in the workspace."],
      ["receiver_inactive", "The chosen reviewer is no longer in the workspace."],
      ["invalid_review_mark", "This card's review note is incomplete, so it cannot be routed."],
    ] as const;
    for (const [code, text] of problems) {
      cleanup();
      show(makeIssue({
        id: `p-${code}`,
        identifier: `CLAWSTA-${code}`,
        status: "blocked",
        metadata: { review_by: "user" },
        awaiting_problem: code,
      }));
      expect(screen.getByText(text)).toBeTruthy();
      expect(screen.queryByText(code)).toBeNull();
    }
  });

  it("falls back to the generic sentence for a reason this client does not know", () => {
    show(makeIssue({
      id: "p-new",
      identifier: "CLAWSTA-NEW",
      status: "blocked",
      metadata: { review_by: "user" },
      awaiting_problem: "dispatcher_reassigned",
    }));
    expect(
      screen.getByText("This card cannot be routed to a person. Ask a workspace admin to check it."),
    ).toBeTruthy();
    expect(screen.queryByText("dispatcher_reassigned")).toBeNull();
  });
});
