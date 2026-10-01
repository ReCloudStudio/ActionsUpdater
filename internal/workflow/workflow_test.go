package workflow

import (
	"strings"
	"testing"
)

func TestParseAndApplyPreservesFormatting(t *testing.T) {
	data := []byte("steps:\r\n  - uses: 'actions/checkout@v3' # keep\r\n  - uses: docker://alpine\r\n  - uses: owner/repo/path@v1\r\n")
	uses, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 3 || uses[0].Repo != "actions/checkout" || uses[0].Ref != "v3" || uses[1].Skip == "" || uses[2].Path != "owner/repo/path" {
		t.Fatalf("unexpected uses: %#v", uses)
	}
	updated, err := Apply(data, []Replacement{{Start: uses[0].Start, End: uses[0].End, Value: "'actions/checkout@v4'"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "'actions/checkout@v4' # keep\r\n") {
		t.Fatalf("format was not preserved: %q", updated)
	}
}

func TestParseMultiDocument(t *testing.T) {
	uses, err := Parse([]byte("---\nuses: actions/checkout@v3\n---\n  uses: actions/setup-go@v5\n"))
	if err != nil || len(uses) != 2 {
		t.Fatalf("Parse() = %#v, %v", uses, err)
	}
}

func TestCheckText(t *testing.T) {
	if err := CheckText([]byte("a\r\nb\n")); err == nil {
		t.Fatal("mixed newlines accepted")
	}
	if err := CheckText([]byte("a\rb")); err == nil {
		t.Fatal("CR newlines accepted")
	}
	if err := CheckText([]byte{0xff}); err == nil {
		t.Fatal("non-UTF-8 accepted")
	}
}
