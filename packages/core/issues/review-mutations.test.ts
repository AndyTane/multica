// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import type { CommentTriggerPreview, Issue } from "../types";
import { ReviewFeedbackDeliveryError, sendReviewFeedback } from "./review-mutations";

vi.mock("../api", () => ({ api: { previewCommentTriggers: vi.fn(), createComment: vi.fn() } }));
const issue = { id: "i1", assignee_type: "agent", assignee_id: "owner" } as Issue;
const preview = (id = "owner", source = "issue_assignee", squad_id?: string): CommentTriggerPreview => ({ agents: [{ id, name: id, source, squad_id, reason: "" }] });
const outgoing = (content: string, type = "agent") => `${content}\n\n[@${type}](mention://${type}/owner)`;

beforeEach(() => { vi.resetAllMocks(); vi.mocked(api.createComment).mockResolvedValue({ id: "c1", issue_revision: 8 } as never); });

describe("review feedback delivery", () => {
  it("keeps ordinary assignee feedback top-level without adding a mention", async () => {
    vi.mocked(api.previewCommentTriggers).mockResolvedValue(preview());
    await sendReviewFeedback(issue, "  Fix this  ");
    expect(api.previewCommentTriggers).toHaveBeenCalledExactlyOnceWith("i1", "Fix this");
    expect(api.createComment).toHaveBeenCalledExactlyOnceWith("i1", "Fix this");
  });

  it("adds the owner when an explicit mention suppresses fallback, and checks exactly the sent content", async () => {
    const content = "Ask [@Other](mention://agent/other) to help";
    vi.mocked(api.previewCommentTriggers).mockResolvedValueOnce(preview("other", "mention_agent")).mockResolvedValueOnce({ agents: [...preview("other").agents, ...preview("owner", "mention_agent").agents] });
    await sendReviewFeedback(issue, content);
    expect(api.previewCommentTriggers).toHaveBeenNthCalledWith(2, "i1", outgoing(content));
    expect(api.createComment).toHaveBeenCalledExactlyOnceWith("i1", outgoing(content));
  });

  it.each(["/note Please fix this", "  /NOTE\nPlease fix this", "/note"])("preserves note semantics for %s", async (content) => {
    vi.mocked(api.previewCommentTriggers).mockResolvedValue({ agents: [] });
    await expect(sendReviewFeedback(issue, content)).rejects.toMatchObject({ isNote: true });
    expect(api.previewCommentTriggers).toHaveBeenLastCalledWith("i1", outgoing(content.trim()));
    expect(api.createComment).not.toHaveBeenCalled();
  });

  it.each(["/notes Please fix", "Please fix"])("rejects an empty final preview without creating a comment: %s", async (content) => {
    vi.mocked(api.previewCommentTriggers).mockResolvedValue({ agents: [] });
    await expect(sendReviewFeedback(issue, content)).rejects.toEqual(new ReviewFeedbackDeliveryError(false));
    expect(api.createComment).not.toHaveBeenCalled();
  });

  it("does not mistake other agents for the owner in the final preview", async () => {
    vi.mocked(api.previewCommentTriggers).mockResolvedValue(preview("other"));
    await expect(sendReviewFeedback(issue, "Fix this")).rejects.toBeInstanceOf(ReviewFeedbackDeliveryError);
    expect(api.createComment).not.toHaveBeenCalled();
  });

  it.each([1, 2])("does not post when preview request %i fails", async (request) => {
    if (request === 2) vi.mocked(api.previewCommentTriggers).mockResolvedValueOnce({ agents: [] });
    vi.mocked(api.previewCommentTriggers).mockRejectedValueOnce(new Error("offline"));
    await expect(sendReviewFeedback(issue, "Fix this")).rejects.toThrow("offline");
    expect(api.createComment).not.toHaveBeenCalled();
  });

  it("accepts squad assignee fallback without accepting a mention of another squad", async () => {
    const squadIssue = { ...issue, assignee_type: "squad" } as Issue;
    vi.mocked(api.previewCommentTriggers).mockResolvedValueOnce(preview("leader"));
    await sendReviewFeedback(squadIssue, "Fix this");
    vi.mocked(api.createComment).mockClear();
    vi.mocked(api.previewCommentTriggers).mockResolvedValue(preview("leader", "mention_squad_leader", "other-squad"));
    await expect(sendReviewFeedback(squadIssue, "Fix this")).rejects.toBeInstanceOf(ReviewFeedbackDeliveryError);
    expect(api.createComment).not.toHaveBeenCalled();
  });

  it("accepts the appended squad mention only when its own leader is in the final preview", async () => {
    vi.mocked(api.previewCommentTriggers).mockResolvedValueOnce(preview("other-leader", "mention_squad_leader"))
      .mockResolvedValueOnce({ agents: [...preview("other-leader", "mention_squad_leader", "other-squad").agents, ...preview("leader", "mention_squad_leader", "owner").agents] });
    await sendReviewFeedback({ ...issue, assignee_type: "squad" }, "Fix this");
    expect(api.createComment).toHaveBeenCalledExactlyOnceWith("i1", outgoing("Fix this", "squad"));
  });

  it("rejects a blocked squad mention even when another squad shares the leader", async () => {
    vi.mocked(api.previewCommentTriggers).mockResolvedValueOnce({ agents: [] }).mockResolvedValueOnce({ ...preview("leader", "mention_squad_leader", "other-squad"), blocked: [{ target_type: "squad", target_id: "owner", status: "blocked", reason_code: "invoke_forbidden" }] });
    await expect(sendReviewFeedback({ ...issue, assignee_type: "squad" }, "Fix this")).rejects.toBeInstanceOf(ReviewFeedbackDeliveryError);
    expect(api.createComment).not.toHaveBeenCalled();
  });

  it("does not guess the squad from a legacy preview with no squad ID", async () => {
    vi.mocked(api.previewCommentTriggers).mockResolvedValue(preview("leader", "mention_squad_leader"));
    await expect(sendReviewFeedback({ ...issue, assignee_type: "squad" }, "Fix this")).rejects.toBeInstanceOf(ReviewFeedbackDeliveryError);
    expect(api.createComment).not.toHaveBeenCalled();
  });

  it("does not require agent routing for a member assignee", async () => {
    await sendReviewFeedback({ ...issue, assignee_type: "member" }, "Fix this");
    expect(api.previewCommentTriggers).not.toHaveBeenCalled();
    expect(api.createComment).toHaveBeenCalledExactlyOnceWith("i1", "Fix this");
  });
});
