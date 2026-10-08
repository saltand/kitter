// scanner.go ports src/effective_skills/scanner.rs: the scan profiles
// and SkillRoot-driven filesystem walks, including the Pi profile's
// gitignore-aware traversal.
package effective

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScanProfile mirrors scanner::ScanProfile.
type ScanProfile struct {
	Kind           scanProfileKind
	MaxDepth       int // Recursive
	MaxDirectories int
	MaxEntries     int
}

type scanProfileKind int

const (
	profileDirectChildren scanProfileKind = iota
	profileRecursive
	profilePiIgnored
)

// DirectChildrenProfile is ScanProfile::DirectChildren.
func DirectChildrenProfile() ScanProfile {
	return ScanProfile{Kind: profileDirectChildren}
}

// RecursiveProfile is ScanProfile::Recursive.
func RecursiveProfile(maxDepth, maxDirectories, maxEntries int) ScanProfile {
	return ScanProfile{
		Kind:           profileRecursive,
		MaxDepth:       maxDepth,
		MaxDirectories: maxDirectories,
		MaxEntries:     maxEntries,
	}
}

// PiIgnoredProfile is ScanProfile::PiIgnored.
func PiIgnoredProfile() ScanProfile {
	return ScanProfile{Kind: profilePiIgnored}
}

// scan is scanner::scan.
func scan(root *SkillRoot, profile ScanProfile) []string {
	if root.ExactSkillFile != "" {
		if isFile(root.ExactSkillFile) {
			return []string{root.ExactSkillFile}
		}
		return nil
	}
	if !isDir(root.Path) {
		return nil
	}
	if root.FlatMarkdownOnly {
		return flatMarkdown(root)
	}
	if root.DirectChildrenOnly {
		return directChildren(root)
	}
	var files []string
	switch profile.Kind {
	case profileDirectChildren:
		files = directChildren(root)
	case profileRecursive:
		files = recursive(root, profile.MaxDepth, profile.MaxDirectories, profile.MaxEntries)
	case profilePiIgnored:
		files = piIgnored(root)
	}
	sort.Strings(files)
	return dedup(files)
}

func dedup(files []string) []string {
	out := files[:0]
	prev := ""
	for i, f := range files {
		if i == 0 || f != prev {
			out = append(out, f)
		}
		prev = f
	}
	return out
}

// flatMarkdown is scanner::flat_markdown.
func flatMarkdown(root *SkillRoot) []string {
	entries, err := os.ReadDir(root.Path)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		path := filepath.Join(root.Path, entry.Name())
		if entry.Type().IsRegular() && filepath.Ext(path) == ".md" {
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files
}

// directChildren is scanner::direct_children.
func directChildren(root *SkillRoot) []string {
	var result []string
	if manifest := filepath.Join(root.Path, "SKILL.md"); isFile(manifest) {
		result = append(result, manifest)
	}
	entries, err := os.ReadDir(root.Path)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		path := filepath.Join(root.Path, entry.Name())
		if entry.IsDir() {
			if manifest := filepath.Join(path, "SKILL.md"); isFile(manifest) {
				result = append(result, manifest)
			}
		} else if root.IncludeRootMarkdown && filepath.Ext(path) == ".md" {
			result = append(result, path)
		}
	}
	return result
}

