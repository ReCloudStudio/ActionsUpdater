package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	gh "github.com/ReCloudStudio/ActionsUpdater/internal/github"
	"github.com/ReCloudStudio/ActionsUpdater/internal/workflow"
)

var version = "dev"
var commit = "none"
var buildDate = "unknown"

type options struct {
	confirm, dryRun, json, sameMajor, prerelease, recursive bool
	concurrency                                             int
	timeout                                                 time.Duration
	only, exclude                                           listFlag
}
type listFlag []string

func (v *listFlag) String() string { return strings.Join(*v, ",") }
func (v *listFlag) Set(s string) error {
	if !validRepo(s) {
		return fmt.Errorf("无效仓库 %q，应为 owner/repo", s)
	}
	*v = append(*v, strings.ToLower(s))
	return nil
}

type update struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Action string `json:"action"`
	Old    string `json:"old"`
	New    string `json:"new"`
}
type report struct {
	Updates []update `json:"updates"`
	Skipped []string `json:"skipped,omitempty"`
	Errors  []string `json:"errors,omitempty"`
	Written bool     `json:"written"`
}

type tagFetcher func(context.Context, string) ([]gh.Tag, error)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
func run(args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("actions-updater", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var o options
	fs.BoolVar(&o.confirm, "confirm", false, "不询问直接写入")
	fs.BoolVar(&o.dryRun, "dry-run", false, "只预览")
	fs.BoolVar(&o.json, "json", false, "输出 JSON")
	fs.BoolVar(&o.sameMajor, "same-major", false, "限制同一主版本")
	fs.BoolVar(&o.prerelease, "include-prerelease", false, "包含预发布标签")
	fs.BoolVar(&o.recursive, "recursive", false, "递归扫描目录")
	fs.IntVar(&o.concurrency, "concurrency", 4, "并发仓库查询数")
	fs.DurationVar(&o.timeout, "timeout", 5*time.Minute, "GitHub 请求总超时")
	fs.Var(&o.only, "only", "仅更新仓库")
	fs.Var(&o.exclude, "exclude", "排除仓库")
	showVersion := fs.Bool("version", false, "显示版本")
	fs.Usage = func() { fmt.Fprintln(errOut, "用法: actions-updater [选项] <文件或目录...>") }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(out, "actions-updater %s (%s, %s)\n", version, commit, buildDate)
		return 0
	}
	if o.confirm && o.dryRun {
		fmt.Fprintln(errOut, "--confirm 与 --dry-run 不能同时使用")
		return 2
	}
	if o.concurrency <= 0 {
		fmt.Fprintln(errOut, "--concurrency 必须大于 0")
		return 2
	}
	if o.timeout <= 0 {
		fmt.Fprintln(errOut, "--timeout 必须大于 0")
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}
	files, e := workflow.Discover(fs.Args(), o.recursive)
	if e != nil {
		fmt.Fprintln(errOut, e)
		return 2
	}
	if len(files) == 0 {
		if o.json {
			json.NewEncoder(out).Encode(report{})
		}
		return 0
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, o.timeout)
	defer cancel()
	r := report{}
	byRepo := map[string]struct{}{}
	parsed := map[string][]workflow.Use{}
	source := map[string][]byte{}
	for _, file := range files {
		if target, linked := workflow.IsSymlink(file); linked {
			r.Skipped = append(r.Skipped, fmt.Sprintf("%s: 符号链接 -> %s（将更新目标文件）", file, target))
		}
		data, e := os.ReadFile(file)
		if e != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("%s: %v", file, e))
			continue
		}
		if e = workflow.CheckText(data); e != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("%s: %v", file, e))
			continue
		}
		uses, e := workflow.Parse(data)
		if e != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("%s: %v", file, e))
			continue
		}
		source[file] = data
		parsed[file] = uses
		for _, u := range uses {
			if u.Skip != "" {
				r.Skipped = append(r.Skipped, fmt.Sprintf("%s:%d: %s", file, u.Line, u.Skip))
				continue
			}
			if allowed(u.Repo, o) {
				byRepo[u.Repo] = struct{}{}
			}
		}
	}
	tags, failures := fetchTags(ctx, byRepo, o, gh.New(version).Tags)
	if signalCtx.Err() != nil {
		return 130
	}
	if ctx.Err() != nil {
		fmt.Fprintf(errOut, "处理超时: %v\n", ctx.Err())
		return 1
	}
	for repo, e := range failures {
		r.Errors = append(r.Errors, fmt.Sprintf("%s: %v", repo, e))
	}
	repls := map[string][]workflow.Replacement{}
	for file, uses := range parsed {
		for _, u := range uses {
			if u.Skip != "" || !allowed(u.Repo, o) {
				continue
			}
			set, ok := tags[u.Repo]
			if !ok {
				continue
			}
			tag, ok := gh.Select(set, u.Ref, o.sameMajor, o.prerelease)
			if !ok {
				r.Skipped = append(r.Skipped, fmt.Sprintf("%s:%d: 没有适合的标签", file, u.Line))
				continue
			}
			if tag.Name == u.Ref {
				continue
			}
			newValue := replaceRef(u.Value, u.Ref, tag.Name)
			repls[file] = append(repls[file], workflow.Replacement{Start: u.Start, End: u.End, Value: newValue})
			r.Updates = append(r.Updates, update{file, u.Line, u.Path, u.Ref, tag.Name})
		}
	}
	sort.Slice(r.Updates, func(i, j int) bool {
		if r.Updates[i].File == r.Updates[j].File {
			return r.Updates[i].Line < r.Updates[j].Line
		}
		return r.Updates[i].File < r.Updates[j].File
	})
	if !o.json {
		printReport(out, r)
	}
	if len(r.Updates) == 0 {
		if o.json {
			json.NewEncoder(out).Encode(r)
		}
		if len(r.Errors) > 0 {
			return 1
		}
		return 0
	}
	if o.dryRun || o.json && !o.confirm {
		if o.json {
			json.NewEncoder(out).Encode(r)
		}
		return status(r)
	}
	if !o.confirm {
		if !isTTY() {
			fmt.Fprintln(errOut, "非交互终端必须传入 --confirm 才能写入")
			return 2
		}
		fmt.Fprint(out, "写入这些更新？[Y/n] ")
		answer, _ := bufio.NewReader(in).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer == "n" || answer == "no" {
			return 130
		}
	}
	for file, rs := range repls {
		updated, e := workflow.Apply(source[file], rs)
		if e == nil {
			e = workflow.AtomicReplace(file, source[file], updated)
		}
		if e != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("%s: %v", file, e))
		} else {
			r.Written = true
		}
	}
	if o.json {
		json.NewEncoder(out).Encode(r)
	}
	return status(r)
}
func fetchTags(ctx context.Context, repos map[string]struct{}, o options, fetch tagFetcher) (map[string][]gh.Tag, map[string]error) {
	result := map[string][]gh.Tag{}
	fail := map[string]error{}
	var mu sync.Mutex
	sem := make(chan struct{}, o.concurrency)
	var wg sync.WaitGroup
	for repo := range repos {
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			tags, e := fetch(ctx, repo)
			mu.Lock()
			defer mu.Unlock()
			if e != nil {
				fail[repo] = e
			} else {
				result[repo] = tags
			}
		}(repo)
	}
	wg.Wait()
	return result, fail
}
func allowed(repo string, o options) bool {
	for _, s := range o.exclude {
		if repo == s {
			return false
		}
	}
	if len(o.only) == 0 {
		return true
	}
	for _, s := range o.only {
		if repo == s {
			return true
		}
	}
	return false
}
func validRepo(s string) bool {
	p := strings.Split(s, "/")
	return len(p) == 2 && p[0] != "" && p[1] != "" && !strings.ContainsAny(s, "@ \\t")
}
func replaceRef(raw, old, new string) string {
	trim := strings.TrimSpace(raw)
	prefix := raw[:strings.Index(raw, trim)]
	suffix := raw[len(prefix)+len(trim):]
	quote := ""
	if len(trim) > 1 && (trim[0] == '\'' || trim[0] == '"') {
		quote = trim[:1]
		trim = trim[1 : len(trim)-1]
	}
	return prefix + quote + strings.TrimSuffix(trim, old) + new + quote + suffix
}
func printReport(w io.Writer, r report) {
	for _, u := range r.Updates {
		fmt.Fprintf(w, "%s:%d: %s@%s -> %s\n", u.File, u.Line, u.Action, u.Old, u.New)
	}
	for _, s := range r.Skipped {
		fmt.Fprintln(w, "跳过:", s)
	}
	for _, e := range r.Errors {
		fmt.Fprintln(w, "错误:", e)
	}
}
func status(r report) int {
	if len(r.Errors) > 0 {
		return 1
	}
	return 0
}
func isTTY() bool {
	info, e := os.Stdout.Stat()
	return e == nil && (info.Mode()&os.ModeCharDevice) != 0
}
