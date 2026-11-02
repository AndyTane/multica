package handler

import (
	"encoding/json"
	"slices"

	"github.com/multica-ai/multica/server/pkg/agent"
)

// checkQuickCreateRuntimeVersion accepts an explicit feature declaration before
// consulting upstream release numbers. Decode the fields independently: malformed
// capabilities must not erase a valid version used by the older-daemon policy.
func checkQuickCreateRuntimeVersion(metadata []byte, minimum, capability string) error {
	var fields struct {
		Version      json.RawMessage `json:"cli_version"`
		Capabilities json.RawMessage `json:"capabilities"`
	}
	if err := json.Unmarshal(metadata, &fields); err != nil {
		return agent.ErrCLIVersionMissing
	}
	var capabilities []string
	if err := json.Unmarshal(fields.Capabilities, &capabilities); err == nil && capability != "" && slices.Contains(capabilities, capability) {
		return nil
	}
	var version string
	_ = json.Unmarshal(fields.Version, &version)
	return agent.CheckMinCLIVersionFor(version, minimum)
}
