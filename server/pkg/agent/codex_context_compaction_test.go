package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCodexContextCompactionWaitsForItsOwnTurn(t *testing.T) {
	c, _, _ := newTestCodexClient(t)
	c.setThreadID("thread-current")
	c.setActiveTurnID("old-turn")
	c.setActiveTurnID("")
	stdin := &fakeStdinWithHook{}
	acked := make(chan struct{})
	stdin.afterWrite = func() { c.handleLine(`{"id":1,"result":{}}`); close(acked) }
	c.stdin = stdin
	done := make(chan error, 1)
	go func() { done <- c.compactContext(t.Context(), time.Second) }()
	<-acked
	select {
	case err := <-done:
		t.Fatalf("ack finished compaction: %v", err)
	default:
	}
	c.handleLine(`{"method":"turn/completed","params":{"threadId":"thread-current","turn":{"id":"old-turn","status":"completed"}}}`)
	c.handleLine(`{"method":"turn/started","params":{"threadId":"thread-current","turn":{"id":"old-turn","status":"inProgress"}}}`)
	c.handleLine(`{"method":"item/completed","params":{"threadId":"thread-current","turnId":"old-turn","item":{"id":"old-compact","type":"contextCompaction"}}}`)
	c.handleLine(`{"method":"turn/completed","params":{"threadId":"thread-current","turn":{"id":"old-turn","status":"completed"}}}`)
	select {
	case err := <-done:
		t.Fatalf("late task event finished maintenance: %v", err)
	default:
	}
	c.handleLine(`{"method":"turn/started","params":{"threadId":"foreign-thread","turn":{"id":"foreign","status":"inProgress"}}}`)
	c.handleLine(`{"method":"turn/started","params":{"threadId":"thread-current","turn":{"id":"compact-turn","status":"inProgress"}}}`)
	c.handleLine(`{"method":"item/completed","params":{"threadId":"thread-current","turnId":"compact-turn","item":{"id":"compact-item","type":"contextCompaction"}}}`)
	c.handleLine(`{"method":"turn/completed","params":{"threadId":"thread-current","turn":{"id":"compact-turn","status":"completed"}}}`)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdin.Lines()[0], `"method":"thread/compact/start"`) {
		t.Fatal(stdin.Lines())
	}
	if c.getTurnError() != "" || c.turnCompleted {
		t.Fatal("maintenance contaminated the task lifecycle")
	}
}

func TestCodexContextCompactionFailuresAreBounded(t *testing.T) {
	for _, mode := range []string{"rpc", "timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			c, _, _ := newTestCodexClient(t)
			c.setThreadID("thread-current")
			stdin := &fakeStdinWithHook{}
			stdin.afterWrite = func() {
				if mode == "rpc" {
					c.handleLine(`{"id":1,"error":{"code":-32601,"message":"unsupported"}}`)
				} else {
					c.handleLine(`{"id":1,"result":{}}`)
				}
			}
			c.stdin = stdin
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			err := c.compactContext(ctx, 20*time.Millisecond)
			if err == nil {
				t.Fatal("accepted failed maintenance")
			}
			if mode == "timeout" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("timeout=%v", err)
			}
			if c.isCompacting() {
				t.Fatal("compaction stayed active")
			}
		})
	}
}
