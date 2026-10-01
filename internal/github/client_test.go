package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTagsPaginatesAndResolvesTimes(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path+"?"+r.URL.RawQuery)
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("X-GitHub-Api-Version") == "" {
			t.Error("missing GitHub API headers")
		}
		switch r.URL.Path {
		case "/repos/owner/repo/tags":
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("Link", `<x>; rel="next"`)
				w.Write([]byte(`[{"name":"v1","commit":{"sha":"abc","type":"commit"}}]`))
			} else {
				w.Write([]byte(`[{"name":"v2","commit":{"sha":"def","type":"tag"}}]`))
			}
		case "/repos/owner/repo/git/commits/abc":
			w.Write([]byte(`{"commit":{"committer":{"date":"2024-01-01T00:00:00Z"}}}`))
		case "/repos/owner/repo/git/tags/def":
			w.Write([]byte(`{"object":{"sha":"ghi","type":"commit"},"tagger":{"date":"2025-01-01T00:00:00Z"}}`))
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, HTTP: server.Client(), Version: "test"}
	tags, err := c.Tags(context.Background(), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[1].Time.IsZero() || tags[1].Commit.SHA != "ghi" {
		t.Fatalf("unexpected tags: %#v", tags)
	}
	if !strings.Contains(strings.Join(requests, "\n"), "page=2") {
		t.Fatalf("did not paginate: %v", requests)
	}
}

func TestRequestRetriesRateLimit(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests < 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, HTTP: server.Client(), Version: "test"}
	start := time.Now()
	if _, err := c.request(context.Background(), "/test", &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || time.Since(start) < time.Second {
		t.Fatalf("requests = %d, elapsed = %s", requests, time.Since(start))
	}
}
