// Package adoption ports src/adoption.rs: one-shot discovery and
// adoption of existing skill installations. Scanning never installs
// packages or changes sources. Cancellation uses context.Context in
// place of Rust's AtomicBool.
//
// External git invocations (remote.origin.url lookups) run through the
// injectable GitRunner so tests never touch the network — see runner.go
// in core/source, which this package reuses via its own thin seam.
package adoption

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/saltand/kitter/native/core/agents"
	"github.com/saltand/kitter/native/core/fslink"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/project"
	"github.com/saltand/kitter/native/core/skillfile"
)

// AdoptionCandidate is adoption::AdoptionCandidate.
type AdoptionCandidate struct {
	ID              string
	Name            string
	Description     string
	Source          string
	Origin          model.SkillOrigin
	References      []model.SkillReference
	Issue           string
	ExistingStorage string
	markerHash      uint64
}

// FixtureCandidate builds a candidate for tests (AdoptionCandidate::fixture).
func FixtureCandidate(id string, origin model.SkillOrigin) *AdoptionCandidate {
	return &AdoptionCandidate{
		ID:     id,
		Name:   "same-name",
		Source: id,
		Origin: origin,
	}
}

// Identity is AdoptionCandidate::identity.
func (c *AdoptionCandidate) Identity() string {
	return c.Origin.IdentityKey(c.Name)
}

// Record is AdoptionCandidate::record.
func (c *AdoptionCandidate) Record() model.SkillRecord {
	return model.SkillRecord{
		Name:        c.Name,
		Description: c.Description,
		Origin:      c.Origin,
	}
}

// Verify is AdoptionCandidate::verify: re-hash SKILL.md before adopt.
func (c *AdoptionCandidate) Verify() error {
	hash, err := markerHash(c.Source)
	if err != nil || hash != c.markerHash {
		return fmt.Errorf("技能已变化，请重新扫描：%s", c.Source)
	}
	return nil
}

// AdoptionScan is adoption::AdoptionScan.
type AdoptionScan struct {
	Candidates []*AdoptionCandidate
	identities map[string][]int
	byID       map[string]int
	selectable map[string]bool
	// Skipped is kept for diagnostics, not presented as a noisy counter.
	Skipped []string
}

// NewAdoptionScan is AdoptionScan::new.
func NewAdoptionScan(candidates []*AdoptionCandidate, skipped []string) *AdoptionScan {
	scan := &AdoptionScan{Candidates: candidates, Skipped: skipped}
	scan.reindex()
	return scan
}

func (s *AdoptionScan) reindex() {
	s.identities = map[string][]int{}
	s.byID = map[string]int{}
	for i, c := range s.Candidates {
		s.identities[c.Identity()] = append(s.identities[c.Identity()], i)
		s.byID[c.ID] = i
	}
	s.selectable = map[string]bool{}
	for _, c := range s.Candidates {
		if c.Issue == "" && !s.HasConflict(c.Identity()) {
			s.selectable[c.ID] = true
		}
	}
}

// Retain is AdoptionScan::retain.
func (s *AdoptionScan) Retain(keep func(*AdoptionCandidate) bool) {
	var kept []*AdoptionCandidate
	for _, c := range s.Candidates {
		if keep(c) {
			kept = append(kept, c)
		}
	}
	s.Candidates = kept
	s.reindex()
}

// HasConflict is AdoptionScan::has_conflict.
func (s *AdoptionScan) HasConflict(identity string) bool {
	return len(s.identities[identity]) > 1
}

// SelectableIDs is AdoptionScan::selectable_ids.
func (s *AdoptionScan) SelectableIDs() map[string]bool { return s.selectable }

// ContainsID is AdoptionScan::contains_id.
func (s *AdoptionScan) ContainsID(id string) bool {
	_, ok := s.byID[id]
	return ok
}

// DefaultSelection is AdoptionScan::default_selection.
func (s *AdoptionScan) DefaultSelection() map[string]bool {
	out := map[string]bool{}
	for id := range s.selectable {
		out[id] = true
	}
	return out
}

// Variants is AdoptionScan::variants.
func (s *AdoptionScan) Variants(candidate *AdoptionCandidate) []*AdoptionCandidate {
	var out []*AdoptionCandidate
	for _, i := range s.identities[candidate.Identity()] {
		out = append(out, s.Candidates[i])
	}
	return out
}

