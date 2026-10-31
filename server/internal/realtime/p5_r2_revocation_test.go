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

// P5-R2: registration and revocation share the hub's state boundary. These
// regressions pin the two sides of it: a handshake that started before a
// revocation never joins the rooms, and a handshake that starts after it (a new
// login with a new session version) is admitted normally.

func waitForClientCount(t *testing.T, hub *Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if totalClients(hub) == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("registered clients = %d, want %d", totalClients(hub), want)
}

func drainUntilClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// TestP5R2NewSessionAfterRevocationStillConnects is the "other side" of the
// handshake fix: once revocation has completed, a fresh valid session must be
// able to connect and receive protected events.
func TestP5R2NewSessionAfterRevocationStillConnects(t *testing.T) {
	validator := &fakeSessionValidator{}
	validator.valid.Store(true)
	hub, server := newSessionHub(t, validator)
	userID := "00000000-0000-0000-0000-0000000000f1"

	first, firstFrame, err := dialSession(t, server, userID)
	if err != nil || !strings.Contains(firstFrame, "auth_ack") {
		t.Fatalf("initial connection: frame=%q err=%v", firstFrame, err)
	}
	defer first.Close()
	waitForClientCount(t, hub, 1)

	if closed := hub.DisconnectUser(userID, "session_revoked"); closed != 1 {
		t.Fatalf("disconnected %d connections, want 1", closed)
	}
	drainUntilClosed(t, first)
	waitForClientCount(t, hub, 0)

	// A new sign-in mints a new session after the revocation boundary. It must
	// not be mistaken for the revoked handle.
	second, secondFrame, err := dialSession(t, server, userID)
	if err != nil {
		t.Fatalf("fresh session rejected: %v", err)
	}
	defer second.Close()
	if !strings.Contains(secondFrame, "auth_ack") {
		t.Fatalf("first frame = %q, want auth_ack", secondFrame)
	}
	waitForClientCount(t, hub, 1)

	hub.BroadcastToWorkspace(testWorkspaceID, []byte(`{"type":"p5r2_new_session"}`))
	second.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, frame, err := second.ReadMessage()
	if err != nil {
		t.Fatalf("new session did not receive the protected event: %v", err)
	}
	if !strings.Contains(string(frame), "p5r2_new_session") {
		t.Fatalf("unexpected frame for the new session: %s", frame)
	}
}

// TestP5R2HandshakeRevocationLeavesOtherUsersConnected keeps the boundary
// scoped to one account: one user's handshake is refused while another user's
// live connection keeps receiving events.
func TestP5R2HandshakeRevocationLeavesOtherUsersConnected(t *testing.T) {
	validator := &fakeSessionValidator{}
	validator.valid.Store(true)
	hub := NewHub()
	go hub.Run()
	blockedUser := "00000000-0000-0000-0000-0000000000f2"
	member := &blockedUserMembership{blocked: blockedUser, entered: make(chan struct{}), release: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(hub, member, validator, nil, nil, w, r)
	}))
	defer server.Close()

	other, otherFrame, err := dialSession(t, server, "00000000-0000-0000-0000-0000000000f3")
	if err != nil || !strings.Contains(otherFrame, "auth_ack") {
		t.Fatalf("other user connection: frame=%q err=%v", otherFrame, err)
	}
	defer other.Close()
	waitForClientCount(t, hub, 1)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+makeTestTokenForUser(t, blockedUser, ""))
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
		t.Fatal("handshake did not reach the membership barrier")
	}
	hub.DisconnectUser(blockedUser, "session_revoked")
	close(member.release)
	res := <-done
	if res.err != nil {
		t.Fatalf("handshake dial failed before registration: %v", res.err)
	}
	defer res.conn.Close()

	// Only the unrelated user stays registered.
	time.Sleep(50 * time.Millisecond)
	if got := totalClients(hub); got != 1 {
		t.Fatalf("registered clients = %d, want only the unrelated user", got)
	}
	res.conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, frame, err := res.conn.ReadMessage(); err == nil && strings.Contains(string(frame), "p5r2_other_user") {
		t.Fatal("the refused handshake received a protected event")
	}

	hub.BroadcastToWorkspace(testWorkspaceID, []byte(`{"type":"p5r2_other_user"}`))
	other.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, frame, err := other.ReadMessage()
	if err != nil || !strings.Contains(string(frame), "p5r2_other_user") {
		t.Fatalf("unrelated user lost its connection: frame=%s err=%v", frame, err)
	}
}

type blockedUserMembership struct {
	blocked string
	entered chan struct{}
	release chan struct{}
}

func (m *blockedUserMembership) IsMember(_ context.Context, userID, _ string) bool {
	if userID == m.blocked {
		close(m.entered)
		<-m.release
	}
	return true
}
