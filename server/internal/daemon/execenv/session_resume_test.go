package execenv

import (
	"os"
	"testing"
)

func TestExistingRootDirIsReadOnly(t *testing.T) {
	root := t.TempDir()
	workspaceID, taskID := "8daf5cf8-6287-44d5-84cd-6e7d72e80f35", "01a136db-92cf-75b1-a804-f42445c19c9b"
	if _, err := ExistingRootDir(root, workspaceID, taskID); !os.IsNotExist(err) {
		t.Fatalf("missing index: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("read created state: %v %v", entries, err)
	}
	want, err := ResolveRootDir(RootDirParams{WorkspacesRoot: root, WorkspaceID: workspaceID, TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExistingRootDir(root, workspaceID, taskID)
	if err != nil || got != want {
		t.Fatalf("existing index = %q, %v; want %q", got, err, want)
	}
}
