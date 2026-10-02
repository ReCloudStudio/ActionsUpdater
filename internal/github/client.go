package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Tag struct {
	Name   string
	Commit struct {
		SHA  string `json:"sha"`
		Type string `json:"type"`
	} `json:"commit"`
	Time time.Time
}
type tagResponse struct {
	Name   string `json:"name"`
	Object struct {
		SHA  string `json:"sha"`
		Type string `json:"type"`
	} `json:"object"`
	Tagger struct {
		Date time.Time `json:"date"`
	} `json:"tagger"`
}
type commitResponse struct {
	Commit struct {
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

type Client struct {
	BaseURL        string
	HTTP           *http.Client
	Token, Version string
}

func New(version, token string) *Client {
	return &Client{BaseURL: "https://api.github.com", HTTP: http.DefaultClient, Token: token, Version: version}
}

func (c *Client) request(ctx context.Context, endpoint string, value any) (http.Header, error) {
	url := strings.TrimRight(c.BaseURL, "/") + endpoint
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("User-Agent", "actions-updater/"+c.Version)
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			last = err
		} else {
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				err = json.NewDecoder(resp.Body).Decode(value)
				resp.Body.Close()
				return resp.Header, err
			}
			resp.Body.Close()
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
			if resp.StatusCode < 500 && resp.StatusCode != 429 && resp.StatusCode != 403 {
				return nil, last
			}
			if attempt == 3 {
				return nil, last
			}
			wait := time.Duration(1<<attempt) * time.Second
			if seconds, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil && seconds > 0 {
				wait = time.Duration(seconds) * time.Second
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		if attempt == 3 {
			return nil, last
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Second):
		}
	}
	return nil, last
}

func (c *Client) Tags(ctx context.Context, repo string) ([]Tag, error) {
	return collectTags(ctx, repo, c)
}

func (c *Client) listTags(ctx context.Context, repo string) ([]tagEntry, error) {
	escaped, err := escapeRepo(repo)
	if err != nil {
		return nil, err
	}
	var all []tagEntry
	for page := 1; ; page++ {
		var result []tagEntry
		header, err := c.request(ctx, fmt.Sprintf("/repos/%s/tags?per_page=100&page=%d", escaped, page), &result)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.ToLower(repo), err)
		}
		all = append(all, result...)
		if !strings.Contains(header.Get("Link"), `rel="next"`) {
			break
		}
	}
	return all, nil
}

func (c *Client) tagObject(ctx context.Context, repo, sha string) (tagResponse, error) {
	escaped, err := escapeRepo(repo)
	if err != nil {
		return tagResponse{}, err
	}
	var result tagResponse
	if _, err := c.request(ctx, fmt.Sprintf("/repos/%s/git/tags/%s", escaped, sha), &result); err != nil {
		return tagResponse{}, err
	}
	return result, nil
}

func (c *Client) commitInfo(ctx context.Context, repo, sha string) (commitResponse, error) {
	escaped, err := escapeRepo(repo)
	if err != nil {
		return commitResponse{}, err
	}
	var result commitResponse
	if _, err := c.request(ctx, fmt.Sprintf("/repos/%s/git/commits/%s", escaped, sha), &result); err != nil {
		return commitResponse{}, err
	}
	return result, nil
}