// Select is AdoptionScan::select.
func (s *AdoptionScan) Select(selected map[string]bool, id string) {
	index, ok := s.byID[id]
	if !ok {
		return
	}
	candidate := s.Candidates[index]
	if candidate.Issue != "" || selected[id] {
		delete(selected, id)
		return
	}
	for _, sibling := range s.Variants(candidate) {
		delete(selected, sibling.ID)
	}
	selected[id] = true
}

// ReferencesFor is AdoptionScan::references_for.
func (s *AdoptionScan) ReferencesFor(candidate *AdoptionCandidate) []model.SkillReference {
	seen := map[string]bool{}
	var out []model.SkillReference
	for _, variant := range s.Variants(candidate) {
		for _, r := range variant.References {
			if seen[project.InstallationKey(r.Path)] {
				continue
			}
			seen[project.InstallationKey(r.Path)] = true
			out = append(out, r)
		}
	}
	return out
}

func markerHash(path string) (uint64, error) {
	content, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
	if err != nil {
		return 0, err
	}
	h := fnv.New64a()
	h.Write(content)
	return h.Sum64(), nil
}

func readMarker(path string) (string, string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(string(text), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", errors.New("缺少 frontmatter")
	}
	var header []string
	closed := false
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			closed = true
			break
		}
		header = append(header, line)
	}
	if !closed {
		return "", "", errors.New("frontmatter 未闭合")
	}
	name, description, err := skillfile.ReadFrontmatter(path)
	if err != nil {
		return "", "", err
	}
	if err := skillfile.ValidateName(name); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(name) == "" {
		return "", "", errors.New("技能名称为空")
	}
	return name, description, nil
}

// Excluded is adoption::excluded: privacy exclusions are applied before
// opening a directory, including symlink targets.
func Excluded(path, home, libraryDir string) bool {
	if strings.HasPrefix(path, libraryDir) {
		return true
	}
	if privacyExcluded(path, home) {
		return true
	}
	for _, suffix := range []string{
		".claude/plugins",
		".codex/plugins",
		".codex/sessions",
		".codex/memories",
		".codex/skills/.system",
		".npm",
		".cache",
		".rustup",
		".cargo/registry",
		".cargo/git",
		".pnpm-store",
		".bun/install/cache",
	} {
		if strings.HasPrefix(path, filepath.Join(home, filepath.FromSlash(suffix))) {
			return true
		}
	}
	for _, component := range strings.Split(path, string(os.PathSeparator)) {
		switch component {
		case ".git", ".venv", "venv", "__pycache__", ".next", ".nuxt",
			"node_modules", "target", "build", "dist", "DerivedData",
			"Caches", ".Trash":
			return true
		}
	}
	return false
}

func privacyExcluded(path, home string) bool {
	for _, name := range []string{"Pictures", "Music", "Movies", "Library", ".Trash", "Applications"} {
		if strings.HasPrefix(path, filepath.Join(home, name)) {
			return true
		}
	}
	for _, component := range strings.Split(path, string(os.PathSeparator)) {
		lower := strings.ToLower(component)
		for _, ext := range []string{".photoslibrary", ".photolibrary", ".musiclibrary", ".imovielibrary", ".app"} {
			if strings.HasSuffix(lower, ext) {
				return true
			}
		}
	}
	return false
}

func readJSON(path string) map[string]any {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal(bytes, &value); err != nil {
		return nil
	}
	return value
}

func installedLocation(path string) bool {
	return filepath.Base(filepath.Dir(path)) == "skills"
}

