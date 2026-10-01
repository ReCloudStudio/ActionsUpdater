package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	workflowDir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflow := filepath.Join(workflowDir, "ci.yml")
	if err := os.WriteFile(workflow, []byte("uses: actions/checkout@v4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := Discover([]string{root}, false)
	if err != nil || len(files) != 1 || files[0] != workflow {
		t.Fatalf("Discover() = %v, %v", files, err)
	}
}

func TestAtomicReplaceRejectsConcurrentChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.yml")
	original := []byte("uses: actions/checkout@v3\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AtomicReplace(path, original, []byte("updated\n")); err == nil {
		t.Fatal("concurrent change accepted")
	}
}
