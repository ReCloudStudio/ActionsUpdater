package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func stubLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	original := lookPath
	lookPath = fn
	t.Cleanup(func() { lookPath = original })
}

func TestPathPrefersEnvOverride(t *testing.T) {
	t.Setenv("ACTIONS_UPDATER_CONFIG", "/tmp/custom.json")
	if got := Path(); got != "/tmp/custom.json" {
		t.Fatalf("Path() = %q", got)
	}
	t.Setenv("ACTIONS_UPDATER_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	want := filepath.Join("/tmp/xdg", "actions-updater", "config.json")
	if got := Path(); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestDetectBackendUsesGHWhenAvailable(t *testing.T) {
	stubLookPath(t, func(string) (string, error) { return "/usr/bin/gh", nil })
	if got := DetectBackend(); got != BackendGH {
		t.Fatalf("DetectBackend() = %q, want %q", got, BackendGH)
	}
}

func TestDetectBackendFallsBackToHTTP(t *testing.T) {
	stubLookPath(t, func(string) (string, error) { return "", os.ErrNotExist })
	if got := DetectBackend(); got != BackendHTTP {
		t.Fatalf("DetectBackend() = %q, want %q", got, BackendHTTP)
	}
}

func TestLoadCreatesConfigWithDetectedBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	t.Setenv("ACTIONS_UPDATER_CONFIG", path)
	stubLookPath(t, func(string) (string, error) { return "/usr/bin/gh", nil })
	cfg, created, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}
	if cfg.Backend != BackendGH || cfg.GHToken != "" {
		t.Fatalf("cfg = %#v", cfg)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config mode = %v, want no group/other access", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk Config
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.Backend != BackendGH {
		t.Fatalf("on-disk backend = %q", onDisk.Backend)
	}
}

func TestLoadCreatesConfigHTTPWithoutGH(t *testing.T) {
	t.Setenv("ACTIONS_UPDATER_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	stubLookPath(t, func(string) (string, error) { return "", os.ErrNotExist })
	cfg, created, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !created || cfg.Backend != BackendHTTP {
		t.Fatalf("cfg = %#v, created = %v", cfg, created)
	}
}

func TestLoadReadsExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("ACTIONS_UPDATER_CONFIG", path)
	content := []byte(`{"gh_token":"secret","backend":"gh"}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, created, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("created = true, want false")
	}
	if cfg.GHToken != "secret" || cfg.Backend != BackendGH {
		t.Fatalf("cfg = %#v", cfg)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("config file modified: %q", got)
	}
}

func TestLoadDefaultsEmptyBackendToHTTP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("ACTIONS_UPDATER_CONFIG", path)
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backend != BackendHTTP {
		t.Fatalf("backend = %q, want %q", cfg.Backend, BackendHTTP)
	}
}

func TestLoadRejectsInvalidBackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("ACTIONS_UPDATER_CONFIG", path)
	if err := os.WriteFile(path, []byte(`{"backend":"gitee"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid backend error")
	}
}

func TestLoadRejectsBrokenJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("ACTIONS_UPDATER_CONFIG", path)
	if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want parse error")
	}
}
