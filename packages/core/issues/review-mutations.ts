import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type { Issue, UpdateIssueRequest } from "../types";
import { issueKeys } from "./queries";
import { onIssueUpdated } from "./ws-updaters";

export function useReviewUpdate(wsId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...request }: UpdateIssueRequest & { id: string }) => api.updateIssue(id, request),
    onSuccess: (issue) => onIssueUpdated(qc, wsId, issue, { statusChanged: true }),
    onError: () => { void qc.invalidateQueries({ queryKey: issueKeys.all(wsId) }); },
  });
}

// Top-level member comments hand work to the assignee. Explicit syntax can
// suppress that fallback, so verify the preview and name that target if needed.
export async function sendReviewFeedback(issue: Issue, content: string) {
  let outgoing = content.trim();
  if (issue.assignee_id && (issue.assignee_type === "agent" || issue.assignee_type === "squad")) {
    const preview = await api.previewCommentTriggers(issue.id, outgoing);
    const reachesAssignee = issue.assignee_type === "agent"
      ? preview.agents.some((a) => a.id === issue.assignee_id)
      : preview.agents.some((a) => a.source === "issue_assignee");
    if (!reachesAssignee) {
      outgoing += `\n\n[@${issue.assignee_type}](mention://${issue.assignee_type}/${issue.assignee_id})`;
      await api.previewCommentTriggers(issue.id, outgoing);
    }
  }
  return api.createComment(issue.id, outgoing);
}
