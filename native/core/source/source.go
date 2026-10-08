// Package source ports src/source.rs: scanning skill sources (local
// folders, the npx skills.sh CLI, Claude plugins), importing scanned
// skills into the library, and checking/applying upstream updates.
//
// External tools (npx, git, claude) are invoked through the injectable
// Runner so tests never touch the network. The Rust code shells out to
// the test binary via env::current_exe for its npx fixture; the Go port
// instead injects a fake Runner — see runner_test.go — because the Go
// CLI does not exist yet (M6), so there is no stable subcommand entry
// point to re-exec.
package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/saltand/kitter/native/core/config"
	"github.com/saltand/kitter/native/core/library"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/skillfile"
)

// ScannedSkill is source::ScannedSkill.
type ScannedSkill struct {
	Name        string
	Description string
	Path        string
}

type scanOrigin struct {
	kind       string // "npx" | "claude" | "local"
	repository string // npx
	plugin     string // claude
	root       string // local
	label      string // local
}

// SkillScan is source::SkillScan. temp keeps the npx download alive for
// import_selected's lock-hash read.
type SkillScan struct {
	origin scanOrigin
	skills []ScannedSkill
	temp   string // retained snapshot dir (npx only)
}

// ImportSummary is source::ImportSummary.
type ImportSummary struct {
	Added   int
	Skipped int
}

// Skills is SkillScan::skills.
func (s *SkillScan) Skills() []ScannedSkill { return s.skills }

// SourceLabel is SkillScan::source_label.
func (s *SkillScan) SourceLabel() string {
	switch s.origin.kind {
	case "npx":
		return s.origin.repository
	case "claude":
		return s.origin.plugin
	default:
		return s.origin.label
	}
}

// SourceKey is SkillScan::source_key.
func (s *SkillScan) SourceKey() string { return s.source().Key() }

// DefaultGroupName is SkillScan::default_group_name.
func (s *SkillScan) DefaultGroupName() string { return s.source().Label() }

func (s *SkillScan) source() model.SkillSource {
	switch s.origin.kind {
	case "npx":
		return model.SkillSource{Type: "npx", Repository: s.origin.repository}
	case "claude":
		return model.SkillSource{Type: "claude_marketplace", Plugin: s.origin.plugin}
	default:
		return model.SkillSource{Type: "local", Path: s.origin.root}
	}
}

// ImportSelected is SkillScan::import_selected.
func (s *SkillScan) ImportSelected(lib *library.SkillLibrary, selected map[string]bool, groupName string) (ImportSummary, error) {
	var summary ImportSummary
	if len(selected) == 0 {
		return summary, errors.New("请至少选择一个技能")
	}
	var discovered []string
	for _, skill := range s.skills {
		discovered = append(discovered, skill.Name)
	}
	// The scan already downloaded a complete snapshot. Import from that
	// snapshot only; update workspaces are prepared lazily by update flows.
	var scanHashes map[string]string
	if s.origin.kind == "npx" {
		if s.temp == "" {
			return summary, errors.New("Npx 扫描结果不可用，请重新扫描")
		}
		var err error
		scanHashes, err = NpxLockHashes(s.temp)
		if err != nil {
			return summary, err
		}
	}
	var addedSkills []string
	group := strings.TrimSpace(groupName)
	var groupID *string
	for _, skill := range s.skills {
		if !selected[skill.Name] {
			continue
		}
		var identity string
		switch s.origin.kind {
		case "npx":
			identity = model.OriginNpx(s.origin.repository, skill.Name, nil).IdentityKey(skill.Name)
		case "claude":
			identity = model.OriginClaudeMarketplace(s.origin.plugin, skill.Name).IdentityKey(skill.Name)
		default:
			identity = model.OriginLocal(skill.Path, &s.origin.root).IdentityKey(skill.Name)
		}
		if lib.ContainsIdentity(identity) {
			summary.Skipped++
			continue
		}
		var skillOrigin model.SkillOrigin
		switch s.origin.kind {
		case "npx":
			var hash *string
			if h, ok := scanHashes[skill.Name]; ok {
				delete(scanHashes, skill.Name)
				hash = &h
			}
			skillOrigin = model.OriginNpx(s.origin.repository, skill.Name, hash)
		case "claude":
			skillOrigin = model.OriginClaudeMarketplace(s.origin.plugin, skill.Name)
		default:
			skillOrigin = model.OriginLocal(skill.Path, &s.origin.root)
		}
		if groupID == nil && group != "" {
			id, err := lib.EnsureGroup(group)
			if err != nil {
				return summary, err
			}
			groupID = &id
		}
		if err := lib.Import(skill.Path, model.SkillRecord{
			Name:         skill.Name,
			Description:  skill.Description,
			Origin:       skillOrigin,
			GroupID:      groupID,
			KitterManual: false,
		}); err != nil {
			return summary, err
		}
		addedSkills = append(addedSkills, skill.Name)
	}
	if err := lib.RecordSource(s.source(), discovered, addedSkills); err != nil {
		return summary, err
	}
	summary.Added = len(addedSkills)
	return summary, nil
}

