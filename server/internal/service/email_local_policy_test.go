package service

import (
	"strings"
	"testing"

	"github.com/resend/resend-go/v2"
)

func TestInvitationEmailRequiresConfiguredAppURL(t *testing.T) {
	t.Setenv("MULTICA_APP_URL", "")
	t.Setenv("FRONTEND_ORIGIN", "")
	s := &EmailService{}
	if err := s.SendInvitationEmail("user@example.test", "Inviter", "Workspace", "id"); err == nil || !strings.Contains(err.Error(), "MULTICA_APP_URL") {
		t.Fatalf("missing app URL error = %v", err)
	}
	for _, key := range []string{"MULTICA_APP_URL", "FRONTEND_ORIGIN"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "https://workspace.example.test/")
			if err := s.SendInvitationEmail("user@example.test", "Inviter", "Workspace", "id"); err != nil {
				t.Fatalf("configured app URL rejected: %v", err)
			}
		})
	}
}

func TestResendRequiresExplicitSenderBeforeSending(t *testing.T) {
	t.Setenv("MULTICA_APP_URL", "https://workspace.example.test")
	// An uninitialized client has no transport: even a guard regression cannot
	// send a real email from this test.
	s := &EmailService{client: &resend.Client{}}
	for _, send := range []func() error{
		func() error { return s.SendVerificationCode("user@example.test", "123456") },
		func() error { return s.SendInvitationEmail("user@example.test", "Inviter", "Workspace", "id") },
	} {
		if err := send(); err == nil || !strings.Contains(err.Error(), "RESEND_FROM_EMAIL") {
			t.Fatalf("missing sender error = %v", err)
		}
	}
}
