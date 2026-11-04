package managedkit

import (
	"strings"
	"testing"
	"time"
)

func TestReviewInFlightTransactionRejectsDifferentOffer(t *testing.T) {
	old := Offer{Version: "v0.2.11", Commit: strings.Repeat("a", 40), Platform: "windows-amd64", SHA256: strings.Repeat("a", 64), Bytes: 1}
	next := old
	next.Version = "v0.2.12"
	next.SHA256 = strings.Repeat("b", 64)
	for _, phase := range []string{"switching", "verifying", "interrupted"} {
		t.Run(phase, func(t *testing.T) {
			j := Journal{Offer: old, Phase: phase, Attempts: 1}
			if CanAttempt(j, next, time.Now()) {
				t.Fatal("new offer admitted while previous installation transaction is unfinished")
			}
		})
	}
}