// ScanLocal is source::scan_local: find every skill below a folder;
// hidden directories are skipped at every depth so repository metadata
// and tool caches never appear as candidates.
func ScanLocal(root string) (*SkillScan, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, errors.New("请选择一个文件夹")
	}
	var skillDirs []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("无法读取文件夹：%s: %w", root, err)
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && d.Name() == "SKILL.md" {
			skillDirs = append(skillDirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(skillDirs)
	var skills []ScannedSkill
	names := map[string]bool{}
	for _, dir := range skillDirs {
		name, description, err := readFrontmatter(filepath.Join(dir, "SKILL.md"))
		if err != nil {
			return nil, err
		}
		if name == "" {
			name = filepath.Base(dir)
		}
		if name == "" {
			return nil, errors.New("无法确定技能名称")
		}
		if names[name] {
			return nil, fmt.Errorf("发现多个名为 %s 的技能，请调整名称后重试", name)
		}
		names[name] = true
		skills = append(skills, ScannedSkill{Name: name, Description: description, Path: dir})
	}
	if len(skills) == 0 {
		return nil, errors.New("这个文件夹中没有找到可用的技能")
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return &SkillScan{
		origin: scanOrigin{kind: "local", root: root, label: root},
		skills: skills,
	}, nil
}

// ScanNpx is source::scan_npx.
func ScanNpx(ctx context.Context, input string) (*SkillScan, error) {
	repository, err := NormalizeNpxSource(input)
	if err != nil {
		return nil, err
	}
	temp, err := os.MkdirTemp("", "kitter-npx-scan-")
	if err != nil {
		return nil, err
	}
	if err := npxAdd(ctx, temp, repository, "*"); err != nil {
		os.RemoveAll(temp)
		return nil, err
	}
	root := filepath.Join(temp, ".agents", "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		os.RemoveAll(temp)
		return nil, fmt.Errorf("没有从来源中找到技能：%s", repository)
	}
	var skills []ScannedSkill
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if !entry.IsDir() || !isFile(filepath.Join(path, "SKILL.md")) {
			continue
		}
		name, description, err := readFrontmatter(filepath.Join(path, "SKILL.md"))
		if err != nil {
			os.RemoveAll(temp)
			return nil, err
		}
		if name == "" {
			name = entry.Name()
		}
		if name == "" {
			os.RemoveAll(temp)
			return nil, errors.New("无法确定技能名称")
		}
		skills = append(skills, ScannedSkill{Name: name, Description: description, Path: path})
	}
	if len(skills) == 0 {
		os.RemoveAll(temp)
		return nil, errors.New("这个地址中没有找到可用的技能")
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return &SkillScan{
		origin: scanOrigin{kind: "npx", repository: repository},
		skills: skills,
		temp:   temp,
	}, nil
}

// ScanClaude is source::scan_claude.
func ScanClaude(ctx context.Context, input string) (*SkillScan, error) {
	plugin, err := NormalizeClaudePlugin(input)
	if err != nil {
		return nil, err
	}
	if err := Run(ctx, ToolCommand("claude", "plugin", "install", plugin)); err != nil {
		return nil, err
	}
	paths, err := findClaudeSkills(plugin)
	if err != nil {
		return nil, err
	}
	var skills []ScannedSkill
	for _, path := range paths {
		name, description, err := readFrontmatter(filepath.Join(path, "SKILL.md"))
		if err != nil {
			return nil, err
		}
		if name == "" {
			name = filepath.Base(path)
		}
		if name == "" {
			return nil, errors.New("无法确定技能名称")
		}
		skills = append(skills, ScannedSkill{Name: name, Description: description, Path: path})
	}
	if len(skills) == 0 {
		return nil, errors.New("这个插件中没有找到可用的技能")
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return &SkillScan{
		origin: scanOrigin{kind: "claude", plugin: plugin},
		skills: skills,
	}, nil
}

// NormalizeNpxSource is normalize_npx_source.
func NormalizeNpxSource(input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", errors.New("请输入 skills.sh 或 GitHub 地址")
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "git@") {
		return normalizeRepositoryURL(value), nil
	}
	parts := strings.Fields(value)
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "add" {
			return normalizeRepositoryURL(parts[i+1]), nil
		}
	}
	for _, part := range parts {
		if strings.HasPrefix(part, "http") {
			return normalizeRepositoryURL(part), nil
		}
	}
	return "", errors.New("没有识别到 skills.sh 或 GitHub 地址")
}

func normalizeRepositoryURL(value string) string {
	if path, ok := strings.CutPrefix(value, "git@github.com:"); ok {
		return "https://github.com/" + strings.TrimSuffix(path, ".git")
	}
	return value
}

// NormalizeClaudePlugin is normalize_claude_plugin.
func NormalizeClaudePlugin(input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", errors.New("请输入 Claude 插件名称")
	}
	parts := strings.Fields(value)
	if len(parts) == 1 {
		return parts[0], nil
	}
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "install" {
			return parts[i+1], nil
		}
	}
	return "", errors.New("没有识别到 Claude 插件名称")
}

