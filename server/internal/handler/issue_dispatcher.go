package handler

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// fillIssueDispatchers adds the read-only B9 dispatcher projection to a batch
// of issue responses. It resolves the whole page with ONE query against the
// issue_dispatcher view — the same resolution rule the list filters use — and
// never falls back to per-row lookups. A failed lookup leaves the fields absent
// rather than claiming the issue has no dispatcher.
func (h *Handler) fillIssueDispatchers(ctx context.Context, workspaceID pgtype.UUID, resps []IssueResponse) {
	if len(resps) == 0 {
		return
	}
	ids := make([]pgtype.UUID, 0, len(resps))
	indexByID := make(map[string][]int, len(resps))
	for i := range resps {
		id, err := util.ParseUUID(resps[i].ID)
		if err != nil {
			continue
		}
		key := resps[i].ID
		if _, seen := indexByID[key]; !seen {
			ids = append(ids, id)
		}
		indexByID[key] = append(indexByID[key], i)
	}
	if len(ids) == 0 {
		return
	}
	rows, err := h.Queries.ListIssueDispatchers(ctx, db.ListIssueDispatchersParams{
		WorkspaceID: workspaceID,
		Ids:         ids,
	})
	if err != nil {
		slog.Warn("resolve issue dispatchers failed", "workspace_id", uuidToString(workspaceID), "error", err)
		return
	}
	for _, row := range rows {
		positions, ok := indexByID[uuidToString(row.ID)]
		if !ok || !row.DispatcherUserID.Valid {
			continue
		}
		active := row.DispatcherActive
		for _, pos := range positions {
			resps[pos].DispatcherUserID = util.UUIDToPtr(row.DispatcherUserID)
			resps[pos].DispatcherActive = &active
		}
	}
}

// fillOneIssueDispatcher is the single-response form of fillIssueDispatchers.
func (h *Handler) fillOneIssueDispatcher(ctx context.Context, workspaceID pgtype.UUID, resp *IssueResponse) {
	if resp == nil {
		return
	}
	batch := []IssueResponse{*resp}
	h.fillIssueDispatchers(ctx, workspaceID, batch)
	*resp = batch[0]
}
