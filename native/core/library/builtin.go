package library

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltand/kitter/native/core/effective"
)

// builtin holds the embedded copy of resources/skills/kitter, kept in sync
// with the repository root by scripts/sync_builtin.sh and guarded by
// builtin_sync_test.go.
//
//go:embed builtin/kitter
var builtinFS embed.FS

// EnsureBuiltinSkill is ensure_builtin_skill: write the embedded Kitter
// skill into the library, refreshing files that differ.
func EnsureBuiltinSkill(libraryDir string) error {
	root := filepath.Join(libraryDir, KitterSkillStorage)
	for _, rel := range []string{
		"SKILL.md",
		filepath.Join("agents", "openai.yaml"),
		filepath.Join("references", "install-cli.md"),
	} {
		content, err := builtinFS.ReadFile(filepath.ToSlash(filepath.Join("builtin/kitter", rel)))
		if err != nil {
			return err
		}
		target := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if existing, err := os.ReadFile(target); err != nil || string(existing) != string(content) {
			if err := os.WriteFile(target, content, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

const disableModelInvocation = "disable-model-invocation"

// applyDisableModelInvocation is library::apply_disable_model_invocation.
func applyDisableModelInvocation(skillMD string, enabled bool) error {
	content, err := os.ReadFile(skillMD)
	if err != nil {
		return fmt.Errorf("读取技能文件失败：%s: %w", skillMD, err)
	}
	updated := setDisableModelInvocationMarkdown(string(content), enabled)
	if updated != string(content) {
		if err := os.WriteFile(skillMD, []byte(updated), 0o644); err != nil {
			return fmt.Errorf("写入技能文件失败：%s: %w", skillMD, err)
		}
	}
	return nil
}

// setDisableModelInvocationMarkdown is
// library::set_disable_model_invocation_markdown.
func setDisableModelInvocationMarkdown(content string, enabled bool) string {
	if enabled && contentDisablesModelInvocation(content) {
		return content
	}
	start, end, ok := frontmatterBodySpan(content)
	if !ok {
		if !enabled {
			return content
		}
		newline := markdownNewline(content)
		return fmt.Sprintf("---%s%s: true%s---%s%s", newline, disableModelInvocation, newline, newline, content)
	}
	body := content[start:end]
	newline := "\n"
	if strings.Contains(body, "\r\n") || strings.Contains(content[:start], "\r\n") {
		newline = "\r\n"
	} else {
		newline = markdownNewline(content)
	}
	lines := rustLines(body)
	found := -1
	for i, line := range lines {
		if yamlLineKey(line) == disableModelInvocation {
			found = i
			break
		}
	}
	if found >= 0 {
		if enabled {
			var indent strings.Builder
			for _, ch := range lines[found] {
				if ch == ' ' || ch == '\t' {
					indent.WriteRune(ch)
				} else {
					break
				}
			}
			lines[found] = indent.String() + disableModelInvocation + ": true"
		} else {
			lines = append(lines[:found], lines[found+1:]...)
		}
	} else if enabled {
		insertAt := len(lines)
		for insertAt > 0 && strings.TrimSpace(lines[insertAt-1]) == "" {
			insertAt--
		}
		lines = append(lines[:insertAt], append([]string{disableModelInvocation + ": true"}, lines[insertAt:]...)...)
	} else {
		return content
	}
	newBody := strings.Join(lines, newline)
	closing := content[end:]
	glue := ""
	if newBody != "" && !strings.HasPrefix(closing, "\n") && !strings.HasPrefix(closing, "\r\n") {
		glue = newline
	}
	return content[:start] + newBody + glue + closing
}

// contentDisablesModelInvocation delegates to the effective package.
var contentDisablesModelInvocation = effective.ContentDisablesModelInvocation

// frontmatterBodySpan is library::frontmatter_body_span.
func frontmatterBodySpan(content string) (int, int, bool) {
	var prefixLen int
	if strings.HasPrefix(content, "---\r\n") {
		prefixLen = 5
	} else if strings.HasPrefix(content, "---\n") {
		prefixLen = 4
	} else {
		return 0, 0, false
	}
	rest := content[prefixLen:]
	var ends []int
	if strings.HasPrefix(rest, "---\n") || strings.HasPrefix(rest, "---\r\n") || rest == "---" {
		ends = append(ends, 0)
	}
	if at := strings.Index(rest, "\n---\n"); at >= 0 {
		ends = append(ends, at)
	}
	if at := strings.Index(rest, "\r\n---\r\n"); at >= 0 {
		ends = append(ends, at)
	}
	if strings.HasSuffix(rest, "\n---") {
		ends = append(ends, len(rest)-4)
	}
	if strings.HasSuffix(rest, "\r\n---") {
		ends = append(ends, len(rest)-5)
	}
	if len(ends) == 0 {
		return 0, 0, false
	}
	end := ends[0]
	for _, e := range ends[1:] {
		if e < end {
			end = e
		}
	}
	return prefixLen, prefixLen + end, true
}

// yamlLineKey is library::yaml_line_key.
func yamlLineKey(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}
	key, _, ok := strings.Cut(trimmed, ":")
	if !ok {
		return ""
	}
	return strings.Trim(strings.TrimSpace(key), `"'`)
}

func markdownNewline(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// rustLines splits like Rust's str::lines(): a trailing newline does not
// produce an empty last element, and each line loses a trailing '\r'.
func rustLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	return lines
}

// copyTree is library::copy_tree: recursive copy, skipping .git.
func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if strings.Split(rel, string(os.PathSeparator))[0] == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		target := filepath.Join(destination, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type().IsRegular() {
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, info.Mode().Perm())
		}
		return nil
	})
}