// Update is source::update.
func Update(ctx context.Context, lib *library.SkillLibrary, name string) error {
	record, err := lib.Record(name)
	if err != nil {
		return err
	}
	return updateRecord(ctx, lib, record)
}

// UpdateByStorage is source::update_by_storage.
func UpdateByStorage(ctx context.Context, lib *library.SkillLibrary, storageName string) error {
	record, err := lib.RecordByStorage(storageName)
	if err != nil {
		return err
	}
	return updateRecord(ctx, lib, record)
}

func updateRecord(ctx context.Context, lib *library.SkillLibrary, record model.SkillRecord) error {
	storageName := record.StorageName
	if lib.IsLinkedSource(storageName) {
		return errors.New("此技能链接到原始目录，请在来源中更新")
	}
	var updatedOrigin *model.SkillOrigin
	var source string
	switch record.Origin.Type {
	case "builtin":
		return errors.New("Kitter 内置 Skill 会随 Kitter 自动更新")
	case "npx":
		repository, skill := record.Origin.Repository, record.Origin.Skill
		workspace := NpxWorkspace(repository)
		if err := ensureNpxSkill(ctx, workspace, repository, skill); err != nil {
			return err
		}
		if err := npxUpdate(ctx, workspace, skill); err != nil {
			return err
		}
		hash, err := npxLockHash(workspace, skill)
		if err != nil {
			return err
		}
		origin := model.OriginNpx(repository, skill, hash)
		updatedOrigin = &origin
		source = npxSkillPath(workspace, skill)
	case "claude_marketplace":
		plugin, skill := record.Origin.Plugin, record.Origin.Skill
		if err := Run(ctx, ToolCommand("claude", "plugin", "update", plugin)); err != nil {
			return err
		}
		path, err := findClaudeSkill(plugin, skill)
		if err != nil {
			return err
		}
		source = path
	case "git":
		temp, err := os.MkdirTemp("", "kitter-git-update-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temp)
		if err := Run(ctx, ToolCommand("git", "clone", "--depth", "1", record.Origin.Repository, filepath.Join(temp, "repo"))); err != nil {
			return err
		}
		base := filepath.Join(temp, "repo")
		if record.Origin.HasSubdir {
			base = filepath.Join(base, record.Origin.Subdir)
		}
		path, err := findSkillDir(base)
		if err != nil {
			return err
		}
		source = path
	case "local":
		source = record.Origin.Path
	default:
		return errors.New("这个技能没有可用的更新来源")
	}
	if updatedOrigin != nil {
		record.Origin = *updatedOrigin
	}
	record.UpdateAvailable = false
	return lib.ReplaceByStorage(source, storageName, record)
}

// CheckUpdates is source::check_updates.
func CheckUpdates(ctx context.Context, lib *library.SkillLibrary) (int, error) {
	skills, err := lib.List()
	if err != nil {
		return 0, err
	}
	var records []model.SkillRecord
	for _, skill := range skills {
		if !skill.Record.Origin.IsBuiltin() && !lib.IsLinkedSource(skill.Record.StorageName) {
			records = append(records, skill.Record)
		}
	}

	// The upstream CLI owns Npx version detection. It updates the
	// persistent source workspace, then we compare its lock hash with the
	// hash recorded when the Kitter copy was last installed.
	npxSources := map[string][]string{}
	var npxOrder []string
	for _, record := range records {
		if record.Origin.Type == "npx" {
			repository := record.Origin.Repository
			if _, ok := npxSources[repository]; !ok {
				npxOrder = append(npxOrder, repository)
			}
			npxSources[repository] = append(npxSources[repository], record.Origin.Skill)
		}
	}
	sort.Strings(npxOrder) // BTreeMap order
	npxHashes := map[string]map[string]string{}
	var failures []string
	for _, repository := range npxOrder {
		hashes, err := func() (map[string]string, error) {
			workspace := NpxWorkspace(repository)
			for _, skill := range npxSources[repository] {
				if err := ensureNpxSkill(ctx, workspace, repository, skill); err != nil {
					return nil, err
				}
			}
			// `skills update` performs the upstream check and refreshes
			// only the source workspace. The Kitter library remains
			// unchanged until the user presses the update action.
			if err := npxUpdate(ctx, workspace, ""); err != nil {
				return nil, err
			}
			return NpxLockHashes(workspace)
		}()
		if err != nil {
			failures = append(failures, fmt.Sprintf("Npx 来源检查失败：%s", err))
		} else {
			npxHashes[repository] = hashes
		}
	}

	count := 0
	for _, record := range records {
		var available bool
		var err error
		if record.Origin.Type == "npx" {
			current, ok := npxHashes[record.Origin.Repository][record.Origin.Skill]
			available = ok && (!record.Origin.HasSourceHsh || record.Origin.SourceHash != current)
		} else {
			available, err = checkOne(ctx, lib, &record)
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s", record.Name, err))
			continue
		}
		if err := lib.SetUpdateAvailableByStorage(record.StorageName, available); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s", record.Name, err))
			continue
		}
		if available {
			count++
		}
	}
	if len(failures) > 0 {
		return count, fmt.Errorf("部分技能检查失败：%s", strings.Join(failures, "；"))
	}
	return count, nil
}

