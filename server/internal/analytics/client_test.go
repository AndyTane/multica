package analytics

import "testing"

func TestNoopClientLifecycle(t *testing.T) {
	c := NewFromEnv()
	// No queue, flush goroutine, or serialization should be required, even for
	// an event containing a value that cannot be encoded as JSON.
	c.Capture(Event{Name: "private", Properties: map[string]any{"value": make(chan int)}})
	c.Close()
	c.Close()
	c.Capture(Event{Name: "after-close"})
}

func TestEmailDomain(t *testing.T) {
	cases := map[string]string{
		"a@example.com":      "example.com",
		"user@Company.co.uk": "company.co.uk",
		"":                   "",
		"no-at":              "",
		"trailing@":          "",
	}
	for in, want := range cases {
		if got := emailDomain(in); got != want {
			t.Errorf("emailDomain(%q) = %q, want %q", in, got, want)
		}
	}
}