func npxOrigin(path, name, home string, cache map[string]any) *model.SkillOrigin {
	// Do not associate a random local folder with a same-named global
	// lock entry.
	if !installedLocation(path) {
		return nil
	}
	installName := filepath.Base(path)
	for root := filepath.Dir(path); ; root = filepath.Dir(root) {
		for _, lockPath := range []string{
			filepath.Join(root, "skills-lock.json"),
			filepath.Join(root, ".agents", ".skill-lock.json"),
		} {
			value, cached := cache[lockPath]
			if !cached {
				value = readJSON(lockPath)
				cache[lockPath] = value
			}
			skills, _ := value.(map[string]any)["skills"].(map[string]any)
			var entry map[string]any
			if skills != nil {
				if e, ok := skills[installName].(map[string]any); ok {
					entry = e
				} else if e, ok := skills[name].(map[string]any); ok {
					entry = e
				}
			}
			if entry == nil {
				continue
			}
			kind, _ := entry["sourceType"].(string)
			if kind == "local" || kind == "node_modules" {
				continue
			}
			repository, _ := entry["source"].(string)
			if repository == "" {
				repository, _ = entry["sourceUrl"].(string)
			}
			if repository == "" {
				continue
			}
			var hash *string
			if h, ok := entry["skillFolderHash"].(string); ok {
				hash = &h
			} else if h, ok := entry["computedHash"].(string); ok {
				hash = &h
			}
			origin := model.OriginNpx(repository, name, hash)
			return &origin
		}
		if root == home {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
	}
	return nil
}

// GitRunner resolves a repository's remote.origin.url; injectable so
// tests never spawn git (the real implementation matches
// Command::new("git") -C root config --get remote.origin.url).
var GitRunner = func(ctx context.Context, root string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "config", "--get", "remote.origin.url")
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitOrigin(ctx context.Context, path string) *model.SkillOrigin {
	for root := path; ; root = filepath.Dir(root) {
		if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			// The consuming project's remote is not the upstream of
			// installed copies.
			for _, c := range strings.Split(relative, string(os.PathSeparator)) {
				switch c {
				case ".agents", ".claude", ".codex", ".cursor", ".pi":
					return nil
				}
			}
			repository, err := GitRunner(ctx, root)
			if err != nil || repository == "" {
				return nil
			}
			var subdir *string
			if relative != "." {
				subdir = &relative
			}
			origin := model.OriginGit(repository, subdir)
			return &origin
		}
		parent := filepath.Dir(root)
		if parent == root {
			return nil
		}
	}
}

// extraDirectRoots is EXTRA_DIRECT_ROOTS: only direct install roots
// count; plugin registries and extension manifests are deliberately not
// read.
var extraDirectRoots = []string{
	".pi/agent/skills",
	".config/opencode/skills",
	".openclaw/skills",
	".hermes/skills",
	".copilot/skills",
	".gemini/config/skills",
	".config/agents/skills",
	".config/amp/skills",
}

func skillRoot(path string) bool {
	for _, target := range agents.ProjectInstallTargets {
		if strings.HasSuffix(path, filepath.FromSlash(agents.TargetDirectory(target))) {
			return true
		}
	}
	for _, suffix := range extraDirectRoots {
		if strings.HasSuffix(path, filepath.FromSlash(suffix)) {
			return true
		}
	}
	return false
}

func agentDirectory(path string) bool {
	for _, target := range agents.ProjectInstallTargets {
		parent := filepath.Dir(filepath.FromSlash(agents.TargetDirectory(target)))
		if strings.HasSuffix(path, parent) {
			return true
		}
	}
	for _, root := range extraDirectRoots {
		parent := filepath.Dir(filepath.FromSlash(root))
		if strings.HasSuffix(path, parent) {
			return true
		}
	}
	return false
}

func pluginPath(path string) bool {
	// Applies to project-local caches as well as global ones, including
	// link targets.
	components := strings.Split(path, string(os.PathSeparator))
	for i := 0; i+1 < len(components); i++ {
		switch components[i] {
		case ".claude", ".codex", ".cursor", ".opencode", ".pi":
			switch components[i+1] {
			case "plugins", "extensions", "packages":
				return true
			}
		}
	}
	for i := 0; i+2 < len(components); i++ {
		if (components[i] == ".pi" && components[i+1] == "agent" ||
			components[i] == ".config" && components[i+1] == "opencode") &&
			(components[i+2] == "plugins" || components[i+2] == "extensions" || components[i+2] == "packages") {
			return true
		}
	}
	return false
}

type scanner struct {
	home          string
	library       string
	ctx           context.Context
	candidates    map[string]*AdoptionCandidate
	order         []string // BTreeMap order
	deferredLinks []model.SkillReference
	locks         map[string]any
	skipped       []string
	visited       map[string]bool
}

func (s *scanner) cancelled() bool {
	return s.ctx.Err() != nil
}