func checkOne(ctx context.Context, lib *library.SkillLibrary, record *model.SkillRecord) (bool, error) {
	var source string
	switch record.Origin.Type {
	case "builtin", "npx":
		return false, nil
	case "git":
		temp, err := os.MkdirTemp("", "kitter-git-check-")
		if err != nil {
			return false, err
		}
		defer os.RemoveAll(temp)
		if err := Run(ctx, ToolCommand("git", "clone", "--depth", "1", record.Origin.Repository, filepath.Join(temp, "repo"))); err != nil {
			return false, err
		}
		base := filepath.Join(temp, "repo")
		if record.Origin.HasSubdir {
			base = filepath.Join(base, record.Origin.Subdir)
		}
		path, err := findSkillDir(base)
		if err != nil {
			return false, err
		}
		source = path
	case "claude_marketplace":
		path, err := findClaudeSkill(record.Origin.Plugin, record.Origin.Skill)
		if err != nil {
			return false, err
		}
		source = path
	case "local":
		source = record.Origin.Path
	default:
		return false, nil
	}
	target, err := lib.SkillPathByStorage(record.StorageName)
	if err != nil {
		return false, err
	}
	same, err := SameTree(source, target)
	return !same, err
}

// npxLockFile is skills-lock.json / .agents/.skill-lock.json.
type npxLockFile struct {
	Skills map[string]npxLockEntry `json:"skills"`
}

