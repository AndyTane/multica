//go:build windows

package agent

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenCodeSelectedConfigReachesTaskProcess(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "external config.jsonc")
	capture := filepath.Join(root, "environment.txt")
	source := filepath.Join(root, "fake.cs")
	binary := filepath.Join(root, "fake-opencode.exe")
	code := `using System; using System.IO;
class Fake {
 static int Main() {
  File.WriteAllLines(Environment.GetEnvironmentVariable("KIT_ENV_CAPTURE"), new [] {
   Environment.GetEnvironmentVariable("OPENCODE_CONFIG") ?? "",
   Environment.GetEnvironmentVariable("MULTICA_TOKEN") ?? "",
   Environment.GetEnvironmentVariable("U_WORKERS_KIT_ROOT") ?? "" });
  Console.WriteLine("{\"type\":\"step_start\",\"sessionID\":\"fake\",\"part\":{\"type\":\"step-start\"}}");
  Console.WriteLine("{\"type\":\"text\",\"part\":{\"type\":\"text\",\"text\":\"fixture\"}}");
  Console.WriteLine("{\"type\":\"step_finish\",\"part\":{\"type\":\"step-finish\",\"reason\":\"stop\"}}");
  return 0;
 }
}`
	if err := os.WriteFile(source, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	csc := filepath.Join(os.Getenv("WINDIR"), "Microsoft.NET", "Framework64", "v4.0.30319", "csc.exe")
	if out, err := exec.Command(csc, "/nologo", "/target:exe", "/out:"+binary, source).CombinedOutput(); err != nil {
		t.Fatalf("fixture compile: %v %s", err, out)
	}
	t.Setenv("OPENCODE_CONFIG", config)
	t.Setenv("MULTICA_TOKEN", "PARENT_MUST_NOT_LEAK")
	t.Setenv("U_WORKERS_KIT_ROOT", root)
	backend, err := New("opencode", Config{ExecutablePath: binary, CLIVersion: "1.17.7", Logger: slog.Default(), Env: map[string]string{"KIT_ENV_CAPTURE": capture}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := backend.Execute(ctx, "fixture", ExecOptions{Cwd: root, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for range session.Messages {
	}
	<-session.Result
	raw, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	want := config + "\r\n\r\n\r\n"
	if string(raw) != want {
		t.Fatalf("task environment did not preserve only selected external config: %q", raw)
	}
}