func (s *scanner) add(entry string) {
	if s.cancelled() {
		return
	}
	source, err := filepath.EvalSymlinks(entry)
	if err != nil {
		return
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return
	}
	if pluginPath(source) {
		return
	}
	link, _ := fslink.Inspect(entry)
	if strings.HasPrefix(source, s.library) {
		if link != nil {
			target := link.Target
			s.deferredLinks = append(s.deferredLinks, model.SkillReference{
				Path:           entry,
				Source:         source,
				Kind:           model.ReferenceLink,
				OriginalTarget: &target,
			})
		}
		return
	}
	if Excluded(source, s.home, s.library) {
		return
	}
	name, description, err := readMarker(filepath.Join(source, "SKILL.md"))
	if err != nil {
		return
	}
	hash, err := markerHash(source)
	if err != nil {
		return
	}
	origin := npxOrigin(entry, name, s.home, s.locks)
	if origin == nil {
		origin = npxOrigin(source, name, s.home, s.locks)
	}
	if origin == nil {
		o := model.OriginLocal(source, nil)
		origin = &o
	}
	var originalTarget *string
	var kind model.ReferenceKind
	if link != nil {
		kind = model.ReferenceLink
		originalTarget = &link.Target
	} else if project.InstallationKey(entry) != entry {
		kind = model.ReferenceAlias
	} else {
		kind = model.ReferenceDirect
	}
	reference := model.SkillReference{
		Path:           entry,
		Source:         source,
		Kind:           kind,
		OriginalTarget: originalTarget,
	}
	candidate, ok := s.candidates[source]
	if !ok {
		candidate = &AdoptionCandidate{
			ID:          source,
			Name:        name,
			Description: description,
			Source:      source,
			Origin:      *origin,
			markerHash:  hash,
		}
		s.candidates[source] = candidate
		s.order = append(s.order, source)
	}
	if origin.Type != "local" {
		if candidate.Origin.Type == "local" {
			candidate.Origin = *origin
		} else if candidate.Origin.IdentityKey(name) != origin.IdentityKey(name) {
			candidate.Issue = "来源记录不一致"
		}
	}
	duplicate := false
	for _, r := range candidate.References {
		if r.Path == reference.Path {
			duplicate = true
			break
		}
	}
	if !duplicate {
		candidate.References = append(candidate.References, reference)
	}
}

func (s *scanner) directRoot(root string) {
	target, err := filepath.EvalSymlinks(root)
	if err != nil {
		return
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return
	}
	if Excluded(target, s.home, s.library) || pluginPath(target) {
		return
	}
	// Deduplicate logical roots, not physical ones: root aliases are
	// references too.
	if !s.visited[root] {
		s.visited[root] = true
	} else {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		s.skipped = append(s.skipped, root)
		return
	}
	for _, entry := range entries {
		if s.cancelled() {
			return
		}
		s.add(filepath.Join(root, entry.Name()))
	}
}

func (s *scanner) walk(root string) {
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if s.cancelled() {
			return filepath.SkipAll
		}
		if err != nil {
			s.skipped = append(s.skipped, path)
			return nil
		}
		if Excluded(path, s.home, s.library) || pluginPath(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if skillRoot(path) {
			s.directRoot(path)
			if d.IsDir() {
				return filepath.SkipDir
			}
		} else if agentDirectory(path) {
			s.directRoot(filepath.Join(path, "skills"))
			if strings.HasSuffix(path, ".pi") {
				s.directRoot(filepath.Join(path, "agent", "skills"))
			}
			if d.IsDir() {
				return filepath.SkipDir
			}
		}
		return nil
	})
}

// Scan is adoption::scan.
func Scan(ctx context.Context, home string, projects []string, libraryDir string, managed []model.SkillSummary) (*AdoptionScan, error) {
	roots := append([]string{home}, projects...)
	return ScanRoots(ctx, home, roots, libraryDir, managed)
}