type npxLockEntry struct {
	ComputedHash    string `json:"computedHash"`
	SkillFolderHash string `json:"skillFolderHash"`
}

func (e npxLockEntry) hash() string {
	if e.SkillFolderHash != "" {
		return e.SkillFolderHash
	}
	return e.ComputedHash
}

// NpxWorkspace is npx_workspace: the persistent per-repository workspace
// under the app data dir (NpxDataDir in tests).
func NpxWorkspace(repository string) string {
	h := fnv.New64a()
	h.Write([]byte(repository))
	return filepath.Join(NpxDataDir(), "npx-sources", fmt.Sprintf("%016x", h.Sum64()))
}

// NpxDataDir is the root npx workspaces live under; tests point it at a
// temp dir instead of config.AppDataDir().
var NpxDataDir = config.AppDataDir

func npxSkillPath(workspace, skill string) string {
	return filepath.Join(workspace, ".agents", "skills", skill)
}

func npxAdd(ctx context.Context, workspace, repository, skill string) error {
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return err
	}
	return Run(ctx, ToolCommand("npx",
		"-y", "skills", "add", repository,
		"--skill", skill, "--agent", "universal", "--yes", "--copy",
	).Dir(workspace))
}

func npxUpdate(ctx context.Context, workspace, skill string) error {
	args := []string{"-y", "skills", "update", "--project", "--yes"}
	if skill != "" {
		args = append(args, skill)
	}
	return Run(ctx, ToolCommand("npx", args...).Dir(workspace))
}

func ensureNpxSkill(ctx context.Context, workspace, repository, skill string) error {
	hasLock := isFile(filepath.Join(workspace, "skills-lock.json")) ||
		isFile(filepath.Join(workspace, ".agents", ".skill-lock.json"))
	if !hasLock || !isFile(filepath.Join(npxSkillPath(workspace, skill), "SKILL.md")) {
		return npxAdd(ctx, workspace, repository, skill)
	}
	return nil
}

func npxLockHash(workspace, skill string) (*string, error) {
	hashes, err := NpxLockHashes(workspace)
	if err != nil {
		return nil, err
	}
	if hash, ok := hashes[skill]; ok {
		return &hash, nil
	}
	return nil, nil
}

