package main

import (
	"testing"

	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// testFeatureFlags builds the env-backed flag service main() boots with, so a
// test can set FF_<KEY> with t.Setenv and have the real router assembly read
// it. Build it AFTER the t.Setenv calls: the env provider snapshots nothing,
// but NewServiceFromEnv decides the provider chain at construction time.
func testFeatureFlags(t *testing.T) *featureflag.Service {
	t.Helper()
	flags, err := featureflag.NewServiceFromEnv()
	if err != nil {
		t.Fatalf("build feature flag service: %v", err)
	}
	return flags
}