// recursive is scanner::recursive: iterative DFS honoring hidden-dir and
// node_modules skips, symlinked-directory following gated on the root
// flag, and the max_directories/max_entries budgets.
func recursive(root *SkillRoot, maxDepth, maxDirectories, maxEntries int) []string {
	var result []string
	stack := []dirDepth{{root.Path, 0}}
	visited := map[string]bool{}
	directories := 0
	entriesSeen := 0
	for len(stack) > 0 {
		if directories >= maxDirectories || entriesSeen >= maxEntries {
			break
		}
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		canonical := canonicalize(top.dir)
		if visited[canonical] {
			continue
		}
		visited[canonical] = true
		directories++
		manifest := filepath.Join(top.dir, "SKILL.md")
		if isFile(manifest) {
			result = append(result, manifest)
			continue
		}
		if top.depth >= maxDepth {
			continue
		}
		entries, err := os.ReadDir(top.dir)
		if err != nil {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			entriesSeen++
			if entriesSeen > maxEntries {
				break
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				continue
			}
			path := filepath.Join(top.dir, name)
			if top.dir == root.Path && root.IncludeRootMarkdown &&
				entry.Type().IsRegular() && filepath.Ext(path) == ".md" {
				result = append(result, path)
			} else if isDirFollow(path, entry, root.FollowDirectorySymlinks) {
				stack = append(stack, dirDepth{path, top.depth + 1})
			}
		}
	}
	return result
}

type dirDepth struct {
	dir   string
	depth int
}

// isDirFollow reports whether entry is (or links to) a directory the
// walk should descend into.
func isDirFollow(path string, entry os.DirEntry, followSymlinks bool) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink != 0 && followSymlinks {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// piIgnored is scanner::pi_ignored. The Rust version uses
// ignore::WalkBuilder with:
//   hidden(false)         hidden entries are NOT skipped
//   git_ignore(true)      .gitignore honored (inside a git repo only,
//                         require_git defaults to true)
//   git_global(false)     no global gitignore
//   git_exclude(true)     .git/info/exclude honored
//   ignore(true)          .ignore files honored
//   parents(true)         ignore files in ancestors of root apply
//   follow_links(true)    symlinked directories are descended
//
// Go implements that subset directly.
func piIgnored(root *SkillRoot) []string {
	rootManifest := filepath.Join(root.Path, "SKILL.md")
	if isFile(rootManifest) {
		return []string{rootManifest}
	}
	var result []string
	ignores := loadIgnoreStack(root.Path)
	filepath.Walk(root.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path == root.Path {
			return nil
		}
		name := info.Name()
		if info.IsDir() {
			if ignores.match(path, true) {
				return filepath.SkipDir
			}
			return nil
		}
		// For files inside symlinked dirs Walk may not report them as
		// regular; accept non-dir non-other entries as files.
		if !info.Mode().IsRegular() {
			return nil
		}
		if ignores.match(path, false) {
			return nil
		}
		if name == "SKILL.md" ||
			(root.IncludeRootMarkdown && filepath.Dir(path) == root.Path &&
				filepath.Ext(path) == ".md") {
			result = append(result, path)
		}
		return nil
	})
	return result
}

// ignoreStack is a minimal .ignore/.gitignore matcher for piIgnored.
// It covers the patterns Rust's ignore crate reads here: .ignore and
// .gitignore files under the root (parents(true) also loads .gitignore
// and .ignore from ancestors of the root), and .git/info/exclude of the
// enclosing repository.
type ignoreStack struct {
	root  string // scan root, forward-slash absolute
	rules []ignoreRule
}

type ignoreRule struct {
	dirOnly    bool
	negate     bool
	anchored   bool
	base       string   // directory the rule's file lives in
	components []string // pattern split on /
}

