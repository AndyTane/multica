package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextCompactionPolicy(t *testing.T) {
	for _, raw := range []string{"", "{}", `{"unrelated":true}`} {
		policy, err := ParseContextCompaction([]byte(raw))
		if err != nil || !policy.ShouldCompact(80000) || policy.ShouldCompact(79999) {
			t.Fatalf("default policy for %q = %#v, %v", raw, policy, err)
		}
	}
	policy, err := ParseContextCompaction([]byte(`{"context_compaction":{"enabled":false,"threshold_tokens":120000}}`))
	if err != nil || policy.ShouldCompact(200000) || policy.ThresholdTokens != 120000 {
		t.Fatalf("disabled policy: %#v %v", policy, err)
	}
	for _, raw := range []string{`{"context_compaction":null}`, `{"context_compaction":{"enabled":null}}`, `{"context_compaction":{"threshold_tokens":null}}`, `{"context_compaction":{"enabled":"yes"}}`, `{"context_compaction":{"threshold_tokens":0}}`, `{"context_compaction":{"threshold_tokens":2000001}}`, `{"context_compaction":{"threshold_tokens":1.5}}`} {
		if _, err := ParseContextCompaction([]byte(raw)); err == nil {
			t.Fatalf("accepted malformed policy %s", raw)
		}
	}
}

func TestContextCompactionLatestContextNotBillingTotals(t *testing.T) {
	file := filepath.Join(t.TempDir(), "rollout.jsonl")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("{\"type\":\"event_msg\",\"payload\":{\"type\":\"token_count\",\"info\":{\"total_token_usage\":{\"input_tokens\":900000},\"last_token_usage\":{\"input_tokens\":76000,\"cached_input_tokens\":70000,\"output_tokens\":5000,\"reasoning_output_tokens\":2000}}}}\n")
	if got := readContextTail(file, "codex"); got != 81000 {
		t.Fatalf("Codex context = %d", got)
	}
	write("{\"type\":\"assistant\",\"message\":{\"usage\":{\"input_tokens\":5000,\"cache_read_input_tokens\":70000,\"cache_creation_input_tokens\":4000,\"output_tokens\":2000}}}\n")
	if got := readContextTail(file, "claude"); got != 81000 {
		t.Fatalf("Claude context = %d", got)
	}
	for _, boundary := range []string{`{"type":"system","subtype":"compact_boundary","message":"Context compacted"}`, `{"type":"compacted","message":"summary"}`, `{"type":"event_msg","payload":{"type":"context_compacted"}}`} {
		write(`{"type":"assistant","message":{"usage":{"input_tokens":81000}}}` + "\n")
		f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.WriteString(boundary + "\n")
		_ = f.Close()
		if got := readContextTail(file, "claude"); got != 0 {
			t.Fatalf("stale pre-compact usage = %d", got)
		}
	}
	write(strings.Repeat("x", int(contextTailLimit)+100) + "\n" + `{"type":"assistant","message":{"usage":{"input_tokens":1234}}}` + "\n{\"type\":")
	if got := readContextTail(file, "claude"); got != 1234 {
		t.Fatalf("bounded malformed tail = %d", got)
	}
}