// ScanRoots is adoption::scan_roots.
func ScanRoots(ctx context.Context, home string, roots []string, libraryDir string, managed []model.SkillSummary) (*AdoptionScan, error) {
	homeAbs, err := filepath.Abs(home)
	if err != nil {
		return nil, errors.New("找不到用户目录")
	}
	if resolved, err := filepath.EvalSymlinks(homeAbs); err == nil {
		homeAbs = resolved
	}
	canonicalLibrary, err := filepath.EvalSymlinks(libraryDir)
	if err != nil {
		canonicalLibrary = libraryDir
	}
	s := &scanner{
		home:       homeAbs,
		library:    canonicalLibrary,
		ctx:        ctx,
		candidates: map[string]*AdoptionCandidate{},
		locks:      map[string]any{},
		visited:    map[string]bool{},
	}
	for _, root := range roots {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			s.walk(root)
		}
	}
	if s.cancelled() {
		return nil, errors.New("已取消扫描")
	}
	for _, candidate := range s.candidates {
		if s.cancelled() {
			return nil, errors.New("已取消扫描")
		}
		if candidate.Origin.Type == "local" {
			if origin := gitOrigin(ctx, candidate.Source); origin != nil {
				candidate.Origin = *origin
			}
		}
	}
	// Existing library identities are reusable; do not collapse
	// same-named different sources.
	for _, skill := range managed {
		source, err := filepath.EvalSymlinks(skill.Path)
		if err != nil {
			continue
		}
		source, err = filepath.Abs(source)
		if err != nil {
			continue
		}
		if candidate, ok := s.candidates[source]; ok {
			candidate.Origin = skill.Record.Origin
			candidate.ExistingStorage = skill.Record.StorageName
		} else {
			has := false
			for _, c := range s.candidates {
				if c.Identity() == skill.Record.IdentityKey() {
					has = true
					break
				}
			}
			if has {
				if hash, err := markerHash(source); err == nil {
					s.candidates[source] = &AdoptionCandidate{
						ID:              source,
						Name:            skill.Record.Name,
						Description:     skill.Record.Description,
						Source:          source,
						Origin:          skill.Record.Origin,
						ExistingStorage: skill.Record.StorageName,
						markerHash:      hash,
					}
					s.order = append(s.order, source)
				}
			}
		}
	}
	for _, reference := range s.deferredLinks {
		if candidate, ok := s.candidates[reference.Source]; ok {
			duplicate := false
			for _, r := range candidate.References {
				if r.Path == reference.Path {
					duplicate = true
					break
				}
			}
			if !duplicate {
				candidate.References = append(candidate.References, reference)
			}
		}
	}
	var candidates []*AdoptionCandidate
	sort.Strings(s.order)
	for _, source := range s.order {
		candidates = append(candidates, s.candidates[source])
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		ki := candidates[i].Origin.Source().Key()
		kj := candidates[j].Origin.Source().Key()
		if ki != kj {
			return ki < kj
		}
		if candidates[i].Name != candidates[j].Name {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].Source < candidates[j].Source
	})
	sort.Strings(s.skipped)
	s.skipped = dedup(s.skipped)
	return NewAdoptionScan(candidates, s.skipped), nil
}

func dedup(items []string) []string {
	var out []string
	for i, item := range items {
		if i == 0 || item != items[i-1] {
			out = append(out, item)
		}
	}
	return out
}

// ValidateReference is adoption::validate_reference: snapshot guards are
// checked again immediately before replacing any link.
func ValidateReference(reference *model.SkillReference) error {
	if reference.Kind != model.ReferenceLink {
		return nil
	}
	link, err := fslink.Inspect(reference.Path)
	if err != nil || link == nil {
		return errors.New("引用已变化，请重新扫描")
	}
	if reference.OriginalTarget == nil || link.Target != *reference.OriginalTarget {
		return fmt.Errorf("引用已变化，请重新扫描：%s", reference.Path)
	}
	resolved, err := filepath.EvalSymlinks(reference.Path)
	if err != nil || resolved != reference.Source {
		return fmt.Errorf("引用已变化，请重新扫描：%s", reference.Path)
	}
	return nil
}

// ReplaceLink is adoption::replace_link: stage the new link in the same
// directory, then rename it over the old path atomically.
func ReplaceLink(path, target string) error {
	parent := filepath.Dir(path)
	temp, err := os.MkdirTemp(parent, ".kitter-adopt-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	staged := filepath.Join(temp, "link")
	if err := fslink.Create(target, staged); err != nil {
		return err
	}
	if err := os.Rename(staged, path); err != nil {
		return fmt.Errorf("无法切换引用：%s: %w", path, err)
	}
	return nil
}

// RollbackLinks is adoption::rollback_links.
func RollbackLinks(changed []model.SkillReference) []string {
	var failures []string
	for i := len(changed) - 1; i >= 0; i-- {
		r := changed[i]
		if r.OriginalTarget == nil {
			continue
		}
		if err := ReplaceLink(r.Path, *r.OriginalTarget); err != nil {
			failures = append(failures, err.Error())
		}
	}
	return failures
}