// loadIgnoreStack loads .ignore/.gitignore under root, plus ancestors'
// files and .git/info/exclude (parents(true) + git_exclude(true) +
// ignore(true), no global).
func loadIgnoreStack(root string) *ignoreStack {
	s := &ignoreStack{root: filepath.ToSlash(root)}
	// Ancestors: from the filesystem root down to the scan root — later
	// files override earlier ones, matching ignore's precedence where
	// deeper/closer files win when checked last-to-first.
	var dirs []string
	for d := filepath.Clean(root); ; {
		dirs = append([]string{d}, dirs...)
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	for _, dir := range dirs {
		s.loadFile(filepath.Join(dir, ".ignore"))
		// git_ignore/git_exclude honor the require_git default: only
		// apply .gitignore/.git/info/exclude inside a repository.
		if inGitRepo(dir) {
			s.loadFile(filepath.Join(dir, ".gitignore"))
			if isDir(filepath.Join(dir, ".git")) {
				s.loadFile(filepath.Join(dir, ".git", "info", "exclude"))
			}
		}
	}
	// Per-directory files inside the root are loaded via loadNested.
	s.loadNested(root)
	return s
}

// inGitRepo reports whether dir sits inside a git repository (some
// ancestor contains a .git entry).
func inGitRepo(dir string) bool {
	for d := filepath.Clean(dir); ; {
		target := filepath.Join(d, ".git")
		if _, err := os.Lstat(target); err == nil {
			return true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false
		}
		d = parent
	}
}

// loadNested loads .ignore/.gitignore files below root.
func (s *ignoreStack) loadNested(root string) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base == ".ignore" {
			s.loadFile(path)
		} else if base == ".gitignore" && inGitRepo(filepath.Dir(path)) {
			s.loadFile(path)
		}
		return nil
	})
}

func (s *ignoreStack) loadFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	base := filepath.ToSlash(filepath.Dir(path))
	for _, line := range strings.Split(string(data), "\n") {
		// gitignore semantics: trim trailing space (escaped space
		// handling is out of scope — the ignore crate handles it but no
		// test needs it), skip comments and blanks.
		line = strings.TrimRight(line, " ")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule := ignoreRule{base: base}
		if strings.HasPrefix(line, "!") {
			rule.negate = true
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			rule.dirOnly = true
			line = line[:len(line)-1]
		}
		if strings.Contains(line, "/") {
			rule.anchored = true
			line = strings.TrimPrefix(line, "/")
		}
		if line == "" {
			continue
		}
		rule.components = strings.Split(line, "/")
		s.rules = append(s.rules, rule)
	}
}

// match reports whether path (a file or dir inside root) is ignored.
// Later rules win over earlier ones (gitignore precedence).
func (s *ignoreStack) match(path string, isDir bool) bool {
	rel := filepath.ToSlash(path)
	ignored := false
	for _, r := range s.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.matchPath(rel, isDir) {
			ignored = !r.negate
		}
	}
	return ignored
}

// matchPath checks a rule against the path, honoring anchor base and
// directory-prefix matching (a pattern matches anything under it).
func (r *ignoreRule) matchPath(path string, isDir bool) bool {
	var rel string
	if r.anchored {
		// Pattern is relative to the rule's directory.
		if !strings.HasPrefix(path, r.base+"/") && path != r.base {
			return false
		}
		rel = strings.TrimPrefix(path, r.base+"/")
	} else {
		// Unanchored: match against any suffix of the path — the git
		// semantics match the pattern's first component at any level.
		// We test each suffix.
		parts := strings.Split(path, "/")
		for i := 0; i < len(parts); i++ {
			if globMatchComponents(r.components, parts[i:]) {
				return true
			}
			// Directory-prefix: a dir pattern matches descendants too,
			// which we handle by matching a prefix of the remaining
			// parts against the pattern's components.
			if len(r.components) <= len(parts)-i {
				if globMatchComponents(r.components, parts[i:i+len(r.components)]) {
					return true
				}
			}
		}
		return false
	}
	parts := strings.Split(rel, "/")
	if len(r.components) > len(parts) {
		return false
	}
	return globMatchComponents(r.components, parts[:len(r.components)])
}

// globMatchComponents matches pattern components against path
// components, supporting `*` (within a component), `?`, `[...]` and
// `**` (any number of components).
func globMatchComponents(pattern, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}
	if pattern[0] == "**" {
		for skip := 0; skip <= len(path); skip++ {
			if globMatchComponents(pattern[1:], path[skip:]) {
				return true
			}
		}
		return false
	}
	if len(path) == 0 {
		return false
	}
	ok, err := filepath.Match(pattern[0], path[0])
	if err != nil || !ok {
		return false
	}
	return globMatchComponents(pattern[1:], path[1:])
}

// canonicalize is fs::canonicalize fallback to the input.
func canonicalize(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
