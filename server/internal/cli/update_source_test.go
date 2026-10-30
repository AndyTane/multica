package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReleaseSourceHasNoDefault(t *testing.T) {
	t.Setenv(releaseAPIBaseURLEnv, "")
	t.Setenv(releaseDownloadBaseURLEnv, "")

	if got := releaseAPIBaseURL(); got != "" {
		t.Fatalf("release API base URL = %q, want %q", got, "")
	}
	if got := releaseAssetDownloadURL("archive.tar.gz", "v1.2.3", "https://github.example/archive.tar.gz"); got != "https://github.example/archive.tar.gz" {
		t.Fatalf("fallback download URL = %q", got)
	}
}

func TestReleaseSourceOverridesGitHub(t *testing.T) {
	t.Setenv(releaseAPIBaseURLEnv, "https://mirror.example/api/")
	t.Setenv(releaseDownloadBaseURLEnv, "https://mirror.example/releases/")

	if got := releaseAPIBaseURL(); got != "https://mirror.example/api" {
		t.Fatalf("release API base URL = %q", got)
	}
	if got := releaseAssetDownloadURL("multica-cli-1.2.3-linux-amd64.tar.gz", "v1.2.3", "https://github.example/archive.tar.gz"); got != "https://mirror.example/releases/v1.2.3/multica-cli-1.2.3-linux-amd64.tar.gz" {
		t.Fatalf("mirror download URL = %q", got)
	}
}

func TestFetchLatestReleaseUsesConfiguredAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/fork/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GitHubRelease{TagName: "v9.8.7"})
	}))
	defer server.Close()
	t.Setenv(releaseAPIBaseURLEnv, server.URL+"/repos/owner/fork/releases")

	got, err := FetchLatestRelease()
	if err != nil {
		t.Fatalf("FetchLatestRelease() error = %v", err)
	}
	if got.TagName != "v9.8.7" {
		t.Fatalf("release tag = %q, want v9.8.7", got.TagName)
	}
}

func TestUpdateRequiresExplicitSource(t *testing.T) {
	t.Setenv(releaseAPIBaseURLEnv, "")
	if _, err := FetchLatestRelease(); err == nil {
		t.Fatal("missing source accepted")
	}
	if _, err := fetchReleaseByTag("v1.2.3"); err == nil {
		t.Fatal("missing source accepted")
	}
	t.Setenv("MULTICA_BREW_PACKAGE", "")
	if _, err := UpdateViaBrew(); err == nil {
		t.Fatal("missing formula accepted")
	}
}

func TestFetchReleaseByTagUsesConfiguredCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom/releases/tags/v2.0.0" {
			t.Errorf("unexpected source path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(GitHubRelease{TagName: "v2.0.0"})
	}))
	defer server.Close()
	t.Setenv(releaseAPIBaseURLEnv, server.URL+"/custom/releases")
	release, err := fetchReleaseByTag("v2.0.0")
	if err != nil || release.TagName != "v2.0.0" {
		t.Fatalf("configured release = %v, error = %v", release, err)
	}
}