// NpxLockHashes is npx_lock_hashes.
func NpxLockHashes(workspace string) (map[string]string, error) {
	paths := []string{
		filepath.Join(workspace, ".agents", ".skill-lock.json"),
		filepath.Join(workspace, "skills-lock.json"),
	}
	hashes := map[string]string{}
	for _, path := range paths {
		if !isFile(path) {
			continue
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取 Npx lock 文件失败：%s: %w", path, err)
		}
		var lock npxLockFile
		if err := json.Unmarshal(bytes, &lock); err != nil {
			return nil, fmt.Errorf("Npx lock 文件格式无效：%s: %w", path, err)
		}
		for name, entry := range lock.Skills {
			if hash := entry.hash(); hash != "" {
				hashes[name] = hash
			}
		}
	}
	return hashes, nil
}

// SameTree is same_tree.
func SameTree(left, right string) (bool, error) {
	collect := func(root string) ([][]byte, error) {
		var files [][]byte
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // filter_map(Result::ok) equivalent: skip errors
			}
			if !d.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			if strings.HasPrefix(rel, ".git"+string(os.PathSeparator)) || rel == ".git" {
				return nil
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			files = append(files, append([]byte(rel+"\x00"), bytes...))
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Slice(files, func(i, j int) bool { return string(files[i]) < string(files[j]) })
		return files, nil
	}
	l, err := collect(left)
	if err != nil {
		return false, err
	}
	r, err := collect(right)
	if err != nil {
		return false, err
	}
	if len(l) != len(r) {
		return false, nil
	}
	for i := range l {
		if string(l[i]) != string(r[i]) {
			return false, nil
		}
	}
	return true, nil
}

func findSkillDir(root string) (string, error) {
	if isFile(filepath.Join(root, "SKILL.md")) {
		return root, nil
	}
	var found string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr == nil && rel != "." && len(strings.Split(rel, string(os.PathSeparator))) > 4 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && d.Name() == "SKILL.md" {
			found = filepath.Dir(path)
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", errors.New("仓库中没有找到 SKILL.md")
	}
	return found, nil
}

func findClaudeSkill(plugin, skill string) (string, error) {
	candidates, err := findClaudeSkills(plugin)
	if err != nil {
		return "", err
	}
	var matching []string
	for _, path := range candidates {
		if filepath.Base(path) == skill {
			matching = append(matching, path)
		}
	}
	sortByMtime(matching)
	if len(matching) == 0 {
		return "", errors.New("Claude 插件中没有找到指定技能")
	}
	return matching[len(matching)-1], nil // last = newest
}

// findClaudeSkills walks ~/.claude/plugins/cache (HomeDir in tests).
func findClaudeSkills(plugin string) ([]string, error) {
	home := HomeDir()
	if home == "" {
		return nil, errors.New("无法确定用户目录")
	}
	root := filepath.Join(home, ".claude", "plugins", "cache")
	var candidates []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr == nil && rel != "." && len(strings.Split(rel, string(os.PathSeparator))) > 7 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() || !isFile(filepath.Join(path, "SKILL.md")) {
			return nil
		}
		belongs := plugin == ""
		if !belongs {
			belongs = true
			for _, part := range strings.Split(plugin, "@") {
				if part == "" {
					continue
				}
				found := false
				for _, component := range strings.Split(path, string(os.PathSeparator)) {
					if component == part {
						found = true
						break
					}
				}
				if !found {
					belongs = false
					break
				}
			}
		}
		if belongs {
			candidates = append(candidates, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortByMtime(candidates)
	if len(candidates) == 0 {
		return nil, errors.New("Claude 插件中没有找到技能")
	}
	return candidates, nil
}

// HomeDir returns the user home; tests override it. Defaults to
// os.UserHomeDir.
var HomeDir = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func sortByMtime(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		var mi, mj int64
		if info, err := os.Stat(paths[i]); err == nil {
			mi = info.ModTime().UnixNano()
		}
		if info, err := os.Stat(paths[j]); err == nil {
			mj = info.ModTime().UnixNano()
		}
		return mi < mj
	})
}

// readFrontmatter is library::read_frontmatter via skillfile.
func readFrontmatter(path string) (string, string, error) {
	return skillfile.ReadFrontmatter(path)
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
