package handler

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/pkg/agent"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestQuickCreateRuntimeVersionCapabilities(t *testing.T) {
	for _, feature := range []struct {
		name, minimum, capability string
	}{
		{"basic", agent.MinQuickCreateCLIVersion, protocol.DaemonCapabilityQuickCreateV1},
		{"fields", agent.MinQuickCreateFieldsCLIVersion, protocol.DaemonCapabilityQuickCreateFieldsV1},
	} {
		t.Run(feature.name, func(t *testing.T) {
			otherCapability := protocol.DaemonCapabilityQuickCreateFieldsV1
			if feature.capability == otherCapability {
				otherCapability = protocol.DaemonCapabilityQuickCreateV1
			}
			for _, tc := range []struct {
				name, version string
				caps          any
				want          error
			}{
				{"fork with support", "v0.2.1", []string{feature.capability}, nil},
				{"capability without release version", "", []string{feature.capability}, nil},
				{"fork without support", "v0.2.1", nil, agent.ErrCLIVersionTooOld},
				{"other quick-create feature", "v0.2.1", []string{otherCapability}, agent.ErrCLIVersionTooOld},
				{"string is not a list", "v0.2.1", feature.capability, agent.ErrCLIVersionTooOld},
				{"malformed list", "v0.2.1", []any{feature.capability, 42}, agent.ErrCLIVersionTooOld},
				{"upstream with malformed capabilities", feature.minimum, []any{feature.capability, 42}, nil},
				{"old upstream", "v0.2.20", nil, agent.ErrCLIVersionTooOld},
				{"upstream at minimum", feature.minimum, nil, nil},
				{"upstream above minimum", "0.6.1", nil, nil},
				{"development build", "v0.2.1-12-gabc1234", nil, nil},
				{"missing version and capability", "", nil, agent.ErrCLIVersionMissing},
				{"unreadable version", "dev", nil, agent.ErrCLIVersionMissing},
			} {
				t.Run(tc.name, func(t *testing.T) {
					metadata, err := json.Marshal(map[string]any{"cli_version": tc.version, "capabilities": tc.caps})
					if err != nil {
						t.Fatal(err)
					}
					if err := checkQuickCreateRuntimeVersion(metadata, feature.minimum, feature.capability); !errors.Is(err, tc.want) {
						t.Fatalf("check = %v, want %v", err, tc.want)
					}
				})
			}
			for _, metadata := range [][]byte{nil, []byte(`null`), []byte(`{"capabilities":`)} {
				if err := checkQuickCreateRuntimeVersion(metadata, feature.minimum, feature.capability); !errors.Is(err, agent.ErrCLIVersionMissing) {
					t.Errorf("metadata %q: error = %v, want missing", metadata, err)
				}
			}
		})
	}
}
