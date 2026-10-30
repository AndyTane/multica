package llm

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/privacy"
	openai "github.com/openai/openai-go/v3"
)

type privacyModelTransport struct {
	t        *testing.T
	requests int
}

func (f *privacyModelTransport) Do(req *http.Request) (*http.Response, error) {
	f.requests++
	if req.URL.String() != "https://user-model.example/v1/chat/completions" {
		f.t.Errorf("configured model destination changed: %s", req.URL)
	}
	if req.Header.Get("Authorization") != "Bearer fake-model-key" {
		f.t.Error("model authentication was not preserved")
	}
	status, body := http.StatusOK, "data: {\"id\":\"fake\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"
	headers := http.Header{"Content-Type": {"text/event-stream"}}
	if f.requests == 1 {
		status, body = http.StatusServiceUnavailable, `{"error":{"message":"retry","type":"server_error"}}`
		headers.Set("Content-Type", "application/json")
		headers.Set("Retry-After", "0")
	}
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

// Exercise the real SDK with a fake transport and a non-loopback destination.
// Reporting policy must not become a model host filter or disable SDK retries.
func TestReportingPolicyPreservesConfiguredModelStreamingAndRetry(t *testing.T) {
	if privacy.ExternalReportingEnabled {
		t.Fatal("test requires the production reporting policy")
	}
	transport := &privacyModelTransport{t: t}
	client := New(Config{APIKey: "fake-model-key", BaseURL: "https://user-model.example/v1", HTTPClient: transport, MaxRetries: retries(1)})
	stream, err := client.ChatStream(context.Background(), openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("fake user task")},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var output strings.Builder
	for stream.Next() {
		for _, choice := range stream.Current().Choices {
			output.WriteString(choice.Delta.Content)
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "hello" || transport.requests != 2 {
		t.Fatalf("output=%q, model requests=%d; want hello after one retry", output.String(), transport.requests)
	}
}
