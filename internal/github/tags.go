package github

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type tagEntry struct {
	Name   string `json:"name"`
	Commit struct {
		SHA  string `json:"sha"`
		Type string `json:"type"`
	} `json:"commit"`
}

type tagAPI interface {
	listTags(ctx context.Context, repo string) ([]tagEntry, error)
	tagObject(ctx context.Context, repo, sha string) (tagResponse, error)
	commitInfo(ctx context.Context, repo, sha string) (commitResponse, error)
}

func collectTags(ctx context.Context, repo string, api tagAPI) ([]Tag, error) {
	entries, err := api.listTags(ctx, repo)
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(repo)
	tags := make([]Tag, 0, len(entries))
	for _, entry := range entries {
		tag := Tag{Name: entry.Name, Commit: entry.Commit}
		if entry.Commit.Type == "tag" {
			resolved, err := resolveAnnotatedTag(ctx, api, repo, entry.Commit.SHA)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", lower, err)
			}
			tag.Commit = resolved.Commit
			tag.Time = resolved.Time
		} else {
			commit, err := api.commitInfo(ctx, repo, entry.Commit.SHA)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", lower, err)
			}
			tag.Time = commit.Commit.Committer.Date
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

func resolveAnnotatedTag(ctx context.Context, api tagAPI, repo, sha string) (Tag, error) {
	annotated, err := api.tagObject(ctx, repo, sha)
	if err != nil {
		return Tag{}, err
	}
	tag := Tag{Commit: annotated.Object, Time: annotated.Tagger.Date}
	if annotated.Object.Type != "tag" {
		return tag, nil
	}
	next, err := resolveAnnotatedTag(ctx, api, repo, annotated.Object.SHA)
	if err != nil {
		return Tag{}, err
	}
	if tag.Time.IsZero() {
		tag.Time = next.Time
	}
	tag.Commit = next.Commit
	return tag, nil
}

func escapeRepo(repo string) (string, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("无效仓库 %q", repo)
	}
	return url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]), nil
}
