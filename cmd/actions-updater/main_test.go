package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/muesli/termenv"

	"github.com/ReCloudStudio/ActionsUpdater/internal/config"
	gh "github.com/ReCloudStudio/ActionsUpdater/internal/github"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "actions-updater-config")
	if err != nil {
		panic(err)
	}
	os.Setenv("ACTIONS_UPDATER_CONFIG", filepath.Join(dir, "config.json"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

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

func TestRunHelpIncludesOptionsAndExamples(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--help"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("run() = %d", code)
	}
	for _, want := range []string{"-dry-run", "-timeout", "actions-updater --same-major --confirm ."} {
		if !bytes.Contains(errOut.Bytes(), []byte(want)) {
			t.Fatalf("help output missing %q: %s", want, errOut.String())
		}
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

func TestRunRejectsInvalidBackend(t *testing.T) {
	var errOut bytes.Buffer
	if code := run([]string{"--backend", "gitee", "x.yml"}, nil, &bytes.Buffer{}, &errOut); code != 2 {
		t.Fatalf("run() = %d", code)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("gh 或 http")) {
		t.Fatalf("errOut = %q", errOut.String())
	}
}

func TestRunBackendFlagOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ACTIONS_UPDATER_CONFIG", filepath.Join(dir, "config.json"))
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"backend":"http"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	var errOut bytes.Buffer
	if code := run([]string{"--backend", "gh", "x.yml"}, nil, &bytes.Buffer{}, &errOut); code != 2 {
		t.Fatalf("run() = %d", code)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("需要 gh 命令")) {
		t.Fatalf("errOut = %q", errOut.String())
	}
}

func TestRunRequiresGHWhenConfigSelectsIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ACTIONS_UPDATER_CONFIG", filepath.Join(dir, "config.json"))
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"backend":"gh"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	var errOut bytes.Buffer
	if code := run([]string{"x.yml"}, nil, &bytes.Buffer{}, &errOut); code != 2 {
		t.Fatalf("run() = %d", code)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("需要 gh 命令")) {
		t.Fatalf("errOut = %q", errOut.String())
	}
}

func TestRunCreatesConfigOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("ACTIONS_UPDATER_CONFIG", path)
	flow := filepath.Join(dir, "ci.yml")
	if err := os.WriteFile(flow, []byte("uses: ./local-action\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"--json", flow}, bytes.NewBuffer(nil), &out, &errOut); code != 0 {
		t.Fatalf("run() = %d: %s", code, errOut.String())
	}
	if !bytes.Contains(errOut.Bytes(), []byte("已创建配置文件")) {
		t.Fatalf("errOut = %q", errOut.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Backend string `json:"backend"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Backend != config.DetectBackend() {
		t.Fatalf("backend = %q, want %q", cfg.Backend, config.DetectBackend())
	}
}

func TestRunKeepsExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("ACTIONS_UPDATER_CONFIG", path)
	content := []byte(`{"gh_token":"secret","backend":"http"}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	flow := filepath.Join(dir, "ci.yml")
	if err := os.WriteFile(flow, []byte("uses: ./local-action\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"--json", flow}, bytes.NewBuffer(nil), &out, &errOut); code != 0 {
		t.Fatalf("run() = %d: %s", code, errOut.String())
	}
	if bytes.Contains(errOut.Bytes(), []byte("已创建配置文件")) {
		t.Fatalf("errOut = %q", errOut.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("config modified: %q", got)
	}
}

func TestResolveTokenPrefersEnv(t *testing.T) {
	cfg := &config.Config{GHToken: "from-config"}
	t.Setenv("GITHUB_TOKEN", "from-env")
	if got := resolveToken(cfg); got != "from-env" {
		t.Fatalf("resolveToken() = %q", got)
	}
	t.Setenv("GITHUB_TOKEN", "")
	if got := resolveToken(cfg); got != "from-config" {
		t.Fatalf("resolveToken() = %q", got)
	}
}

func TestRunPrintsSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ci.yml")
	if err := os.WriteFile(path, []byte("uses: actions/checkout@v4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"--dry-run", "--only", "foo/bar", path}, bytes.NewBuffer(nil), &out, &errOut); code != 0 {
		t.Fatalf("run() = %d: %s", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("共更新 0 处，跳过 0 处，错误 0 处")) {
		t.Fatalf("out = %q", out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"--json", "--only", "foo/bar", path}, bytes.NewBuffer(nil), &out, &errOut); code != 0 {
		t.Fatalf("run() = %d: %s", code, errOut.String())
	}
	var r report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("json output not clean: %v: %q", err, out.String())
	}
}

func TestRunColorsOutputWhenForced(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	path := filepath.Join(t.TempDir(), "ci.yml")
	if err := os.WriteFile(path, []byte("uses: actions/checkout@v4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := run([]string{"--dry-run", "--only", "foo/bar", path}, bytes.NewBuffer(nil), &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("run() = %d", code)
	}
	if !bytes.Contains(out.Bytes(), []byte("\x1b[")) {
		t.Fatalf("expected escapes, out = %q", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("共更新")) {
		t.Fatalf("out = %q", out.String())
	}
}

func TestProgressLines(t *testing.T) {
	var buf bytes.Buffer
	p := &progress{w: &buf, o: termenv.NewOutput(&buf), total: 2, label: "查询中", detail: "actions 10"}
	wrapped := wrapProgress(func(_ context.Context, repo string) ([]gh.Tag, error) {
		if repo == "owner/bad" {
			return nil, errors.New("boom")
		}
		return nil, nil
	}, p)
	if _, err := wrapped(context.Background(), "owner/good"); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped(context.Background(), "owner/bad"); err == nil {
		t.Fatal("expected error")
	}
	p.finish()
	for _, want := range []string{"查询中 1/2 · actions 10", "错误: owner/bad: boom", "查询中 2/2 · actions 10"} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Fatalf("progress missing %q: %q", want, buf.String())
		}
	}
}

func TestProgressParsePhase(t *testing.T) {
	var buf bytes.Buffer
	p := &progress{w: &buf, o: termenv.NewOutput(&buf), total: 8, label: "解析中", detail: "actions 12"}
	p.render(3)
	p.finish()
	for _, want := range []string{"解析中 3/8 · actions 12", "\r\x1b[K"} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Fatalf("progress missing %q: %q", want, buf.String())
		}
	}
}

func TestWriterTTY(t *testing.T) {
	if writerTTY(&bytes.Buffer{}) {
		t.Fatal("buffer must not be tty")
	}
	if writerTTY(nil) {
		t.Fatal("nil must not be tty")
	}
}
