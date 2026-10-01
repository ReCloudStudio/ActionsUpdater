package workflow

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var usesLine = regexp.MustCompile(`(?m)^([ \t]*(?:-[ \t]+)?uses[ \t]*:[ \t]*)([^ \t#\r\n][^#\r\n]*?)([ \t]*(?:#.*)?)(\r?\n|$)`)

type Use struct {
	Line       int `json:"line"`
	Start, End int
	Value      string `json:"value"`
	Repo       string `json:"repository,omitempty"`
	Path       string `json:"path,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Skip       string `json:"skip,omitempty"`
}

type Replacement struct {
	Start, End int
	Value      string
}

func Parse(data []byte) ([]Use, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("不是 UTF-8 文本")
	}
	uses := []Use{}
	for _, m := range usesLine.FindAllSubmatchIndex(data, -1) {
		line := bytes.Count(data[:m[0]], []byte("\n")) + 1
		raw := string(data[m[4]:m[5]])
		u := Use{Line: line, Start: m[4], End: m[5], Value: raw}
		value := strings.TrimSpace(raw)
		if strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">") || strings.Contains(value, "${{") {
			u.Skip = "动态或块标量"
			uses = append(uses, u)
			continue
		}
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		} else if strings.ContainsAny(value, "'\"") {
			u.Skip = "复杂 YAML 标量"
			uses = append(uses, u)
			continue
		}
		switch {
		case strings.HasPrefix(value, "docker://"):
			u.Skip = "Docker action"
		case strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../"):
			u.Skip = "本地 action"
		default:
			at := strings.LastIndex(value, "@")
			if at <= 0 || at == len(value)-1 {
				u.Skip = "缺少静态 ref"
				break
			}
			parts := strings.Split(value[:at], "/")
			if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
				u.Skip = "不是 GitHub action"
				break
			}
			u.Repo = strings.ToLower(parts[0] + "/" + parts[1])
			u.Path = value[:at]
			u.Ref = value[at+1:]
		}
		uses = append(uses, u)
	}
	return uses, nil
}

func Apply(data []byte, replacements []Replacement) ([]byte, error) {
	for i := 1; i < len(replacements); i++ {
		if replacements[i-1].End > replacements[i].Start {
			return nil, fmt.Errorf("重叠替换")
		}
	}
	var out bytes.Buffer
	at := 0
	for _, r := range replacements {
		out.Write(data[at:r.Start])
		out.WriteString(r.Value)
		at = r.End
	}
	out.Write(data[at:])
	return out.Bytes(), nil
}

func IsYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yml" || ext == ".yaml"
}
