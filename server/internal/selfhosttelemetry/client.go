package selfhosttelemetry

import (
	"bytes"
	"context"
	"net/http"
)

type deliveryDisposition int

const (
	deliveryRetry deliveryDisposition = iota
	deliverySuccess
	deliveryStopForDay
)

type eventSender interface {
	Send(context.Context, []byte) deliveryDisposition
	Close()
}

// HTTPClient has no production endpoint or transport. Only tests inject one
// to exercise the historical delivery protocol against fake services.
type HTTPClient struct {
	endpoint string
	client   *http.Client
}

func newHTTPClient() *HTTPClient {
	return &HTTPClient{}
}

func newHTTPClientForTest(endpoint string, client *http.Client) *HTTPClient {
	return &HTTPClient{endpoint: endpoint, client: client}
}

func (c *HTTPClient) Send(ctx context.Context, body []byte) deliveryDisposition {
	if c == nil || c.client == nil || c.endpoint == "" {
		return deliveryStopForDay
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return deliveryStopForDay
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return deliveryRetry
	}
	// The response body is deliberately never read or logged.
	_ = resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusAccepted:
		return deliverySuccess
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return deliveryRetry
	default:
		return deliveryStopForDay
	}
}

func (c *HTTPClient) Close() {
	if c == nil || c.client == nil {
		return
	}
	c.client.CloseIdleConnections()
}
