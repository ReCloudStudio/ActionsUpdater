package workflow

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ryanuber/go-glob"
)

func Discover(inputs []string, recursive bool) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	add := func(path string) error {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		canonical, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return err
		}
		if !seen[canonical] {
			seen[canonical] = true
			files = append(files, abs)
		}
		return nil
	}
	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", input, err)
		}
		if !info.IsDir() {
			if !IsYAML(input) {
				return nil, fmt.Errorf("%s 不是 YAML workflow", input)
			}
			if err := add(input); err != nil {
				return nil, err
			}
			continue
		}
		root := input
		if !recursive {
			root = filepath.Join(input, ".github", "workflows")
			if _, err := os.Stat(root); os.IsNotExist(err) {
				continue
			}
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if path != root && recursive && map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true}[entry.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 && IsYAML(path) {
				if recursive && ignored(path, root) {
					return nil
				}
				return add(path)
			}
			if !entry.Type().IsRegular() || !IsYAML(path) {
				return nil
			}
			if recursive && ignored(path, root) {
				return nil
			}
			return add(path)
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

func ignored(path, root string) bool {
	gitRoot := findGitRoot(root)
	boundary := root
	if gitRoot != "" {
		boundary = gitRoot
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		patterns, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err == nil {
			rel, err := filepath.Rel(dir, path)
			if err == nil && matchesIgnore(string(patterns), filepath.ToSlash(rel)) {
				return true
			}
		}
		if dir == boundary || dir == filepath.Dir(dir) {
			return false
		}
	}
}

func findGitRoot(start string) string {
	for dir := start; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir
		}
		if dir == filepath.Dir(dir) {
			return ""
		}
	}
}

func matchesIgnore(contents, path string) bool {
	ignored := false
	for _, raw := range strings.Split(contents, "\n") {
		pattern := strings.TrimSpace(raw)
		if pattern == "" || strings.HasPrefix(pattern, "#") {
			continue
		}
		negated := strings.HasPrefix(pattern, "!")
		if negated {
			pattern = strings.TrimPrefix(pattern, "!")
		}
		pattern = strings.TrimPrefix(pattern, "/")
		if strings.HasSuffix(pattern, "/") {
			pattern += "**"
		}
		if glob.Glob(pattern, path) || (!strings.Contains(pattern, "/") && glob.Glob(pattern, filepath.Base(path))) {
			ignored = !negated
		}
	}
	return ignored
}

func CheckText(data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("不是 UTF-8 文本")
	}
	hasCRLF := bytes.Contains(data, []byte("\r\n"))
	if bytes.Contains(data, []byte("\r")) && !hasCRLF {
		return fmt.Errorf("混合或不支持的换行符")
	}
	if !hasCRLF {
		return nil
	}
	withoutCRLF := bytes.ReplaceAll(data, []byte("\r\n"), nil)
	if bytes.Contains(withoutCRLF, []byte("\n")) || bytes.Contains(withoutCRLF, []byte("\r")) {
		return fmt.Errorf("混合或不支持的换行符")
	}
	return nil
}

func AtomicReplace(path string, original, updated []byte) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(resolved)
	if err != nil {
		return err
	}
	if sha256.Sum256(current) != sha256.Sum256(original) {
		return fmt.Errorf("文件在读取后已变更")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(resolved), ".actions-updater-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(info.Mode()); err == nil {
		_, err = tmp.Write(updated)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, resolved)
}

func IsSymlink(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", false
	}
	target, err := os.Readlink(path)
	return target, err == nil
}

func HasBOM(data []byte) bool { return bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) }
func Newline(data []byte) string {
	if strings.Contains(string(data), "\r\n") {
		return "\r\n"
	}
	return "\n"
}
