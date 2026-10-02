package github

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCLITagsSkipsTimeLookupForSemver(t *testing.T) {
	var calls []string
	c := &CLI{}
	c.Run = func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		joined := strings.Join(args, " ")
		if slices.Contains(args, "--slurp") {
			if !slices.Contains(args, "repos/owner/repo/tags") || !slices.Contains(args, "--paginate") {
				t.Errorf("listTags args = %q", joined)
			}
			return []byte(`[[{"name":"v1","commit":{"sha":"aaa","type":"commit"}}],[{"name":"v2","commit":{"sha":"bbb","type":"tag"}}]]`), nil
		}
		return nil, errors.New("unexpected gh call: " + joined)
	}
	tags, err := c.Tags(context.Background(), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("len(tags) = %d", len(tags))
	}
	if !tags[0].Time.IsZero() || !tags[1].Time.IsZero() ||
		tags[0].Commit.SHA != "aaa" || tags[1].Commit.SHA != "bbb" {
		t.Fatalf("tags = %#v", tags)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %v", calls)
	}
}

func TestCLITagsResolvesAnnotatedAndCommitTimes(t *testing.T) {
	var calls []string
	c := &CLI{}
	c.Run = func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		joined := strings.Join(args, " ")
		switch {
		case slices.Contains(args, "--slurp"):
			if !slices.Contains(args, "repos/owner/repo/tags") || !slices.Contains(args, "--paginate") {
				t.Errorf("listTags args = %q", joined)
			}
			return []byte(`[[{"name":"latest","commit":{"sha":"aaa","type":"commit"}}],[{"name":"nightly","commit":{"sha":"bbb","type":"tag"}}]]`), nil
		case slices.Contains(args, "repos/owner/repo/git/commits/aaa"):
			return []byte(`{"commit":{"committer":{"date":"2024-01-01T00:00:00Z"}}}`), nil
		case slices.Contains(args, "repos/owner/repo/git/tags/bbb"):
			return []byte(`{"object":{"sha":"ccc","type":"commit"},"tagger":{"date":"2025-01-01T00:00:00Z"}}`), nil
		}
		return nil, errors.New("unexpected gh call: " + joined)
	}
	tags, err := c.Tags(context.Background(), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("len(tags) = %d", len(tags))
	}
	if tags[0].Time.IsZero() || tags[1].Time.IsZero() || tags[1].Commit.SHA != "ccc" {
		t.Fatalf("tags = %#v", tags)
	}
	if len(calls) != 3 {
		t.Fatalf("calls = %v", calls)
	}
}

func TestCLITagsAcceptsFlatJSON(t *testing.T) {
	c := &CLI{}
	c.Run = func(_ context.Context, args ...string) ([]byte, error) {
		if slices.Contains(args, "--slurp") {
			return []byte(`[{"name":"v1","commit":{"sha":"abc","type":"commit"}}]`), nil
		}
		return []byte(`{"commit":{"committer":{"date":"2024-01-01T00:00:00Z"}}}`), nil
	}
	tags, err := c.Tags(context.Background(), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].Name != "v1" {
		t.Fatalf("tags = %#v", tags)
	}
}

func TestCLITagsRejectsInvalidRepo(t *testing.T) {
	c := &CLI{}
	c.Run = func(context.Context, ...string) ([]byte, error) {
		t.Fatal("gh should not run for invalid repo")
		return nil, nil
	}
	if _, err := c.Tags(context.Background(), "nodivider"); err == nil {
		t.Fatal("error = nil, want invalid repo error")
	}
}

func TestCLITagsPropagatesGHErrors(t *testing.T) {
	c := &CLI{}
	c.Run = func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("gh api failed")
	}
	if _, err := c.Tags(context.Background(), "owner/repo"); err == nil || !strings.Contains(err.Error(), "gh api failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestCLIGetsRejectsBrokenJSON(t *testing.T) {
	c := &CLI{}
	c.Run = func(context.Context, ...string) ([]byte, error) {
		return []byte("nope"), nil
	}
	if _, err := c.Tags(context.Background(), "owner/repo"); err == nil {
		t.Fatal("error = nil, want JSON parse error")
	}
}
