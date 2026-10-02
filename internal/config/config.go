package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	BackendGH   = "gh"
	BackendHTTP = "http"
)

var lookPath = exec.LookPath

type Config struct {
	GHToken string `json:"gh_token,omitempty"`
	Backend string `json:"backend"`
}

func Path() string {
	if p := os.Getenv("ACTIONS_UPDATER_CONFIG"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "actions-updater", "config.json")
}

func DetectBackend() string {
	if _, err := lookPath("gh"); err == nil {
		return BackendGH
	}
	return BackendHTTP
}

func Load() (cfg *Config, created bool, err error) {
	path := Path()
	if path == "" {
		return nil, false, errors.New("无法确定配置文件路径")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, false, fmt.Errorf("读取配置文件 %s: %w", path, err)
		}
		cfg = &Config{Backend: DetectBackend()}
		if err := save(path, cfg); err != nil {
			return nil, false, err
		}
		return cfg, true, nil
	}
	cfg = &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, false, fmt.Errorf("解析配置文件 %s: %w", path, err)
	}
	if cfg.Backend == "" {
		cfg.Backend = BackendHTTP
	}
	if cfg.Backend != BackendGH && cfg.Backend != BackendHTTP {
		return nil, false, fmt.Errorf("配置文件 %s 的 backend=%q 无效，应为 gh 或 http", path, cfg.Backend)
	}
	return cfg, false, nil
}

func save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("写入配置文件 %s: %w", path, err)
	}
	return nil
}
