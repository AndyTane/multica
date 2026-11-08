package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// SubmitIssueReview is a capability-specific entry point: an older server must
// reject the request, never silently apply only its status field. The existing
// issue transaction owns authorization, revision checks and review validation.
func (h *Handler) SubmitIssueReview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status           string                     `json:"status"`
		ExpectedRevision int64                      `json:"expected_revision"`
		ReviewMetadata   map[string]json.RawMessage `json:"review_metadata"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid review submission")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid review submission")
		return
	}
	var action, by string
	_ = json.Unmarshal(req.ReviewMetadata["review_action"], &action)
	_ = json.Unmarshal(req.ReviewMetadata["review_by"], &by)
	if !isReviewStatus(req.Status) || req.ExpectedRevision < 1 ||
		(by != reviewByUser && by != reviewBySpecialist) || strings.TrimSpace(action) == "" {
		writeError(w, http.StatusBadRequest, "review submission requires in_review or blocked, expected_revision, review_by and a non-empty review_action")
		return
	}
	body, err := json.Marshal(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid review submission")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	h.UpdateIssue(w, r)
}
