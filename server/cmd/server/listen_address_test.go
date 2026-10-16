package main

import "testing"

func TestMainListenAddress(t *testing.T) {
	for _, tc := range []struct {
		name string
		host string
		port string
		want string
	}{
		{name: "existing default", want: ":8080"},
		{name: "existing custom port", port: "9000", want: ":9000"},
		{name: "loopback", host: "127.0.0.1", port: "8080", want: "127.0.0.1:8080"},
		{name: "IPv6 loopback", host: "::1", port: "8080", want: "[::1]:8080"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOST", tc.host)
			t.Setenv("PORT", tc.port)
			if got := mainListenAddress(); got != tc.want {
				t.Fatalf("mainListenAddress() = %q, want %q", got, tc.want)
			}
		})
	}
}
