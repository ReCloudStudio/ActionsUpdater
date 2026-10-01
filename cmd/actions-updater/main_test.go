package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gh "github.com/ReCloudStudio/ActionsUpdater/internal/github"
)

func TestRunRejectsConflictingFlags(t *testing.T) {
	if code := run([]string{"--confirm", "--dry-run", "x.yml"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Fatalf("run() = %d", code)
	}
}

func TestRunRejectsNoInput(t *testing.T) {
	if code := run(nil, nil, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Fatalf("run() = %d", code)
	}
}

func TestRunRejectsInvalidTimeout(t *testing.T) {
	if code := run([]string{"--timeout", "0s", "x.yml"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Fatalf("run() = %d", code)
	}
}

func TestRunJSONDoesNotWriteWithoutConfirm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ci.yml")
	original := []byte("uses: ./local-action\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"--json", path}, bytes.NewBuffer(nil), &out, &errOut); code != 0 {
		t.Fatalf("run() = %d: %s", code, errOut.String())
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("file changed: %q, %v", got, err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"skipped"`)) {
		t.Fatalf("not JSON report: %q", out.String())
	}
}

func TestFetchTagsStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, failures := fetchTags(ctx, map[string]struct{}{"owner/repo": {}}, options{concurrency: 1}, func(ctx context.Context, _ string) ([]gh.Tag, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if failures["owner/repo"] == nil || failures["owner/repo"] != context.Canceled {
		t.Fatalf("failures = %#v", failures)
	}
}

func TestFetchTagsHonorsTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, failures := fetchTags(ctx, map[string]struct{}{"owner/repo": {}}, options{concurrency: 1}, func(ctx context.Context, _ string) ([]gh.Tag, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if failures["owner/repo"] == nil || failures["owner/repo"] != context.DeadlineExceeded {
		t.Fatalf("failures = %#v", failures)
	}
}
