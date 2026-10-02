package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type CLI struct {
	Token, Version string
	Run            func(ctx context.Context, args ...string) ([]byte, error)
}

func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func NewCLI(version, token string) *CLI {
	return &CLI{Token: token, Version: version}
}

func (c *CLI) Tags(ctx context.Context, repo string) ([]Tag, error) {
	return collectTags(ctx, repo, c)
}

func (c *CLI) run(ctx context.Context, args ...string) ([]byte, error) {
	if c.Run != nil {
		return c.Run(ctx, args...)
	}
	return c.exec(ctx, args...)
}

func (c *CLI) exec(ctx context.Context, args ...string) ([]byte, error) {
	if !Available() {
		return nil, fmt.Errorf("未找到 gh 命令")
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	env := os.Environ()
	if c.Token != "" && os.Getenv("GH_TOKEN") == "" && os.Getenv("GITHUB_TOKEN") == "" {
		env = append(env, "GH_TOKEN="+c.Token)
	}
	cmd.Env = append(env, "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, msg)
		}
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func (c *CLI) listTags(ctx context.Context, repo string) ([]tagEntry, error) {
	escaped, err := escapeRepo(repo)
	if err != nil {
		return nil, err
	}
	out, err := c.run(ctx, "api", "-X", "GET", "repos/"+escaped+"/tags", "-F", "per_page=100", "--paginate", "--slurp")
	if err != nil {
		return nil, err
	}
	var pages [][]tagEntry
	if err := json.Unmarshal(out, &pages); err == nil {
		var all []tagEntry
		for _, page := range pages {
			all = append(all, page...)
		}
		return all, nil
	}
	var flat []tagEntry
	if err := json.Unmarshal(out, &flat); err != nil {
		return nil, fmt.Errorf("解析 gh 输出失败: %w", err)
	}
	return flat, nil
}

func (c *CLI) tagObject(ctx context.Context, repo, sha string) (tagResponse, error) {
	escaped, err := escapeRepo(repo)
	if err != nil {
		return tagResponse{}, err
	}
	var result tagResponse
	if err := c.get(ctx, fmt.Sprintf("repos/%s/git/tags/%s", escaped, sha), &result); err != nil {
		return tagResponse{}, err
	}
	return result, nil
}

func (c *CLI) commitInfo(ctx context.Context, repo, sha string) (commitResponse, error) {
	escaped, err := escapeRepo(repo)
	if err != nil {
		return commitResponse{}, err
	}
	var result commitResponse
	if err := c.get(ctx, fmt.Sprintf("repos/%s/git/commits/%s", escaped, sha), &result); err != nil {
		return commitResponse{}, err
	}
	return result, nil
}

func (c *CLI) get(ctx context.Context, endpoint string, out any) error {
	raw, err := c.run(ctx, "api", "-X", "GET", endpoint)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
