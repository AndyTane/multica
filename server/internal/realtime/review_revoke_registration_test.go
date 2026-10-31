package realtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Review reproducer (P5-R1): pause between successful session validation and
// hub registration, revoke the session, then finish the handshake. The revoked
// handle must not register and must not receive a protected event.
type reviewBlockedMembership struct {
	entered, release chan struct{}
}

func (m *reviewBlockedMembership) IsMember(context.Context, string, string) bool {
	close(m.entered)
	<-m.release
	return true
}

func TestReviewRevocationDuringHandshake(t *testing.T) {
	validator := &fakeSessionValidator{}
	validator.valid.Store(true)
	hub := NewHub()
	go hub.Run()
	member := &reviewBlockedMembership{make(chan struct{}), make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(hub, member, validator, nil, nil, w, r)
	}))
	defer server.Close()
	userID := "00000000-0000-0000-0000-0000000000e1"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+makeTestTokenForUser(t, userID, ""))
	type result struct {
		conn *websocket.Conn
		err  error
	}
	done := make(chan result, 1)
	go func() {
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws?workspace_id="+testWorkspaceID, header)
		done <- result{conn, err}
	}()
	select {
	case <-member.entered:
	case <-time.After(3 * time.Second):
		close(member.release)
		t.Fatal("handshake did not reach membership barrier")
	}
	validator.valid.Store(false)
	hub.DisconnectUser(userID, "session_revoked")
	close(member.release)
	res := <-done
	if res.err != nil {
		return
	}
	defer res.conn.Close()
	deadline := time.Now().Add(time.Second)
	for totalClients(hub) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	hub.BroadcastToWorkspace(testWorkspaceID, []byte(`{"type":"review_protected_event"}`))
	res.conn.SetReadDeadline(time.Now().Add(time.Second))
	_, frame, err := res.conn.ReadMessage()
	if err == nil && strings.Contains(string(frame), "review_protected_event") {
		t.Fatal("revoked session registered after DisconnectUser and received protected event")
	}
}
