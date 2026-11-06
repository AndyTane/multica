import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, api } from "@multica/core/api";
import type { Issue } from "@multica/core/types";
import en from "../locales/en/issues.json";
import { AwaitingRow } from "./awaiting-page";

const { toastSuccess, toastError, peekOpen } = vi.hoisted(() => ({ toastSuccess: vi.fn(), toastError: vi.fn(), peekOpen: vi.fn() }));
vi.mock("sonner", () => ({ toast: { success: toastSuccess, error: toastError } }));
vi.mock("@multica/core/api", async (original) => ({ ...await original<typeof import("@multica/core/api")>(), api: { updateIssue: vi.fn(), createComment: vi.fn(), previewCommentTriggers: vi.fn() } }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws" }));
vi.mock("@multica/core/workspace/hooks", () => ({ useActorName: () => ({ getActorName: () => "Deliverer" }) }));
vi.mock("../common/actor-avatar", () => ({ ActorAvatar: () => null }));
vi.mock("../issues/components/issue-peek", () => ({ IssuePeekHost: ({ children }: { children: React.ReactNode }) => children }));
vi.mock("../issues/surface/peek-context", () => ({ PEEK_TARGET_ATTR: "data-peek-target", useIssuePeekActions: () => ({ open: peekOpen }) }));
vi.mock("../i18n", () => ({ useT: () => ({ t: (selector: (resources: typeof en) => string, args: Record<string, string | number> = {}) => selector(en).replace(/{{(.*?)}}/g, (_, key: string) => String(args[key] ?? "")) }) }));

const issue: Issue = { id: "i1", workspace_id: "ws", number: 143, identifier: "CLAWSTA-143", title: "Runtime timeout", description: null, status: "in_review", priority: "none", assignee_type: "agent", assignee_id: "agent1", creator_type: "member", creator_id: "me", parent_issue_id: null, project_id: null, position: 0, stage: null, start_date: null, due_date: null, created_at: "2026-11-01T00:00:00Z", updated_at: "2026-11-01T00:00:00Z", metadata: { review_by: "user", review_action: "Run a 40 minute build", review_user: "me" }, properties: {}, revision: 7 };

function show() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><ul><AwaitingRow issue={issue} /></ul></QueryClientProvider>);
  return qc;
}
beforeEach(() => { vi.clearAllMocks(); vi.mocked(api.previewCommentTriggers).mockResolvedValue({ agents: [{ id: "agent1", name: "Deliverer", source: "issue_assignee", reason: "" }] }); });
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
    show(); fireEvent.click(screen.getByRole("button", { name: "Send back" }));
    const textbox = await screen.findByRole("textbox");
    fireEvent.change(textbox, { target: { value: "Fix the build" } });
    fireEvent.click(screen.getAllByRole("button", { name: "Send back" }).at(-1)!);
    await screen.findByText("Feedback sent, status not updated");
    fireEvent.click(screen.getByRole("button", { name: "Retry status update" }));
    await waitFor(() => expect(api.updateIssue).toHaveBeenCalledTimes(2));
    expect(api.createComment).toHaveBeenCalledTimes(1);
    expect(api.createComment).toHaveBeenCalledWith("i1", "Fix the build");
    expect(api.updateIssue).toHaveBeenLastCalledWith("i1", { status: "in_progress", expected_revision: 8 });
  });

  it("keeps feedback on comment failure and prevents retry over a concurrent status change", async () => {
    vi.mocked(api.createComment).mockRejectedValueOnce(new Error("offline"));
    show(); fireEvent.click(screen.getByRole("button", { name: "Send back" }));
    const textbox = await screen.findByRole("textbox");
    fireEvent.change(textbox, { target: { value: "Needs work" } });
    fireEvent.click(screen.getAllByRole("button", { name: "Send back" }).at(-1)!);
    await screen.findByRole("alert");
    expect((textbox as HTMLTextAreaElement).value).toBe("Needs work"); expect(api.updateIssue).not.toHaveBeenCalled();
    vi.mocked(api.createComment).mockResolvedValueOnce({ id: "c1", issue_revision: 8 } as never);
    vi.mocked(api.updateIssue).mockRejectedValueOnce(new ApiError("Conflict", 409, "Conflict"));
    fireEvent.click(screen.getAllByRole("button", { name: "Send back" }).at(-1)!);
    await screen.findByText("This task has changed. Open it to continue.");
    expect(toastError).toHaveBeenLastCalledWith("This task has changed");
    expect((screen.getByRole("button", { name: "Retry status update" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
