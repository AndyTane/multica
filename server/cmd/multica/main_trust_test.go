package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/daemon/execenv"
	"github.com/multica-ai/multica/server/internal/tlsx"
)

func TestBootstrapClientTrustSources(t *testing.T) {
	for _, name := range []string{"default-profile", "named-profile", "explicit-env-over-profile", "wrong-env-over-profile", "system-trust-only"} {
		t.Run(name, func(t *testing.T) {
			isolateConnectHome(t)
			deployment := newFakeDeployment(t)
			unrelated := newFakeDeployment(t)
			profile, profileCA, explicitCA := "", deployment.caPath, ""
			if name == "named-profile" {
				profile = "synthetic"
			}
			if name == "explicit-env-over-profile" {
				profileCA, explicitCA = unrelated.caPath, deployment.caPath
			}
			if name == "wrong-env-over-profile" {
				explicitCA = unrelated.caPath
			}
			if name == "system-trust-only" {
				profileCA = ""
			}
			if err := cli.SaveCLIConfigForProfile(cli.CLIConfig{RootCAFile: profileCA}, profile); err != nil {
				t.Fatal(err)
			}
			t.Setenv(tlsx.RootCAEnv, explicitCA)
			cmd := &cobra.Command{}
			cmd.Flags().String("profile", profile, "")
			bootstrapClientTrust(cmd)
			want := profileCA
			if explicitCA != "" {
				want = explicitCA
			}
			if tlsx.EnvPath() != want {
				t.Fatalf("selected CA = %q, want %q", tlsx.EnvPath(), want)
			}
			transport := tlsx.HTTPTransport()
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport}
			response, err := client.Get(deployment.server.URL + "/health")
			wantOK := name != "wrong-env-over-profile" && name != "system-trust-only"
			if response != nil {
				response.Body.Close()
			}
			if (err == nil) != wantOK {
				t.Fatalf("profile/env trust handshake = %v, want success %v", err, wantOK)
			}
			cfg, err := cli.LoadCLIConfigForProfile(profile)
			if err != nil || cfg.RootCAFile != profileCA {
				t.Fatal("trust bootstrap changed the synthetic source profile")
			}
		})
	}
}

func TestBootstrapClientTrustSkipsOwnerProfileInTaskWithoutConfigRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, key := range []string{"MULTICA_TASK_CONFIG_ROOT", "MULTICA_AGENT_ID", "MULTICA_TASK_ID", "MULTICA_DAEMON_PORT", tlsx.RootCAEnv} {
		t.Setenv(key, "")
	}
	if err := tlsx.InstallFromEnv(); err != nil {
		t.Fatal(err)
	}
	owner := filepath.Join(home, ".multica")
	if err := os.MkdirAll(owner, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(owner, "config.json"), []byte(`{"root_ca_file":"owner-profile-must-not-be-read.pem"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	marker := filepath.Join(work, execenv.TaskContextMarkerRelPath)
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte(`{"managed_by":"multica-daemon-task"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	bootstrapClientTrust(cmd)
	if tlsx.EnvPath() != "" || tlsx.Pool() != nil {
		t.Fatal("task trust bootstrap discovered the owner's profile")
	}
}
