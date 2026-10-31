package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// P5-R1: a WebSocket connection is an authorized session, not a one-time
// handshake. These tests cover the three ways that authority is enforced without
// a database: the handshake validator, immediate disconnection of an account,
// and periodic revalidation of a live connection.

type fakeSessionValidator struct {
	valid atomic.Bool
}

func (f *fakeSessionValidator) ValidateSession(context.Context, string, int64) bool {
	return f.valid.Load()
}

func newSessionHub(t *testing.T, sessions SessionValidator) (*Hub, *httptest.Server) {
	t.Helper()
	hub := NewHub()
	go hub.Run()
	mc := &mockMembershipChecker{}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		HandleWebSocket(hub, mc, sessions, nil, nil, w, r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return hub, server
}

// dialSession opens a connection for userID and returns it together with the
// first frame the server sent (auth_ack when accepted).
func dialSession(t *testing.T, server *httptest.Server, userID string) (*websocket.Conn, string, error) {
	t.Helper()
	token := makeTestTokenForUser(t, userID, "")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?workspace_id=" + testWorkspaceID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	authMsg, _ := json.Marshal(map[string]any{
		"type":    "auth",
		"payload": map[string]string{"token": token},
	})
	if err := conn.WriteMessage(websocket.TextMessage, authMsg); err != nil {
		conn.Close()
		t.Fatalf("send auth: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, frame, err := conn.ReadMessage()
	conn.SetReadDeadline(time.Time{})
	return conn, string(frame), err
}

func TestP5R1WebSocketHandshakeRejectsRevokedSession(t *testing.T) {
	validator := &fakeSessionValidator{}
	validator.valid.Store(false)
	_, server := newSessionHub(t, validator)

	conn, frame, err := dialSession(t, server, "00000000-0000-0000-0000-0000000000aa")
	defer conn.Close()
	if err == nil && strings.Contains(frame, "auth_ack") {
		t.Fatalf("revoked session was accepted: %s", frame)
	}
	if err == nil && !strings.Contains(frame, "revoked") {
		t.Fatalf("close frame = %s, want a session-revoked error", frame)
	}
}

func TestP5R1WebSocketHandshakeAcceptsLiveSession(t *testing.T) {
	validator := &fakeSessionValidator{}
	validator.valid.Store(true)
	_, server := newSessionHub(t, validator)

	conn, frame, err := dialSession(t, server, "00000000-0000-0000-0000-0000000000bb")
	defer conn.Close()
	if err != nil {
		t.Fatalf("live session rejected: %v", err)
	}
	if !strings.Contains(frame, "auth_ack") {
		t.Fatalf("first frame = %s, want auth_ack", frame)
	}
}

func TestP5R1DisconnectUserLeavesOtherAccountsConnected(t *testing.T) {
	validator := &fakeSessionValidator{}
	validator.valid.Store(true)
	hub, server := newSessionHub(t, validator)

	first, _, err := dialSession(t, server, "00000000-0000-0000-0000-0000000000c1")
	if err != nil {
		t.Fatalf("first connection: %v", err)
	}
	defer first.Close()
	second, _, err := dialSession(t, server, "00000000-0000-0000-0000-0000000000c2")
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	defer second.Close()
	// Registration is an asynchronous hand-off to the hub loop; wait for both
	// sockets to be registered before asking the hub to disconnect one.
	deadline := time.Now().Add(2 * time.Second)
	for totalClients(hub) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := totalClients(hub); got != 2 {
		t.Fatalf("registered clients = %d, want 2", got)
	}

	if closed := hub.DisconnectUser("00000000-0000-0000-0000-0000000000c1", "session_revoked"); closed != 1 {
		t.Fatalf("disconnected %d connections, want 1", closed)
	}

	// The revoked account's socket reports a close; the other account keeps
	// receiving hub traffic.
	first.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := first.ReadMessage(); err != nil {
			break
		}
	}
	hub.BroadcastToWorkspace(testWorkspaceID, []byte(`{"type":"p5r1"}`))
	second.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, frame, err := second.ReadMessage()
	if err != nil {
		t.Fatalf("an unrelated account lost its connection: %v", err)
	}
	if !strings.Contains(string(frame), "p5r1") {
		t.Fatalf("unexpected frame for the unrelated account: %s", frame)
	}
}

func TestP5R1WatcherClosesAConnectionWhoseSessionWasRevoked(t *testing.T) {
	previous := wsSessionRevalidateInterval
	wsSessionRevalidateInterval = 20 * time.Millisecond
	t.Cleanup(func() { wsSessionRevalidateInterval = previous })

	validator := &fakeSessionValidator{}
	validator.valid.Store(true)
	_, server := newSessionHub(t, validator)

	conn, _, err := dialSession(t, server, "00000000-0000-0000-0000-0000000000d1")
	defer conn.Close()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	validator.valid.Store(false)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, err := conn.ReadMessage(); err != nil {
			return // closed, as required
		}
	}
	t.Fatal("a revoked session kept its WebSocket open")
}
