package main

import "net/http"

func upstreamCloudUnavailable(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGone)
	_, _ = w.Write([]byte(`{"error":"Official cloud services are not available in U Workers"}`))
}
