// Package library mirrors src/library.rs: the managed skill library,
// registry.json, groups, and the embedded built-in Kitter skill.
package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/maphash"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/saltand/kitter/native/core/config"
	"github.com/saltand/kitter/native/core/effective"
	"github.com/saltand/kitter/native/core/fslink"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/project"
	"github.com/saltand/kitter/native/core/skillfile"
)

// KitterSkillStorage is the storage directory name of the built-in skill.
const KitterSkillStorage = "_kitter-builtin"

const kitterSkillName = "kitter"

// Registry mirrors the private Registry struct in library.rs.
//
// serde note: every collection field is #[serde(default)], which tolerates
// a missing key but not an explicit null. Marshal/Unmarshal below keep the
// Go nil representation off the wire and accept null when reading.
type Registry struct {
	Skills               map[string]model.SkillRecord       `json:"skills"`
	Sources              map[string]model.SkillSourceRecord `json:"sources"`
	Groups               []model.SkillGroup                 `json:"groups"`
	SourceGroupsMigrated bool                               `json:"source_groups_migrated"`
	AdoptedSources       map[string]AdoptedSource           `json:"adopted_sources"`
}

// MarshalJSON emits empty collections instead of null.
func (r *Registry) MarshalJSON() ([]byte, error) {
	type alias Registry
	r.normalize()
	return json.Marshal((*alias)(r))
}

// UnmarshalJSON accepts missing or null collection fields.
func (r *Registry) UnmarshalJSON(data []byte) error {
	type alias Registry
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*r = Registry(a)
	r.normalize()
	return nil
}

// AdoptedSource mirrors the private AdoptedSource struct.
type AdoptedSource struct {
	Source          string                 `json:"source"`
	References      []model.SkillReference `json:"references"`
	PreviousLibrary *string                `json:"previous_library,omitempty"`
}

// MarshalJSON keeps references a (possibly empty) array: Rust declares it
// without serde(default), so it must be present and non-null.
func (s AdoptedSource) MarshalJSON() ([]byte, error) {
	type alias AdoptedSource
	a := alias(s)
	if a.References == nil {
		a.References = []model.SkillReference{}
	}
	return json.Marshal(a)
}

// UnmarshalJSON accepts a missing or null references array.
func (s *AdoptedSource) UnmarshalJSON(data []byte) error {
	type alias AdoptedSource
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*s = AdoptedSource(a)
	if s.References == nil {
		s.References = []model.SkillReference{}
	}
	return nil
}

func newRegistry() *Registry {
	return &Registry{
		Skills:         map[string]model.SkillRecord{},
		Sources:        map[string]model.SkillSourceRecord{},
		Groups:         []model.SkillGroup{},
		AdoptedSources: map[string]AdoptedSource{},
	}
}

func (r *Registry) normalize() {
	if r.Skills == nil {
		r.Skills = map[string]model.SkillRecord{}
	}
	if r.Sources == nil {
		r.Sources = map[string]model.SkillSourceRecord{}
	}
	if r.Groups == nil {
		r.Groups = []model.SkillGroup{}
	}
	if r.AdoptedSources == nil {
		r.AdoptedSources = map[string]AdoptedSource{}
	}
}

// SkillLibrary mirrors library::SkillLibrary.
type SkillLibrary struct {
	Config   config.AppConfig
	Registry *Registry
	DataDir  string
}

// Open is SkillLibrary::open.
func Open() (*SkillLibrary, error) { return OpenIn(config.AppDataDir()) }

// OpenIn is SkillLibrary::open_in.
func OpenIn(dataDir string) (*SkillLibrary, error) {
	cfg, err := config.LoadFrom(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.LibraryDir, 0o755); err != nil {
		return nil, err
	}
	if err := EnsureBuiltinSkill(cfg.LibraryDir); err != nil {
		return nil, err
	}
	registry := newRegistry()
	path := filepath.Join(dataDir, "registry.json")
	if bytes, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(bytes, registry); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	registry.normalize()
	changed := false
	builtin, ok := registry.Skills[KitterSkillStorage]
	if !ok {
		changed = true
		builtin = model.SkillRecord{
			Name:           kitterSkillName,
			StorageName:    KitterSkillStorage,
			Origin:         model.OriginBuiltin(),
			LastOperatedAt: ^uint64(0),
		}
	}
	if builtin.Name != kitterSkillName ||
		builtin.StorageName != KitterSkillStorage ||
		!builtin.Origin.IsBuiltin() ||
		builtin.UpdateAvailable ||
		builtin.GroupID != nil ||
		builtin.LastOperatedAt != ^uint64(0) {
		builtin.Name = kitterSkillName
		builtin.StorageName = KitterSkillStorage
		builtin.Origin = model.OriginBuiltin()
		builtin.UpdateAvailable = false
		builtin.GroupID = nil
		builtin.LastOperatedAt = ^uint64(0)
		changed = true
	}
	registry.Skills[KitterSkillStorage] = builtin
	for storageName, record := range registry.Skills {
		if record.StorageName == "" {
			record.StorageName = storageName
			registry.Skills[storageName] = record
			changed = true
		}
	}
	for _, record := range registry.Skills {
		source := record.Origin.Source()
		key := source.Key()
		sourceRecord, exists := registry.Sources[key]
		if !exists {
			changed = true
			sourceRecord = model.SkillSourceRecord{
				Source:           source,
				DiscoveredSkills: []string{},
				AddedSkills:      []string{},
			}
		}
		if !contains(sourceRecord.DiscoveredSkills, record.Name) {
			sourceRecord.DiscoveredSkills = append(sourceRecord.DiscoveredSkills, record.Name)
			changed = true
		}
		if !contains(sourceRecord.AddedSkills, record.Name) {
			sourceRecord.AddedSkills = append(sourceRecord.AddedSkills, record.Name)
			changed = true
		}
		registry.Sources[key] = sourceRecord
	}
	if !registry.SourceGroupsMigrated {
		if len(registry.Groups) == 0 {
			type entry struct {
				source  model.SkillSource
				storage []string
			}
			sourceSkills := map[string]*entry{}
			var keys []string
			for storageName, record := range registry.Skills {
				source := record.Origin.Source()
				key := source.Key()
				e, ok := sourceSkills[key]
				if !ok {
					e = &entry{source: source}
					sourceSkills[key] = e
					keys = append(keys, key)
				}
				e.storage = append(e.storage, storageName)
			}
			sort.Strings(keys) // BTreeMap iteration order
			usedNames := map[string]bool{}
			for _, key := range keys {
				e := sourceSkills[key]
				if key == "unknown" || len(e.storage) < 2 {
					continue
				}
				name := uniqueGroupName(e.source.Label(), usedNames)
				id := migratedGroupID(key, registry.Groups)
				registry.Groups = append(registry.Groups, model.SkillGroup{
					ID:        id,
					Name:      name,
					CreatedAt: operationStamp(),
				})
				for _, storageName := range e.storage {
					record := registry.Skills[storageName]
					record.GroupID = strPtr(id)
					registry.Skills[storageName] = record
				}
			}
		}
		registry.SourceGroupsMigrated = true
		changed = true
	}
	groupIDs := map[string]bool{}
	for _, group := range registry.Groups {
		groupIDs[group.ID] = true
	}
	for storageName, record := range registry.Skills {
		if record.GroupID != nil && !groupIDs[*record.GroupID] {
			record.GroupID = nil
			registry.Skills[storageName] = record
			changed = true
		}
	}
	if changed {
		for key, source := range registry.Sources {
			sort.Strings(source.DiscoveredSkills)
			sort.Strings(source.AddedSkills)
			registry.Sources[key] = source
		}
	}
	library := &SkillLibrary{Config: *cfg, Registry: registry, DataDir: dataDir}
	if changed {
		if err := library.Save(); err != nil {
			return nil, err
		}
	}
	return library, nil
}

// Save is SkillLibrary::save.
func (l *SkillLibrary) Save() error {
	if err := config.SaveJSON(filepath.Join(l.DataDir, "config.json"), &l.Config); err != nil {
		return err
	}
	return config.SaveJSON(filepath.Join(l.DataDir, "registry.json"), l.Registry)
}

// List is SkillLibrary::list.
func (l *SkillLibrary) List() ([]model.SkillSummary, error) {
	var result []model.SkillSummary
	entries, err := os.ReadDir(l.Config.LibraryDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		path := filepath.Join(l.Config.LibraryDir, entry.Name())
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
			continue
		}
		storageName := entry.Name()
		_, description, _ := skillfile.ReadFrontmatter(filepath.Join(path, "SKILL.md"))
		record, ok := l.Registry.Skills[storageName]
		if !ok {
			record = model.SkillRecord{
				Name:        storageName,
				StorageName: storageName,
				Origin:      model.OriginUnknown(),
			}
		}
		if record.StorageName == "" {
			record.StorageName = storageName
		}
		record.Description = description
		result = append(result, model.SkillSummary{
			InstalledProjects: l.installCount(record.Name, path),
			Record:            record,
			ManualOnly:        effective.IsManualSkill(path),
			Path:              path,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		bi, bj := result[i].Record.Origin.IsBuiltin(), result[j].Record.Origin.IsBuiltin()
		if bi != bj {
			return bi
		}
		return result[i].Record.Name < result[j].Record.Name
	})
	return result, nil
}

func (l *SkillLibrary) installCount(name, source string) int {
	count := 0
	for _, p := range l.Config.RecentProjects {
		if project.IsInstalledAnyFromPath(p, name, source) {
			count++
		}
	}
	return count
}

// SkillPath is SkillLibrary::skill_path.
func (l *SkillLibrary) SkillPath(name string) (string, error) {
	summary, err := l.ResolveSkill(name)
	if err != nil {
		return "", err
	}
	return summary.Path, nil
}

// ResolveSkill is SkillLibrary::resolve_skill.
func (l *SkillLibrary) ResolveSkill(selector string) (*model.SkillSummary, error) {
	if storageName, ok := strings.CutPrefix(selector, "id:"); ok {
		if err := skillfile.ValidateName(storageName); err != nil {
			return nil, err
		}
		skills, err := l.List()
		if err != nil {
			return nil, err
		}
		for i := range skills {
			if skills[i].Record.StorageName == storageName {
				return &skills[i], nil
			}
		}
		return nil, fmt.Errorf("没有找到技能：%s", selector)
	}
	if err := skillfile.ValidateName(selector); err != nil {
		return nil, err
	}
	skills, err := l.List()
	if err != nil {
		return nil, err
	}
	var matches []model.SkillSummary
	for _, skill := range skills {
		if skill.Record.Name == selector {
			matches = append(matches, skill)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("没有找到技能：%s", selector)
	default:
		return nil, errors.New("存在多个同名技能，请使用 id:<值> 明确选择")
	}
}

// SkillPathByStorage is SkillLibrary::skill_path_by_storage.
func (l *SkillLibrary) SkillPathByStorage(storageName string) (string, error) {
	if err := skillfile.ValidateName(storageName); err != nil {
		return "", err
	}
	path := filepath.Join(l.Config.LibraryDir, storageName)
	if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
		return "", fmt.Errorf("没有找到技能：%s", storageName)
	}
	return path, nil
}

// Files is SkillLibrary::files.
func (l *SkillLibrary) Files(name string) ([]string, error) {
	root, err := l.SkillPath(name)
	if err != nil {
		return nil, err
	}
	return filesIn(root)
}

// FilesByStorage is SkillLibrary::files_by_storage.
func (l *SkillLibrary) FilesByStorage(storageName string) ([]string, error) {
	root, err := l.SkillPathByStorage(storageName)
	if err != nil {
		return nil, err
	}
	return filesIn(root)
}

func filesIn(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // WalkDir filter_map(Result::ok)
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(root, path)
			if err == nil {
				files = append(files, rel)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// FilesInDir is filesIn as a package-level helper: it lists the files
// under a skill directory without opening the library, for use on
// background goroutines where SkillLibrary's registry writes are off
// limits.
func FilesInDir(root string) ([]string, error) { return filesIn(root) }

// ReadFileInDir is readFileIn without a SkillLibrary: it resolves the
// symlinked root, keeps the result inside it, and reads UTF-8 text.
func ReadFileInDir(root, relative string) (string, error) {
	return readFileInDir(root, relative)
}

// ReadFile is SkillLibrary::read_file.
func (l *SkillLibrary) ReadFile(name, relative string) (string, error) {
	root, err := l.SkillPath(name)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	return l.readFileIn(root, relative)
}

// ReadFileByStorage is SkillLibrary::read_file_by_storage.
func (l *SkillLibrary) ReadFileByStorage(storageName, relative string) (string, error) {
	root, err := l.SkillPathByStorage(storageName)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	return l.readFileIn(root, relative)
}

func (l *SkillLibrary) readFileIn(root, relative string) (string, error) {
	return readFileInDir(root, relative)
}

func readFileInDir(root, relative string) (string, error) {
	path, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return "", errors.New("该文件不是可预览的文本文件")
	}
	rootWithSep := root + string(os.PathSeparator)
	if path != root && !strings.HasPrefix(path, rootWithSep) {
		return "", errors.New("文件不在技能目录内")
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("该文件不是可预览的文本文件")
	}
	if !utf8.Valid(bytes) {
		return "", errors.New("该文件不是可预览的文本文件")
	}
	return string(bytes), nil
}

// Import is SkillLibrary::import.
func (l *SkillLibrary) Import(source string, record model.SkillRecord) error {
	if _, err := os.Stat(filepath.Join(source, "SKILL.md")); err != nil {
		return errors.New("所选目录中没有 SKILL.md")
	}
	if err := skillfile.ValidateName(record.Name); err != nil {
		return err
	}
	identity := record.IdentityKey()
	for _, existing := range l.Registry.Skills {
		if existing.IdentityKey() == identity {
			return fmt.Errorf("这个来源中的技能已添加：%s", record.Name)
		}
	}
	storageName := l.storageNameFor(record.Name, identity)
	destination := filepath.Join(l.Config.LibraryDir, storageName)
	if err := copyTree(source, destination); err != nil {
		return err
	}
	record.StorageName = storageName
	if record.LastOperatedAt == 0 {
		record.LastOperatedAt = operationStamp()
	}
	if record.GroupID != nil {
		found := false
		for _, group := range l.Registry.Groups {
			if group.ID == *record.GroupID {
				found = true
				break
			}
		}
		if !found {
			record.GroupID = nil
		}
	}
	l.Registry.Skills[storageName] = record
	return l.Save()
}

// ContainsIdentity is SkillLibrary::contains_identity.
func (l *SkillLibrary) ContainsIdentity(identity string) bool {
	for _, record := range l.Registry.Skills {
		if record.IdentityKey() == identity {
			return true
		}
	}
	return false
}

// IsLinkedSource is SkillLibrary::is_linked_source.
func (l *SkillLibrary) IsLinkedSource(storageName string) bool {
	dl, err := fslink.Inspect(filepath.Join(l.Config.LibraryDir, storageName))
	return err == nil && dl != nil
}

// Replace is SkillLibrary::replace.
func (l *SkillLibrary) Replace(source string, record model.SkillRecord) error {
	storageName := record.StorageName
	if storageName == "" {
		storageName = record.Name
	}
	return l.ReplaceByStorage(source, storageName, record)
}

// ReplaceByStorage is SkillLibrary::replace_by_storage.
func (l *SkillLibrary) ReplaceByStorage(source, storageName string, record model.SkillRecord) error {
	if l.IsLinkedSource(storageName) {
		return errors.New("此技能链接到原始目录，请在来源中更新")
	}
	destination := filepath.Join(l.Config.LibraryDir, storageName)
	var affected []string
	for _, p := range l.Config.RecentProjects {
		if project.IsInstalledAnyFromPath(p, record.Name, destination) {
			affected = append(affected, p)
		}
	}
	if _, err := l.SkillPathByStorage(storageName); err != nil {
		return err
	}
	backup := strings.TrimSuffix(destination, filepath.Ext(destination)) + ".kitter-backup"
	if _, err := os.Stat(backup); err == nil {
		if err := os.RemoveAll(backup); err != nil {
			return err
		}
	}
	if err := os.Rename(destination, backup); err != nil {
		return err
	}
	if err := copyTree(source, destination); err != nil {
		os.RemoveAll(destination)
		os.Rename(backup, destination)
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	record.StorageName = storageName
	record.LastOperatedAt = operationStamp()
	if record.KitterManual {
		if effective.HasDisableModelInvocation(destination) {
			record.KitterManual = false
		} else {
			if err := applyDisableModelInvocation(filepath.Join(destination, "SKILL.md"), true); err != nil {
				return err
			}
		}
	}
	l.Registry.Skills[storageName] = record
	for _, p := range affected {
		l.Config.TouchProject(p)
	}
	return l.Save()
}

// Remove is SkillLibrary::remove.
func (l *SkillLibrary) Remove(name string) error {
	record, err := l.Record(name)
	if err != nil {
		return err
	}
	return l.RemoveByStorage(record.StorageName)
}

// RemoveByStorage is SkillLibrary::remove_by_storage.
func (l *SkillLibrary) RemoveByStorage(storageName string) error {
	record, err := l.RecordByStorage(storageName)
	if err != nil {
		return err
	}
	if record.Origin.IsBuiltin() {
		return errors.New("Kitter 内置 Skill 不能删除")
	}
	sourceKey := record.Origin.Source().Key()
	path, err := l.SkillPathByStorage(storageName)
	if err != nil {
		return err
	}
	var affected []string
	for _, p := range l.Config.RecentProjects {
		if project.IsInstalledAnyFromPath(p, record.Name, path) {
			affected = append(affected, p)
		}
	}
	for _, p := range l.Config.RecentProjects {
		if err := project.UninstallAllFromPath(p, record.Name, path); err != nil {
			return err
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if err := project.UninstallAllFromPath(home, record.Name, path); err != nil {
			return err
		}
	}
	if adopted, ok := l.Registry.AdoptedSources[storageName]; ok {
		for _, reference := range adopted.References {
			if reference.Kind != model.ReferenceLink {
				continue
			}
			link, err := fslink.Inspect(reference.Path)
			if err != nil {
				return err
			}
			if link == nil {
				continue
			}
			// Never remove a reference that another tool/user repointed.
			if resolved, err := filepath.EvalSymlinks(reference.Path); err == nil && resolved == adopted.Source {
				if err := fslink.Remove(reference.Path, *link); err != nil {
					return err
				}
			}
		}
	}
	if dl, err := fslink.Inspect(path); err != nil {
		return err
	} else if dl != nil {
		if err := fslink.Remove(path, *dl); err != nil {
			return err
		}
	} else {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	delete(l.Registry.Skills, storageName)
	delete(l.Registry.AdoptedSources, storageName)
	if source, ok := l.Registry.Sources[sourceKey]; ok {
		var kept []string
		for _, s := range source.AddedSkills {
			if s != record.Name {
				kept = append(kept, s)
			}
		}
		source.AddedSkills = kept
		l.Registry.Sources[sourceKey] = source
	}
	for _, p := range affected {
		l.Config.TouchProject(p)
	}
	return l.Save()
}

// RecordSource is SkillLibrary::record_source.
func (l *SkillLibrary) RecordSource(source model.SkillSource, discovered, added []string) error {
	sort.Strings(discovered)
	discovered = dedup(discovered)
	key := source.Key()
	record, ok := l.Registry.Sources[key]
	if !ok {
		record = model.SkillSourceRecord{
			Source:           source,
			DiscoveredSkills: []string{},
			AddedSkills:      []string{},
		}
	}
	record.Source = source
	record.DiscoveredSkills = discovered
	record.AddedSkills = append(record.AddedSkills, added...)
	sort.Strings(record.AddedSkills)
	record.AddedSkills = dedup(record.AddedSkills)
	l.Registry.Sources[key] = record
	return l.Save()
}

// SourceRecords is SkillLibrary::source_records.
func (l *SkillLibrary) SourceRecords() []model.SkillSourceRecord {
	var out []model.SkillSourceRecord
	for _, s := range l.Registry.Sources {
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Source.Label() < out[j].Source.Label() })
	return out
}

// Groups is SkillLibrary::groups.
func (l *SkillLibrary) Groups() []model.SkillGroup {
	return append([]model.SkillGroup(nil), l.Registry.Groups...)
}

// MoveGroup is SkillLibrary::move_group.
func (l *SkillLibrary) MoveGroup(id, target string, after bool) (bool, error) {
	groups := l.Registry.Groups
	from, to := -1, -1
	for i, g := range groups {
		if g.ID == id {
			from = i
		}
		if g.ID == target {
			to = i
		}
	}
	if from < 0 || to < 0 || from == to {
		return false, nil
	}
	insert := to
	if after {
		insert++
	}
	if from < to {
		insert--
	}
	if from == insert {
		return false, nil
	}
	previous := append([]model.SkillGroup(nil), groups...)
	group := groups[from]
	groups = append(groups[:from], groups[from+1:]...)
	groups = append(groups[:insert], append([]model.SkillGroup{group}, groups[insert:]...)...)
	l.Registry.Groups = groups
	if err := l.Save(); err != nil {
		l.Registry.Groups = previous
		return false, err
	}
	return true, nil
}

// EnsureGroup is SkillLibrary::ensure_group.
func (l *SkillLibrary) EnsureGroup(name string) (string, error) {
	name, err := normalizeGroupName(name)
	if err != nil {
		return "", err
	}
	for _, group := range l.Registry.Groups {
		if strings.EqualFold(group.Name, name) {
			return group.ID, nil
		}
	}
	id := l.newGroupID()
	l.Registry.Groups = append(l.Registry.Groups, model.SkillGroup{
		ID:        id,
		Name:      name,
		CreatedAt: operationStamp(),
	})
	if err := l.Save(); err != nil {
		return "", err
	}
	return id, nil
}

// CreateGroup is SkillLibrary::create_group.
func (l *SkillLibrary) CreateGroup(name string) (*model.SkillGroup, error) {
	name, err := normalizeGroupName(name)
	if err != nil {
		return nil, err
	}
	for _, group := range l.Registry.Groups {
		if strings.EqualFold(group.Name, name) {
			return nil, errors.New("已经存在同名分组")
		}
	}
	group := model.SkillGroup{
		ID:        l.newGroupID(),
		Name:      name,
		CreatedAt: operationStamp(),
	}
	l.Registry.Groups = append(l.Registry.Groups, group)
	if err := l.Save(); err != nil {
		return nil, err
	}
	return &group, nil
}

// RenameGroup is SkillLibrary::rename_group.
func (l *SkillLibrary) RenameGroup(id, name string) error {
	name, err := normalizeGroupName(name)
	if err != nil {
		return err
	}
	for _, group := range l.Registry.Groups {
		if group.ID != id && strings.EqualFold(group.Name, name) {
			return errors.New("已经存在同名分组")
		}
	}
	for i := range l.Registry.Groups {
		if l.Registry.Groups[i].ID == id {
			l.Registry.Groups[i].Name = name
			return l.Save()
		}
	}
	return errors.New("分组不存在")
}

// AssignGroup is SkillLibrary::assign_group.
func (l *SkillLibrary) AssignGroup(skill string, groupID *string) error {
	if _, ok := l.Registry.Skills[skill]; ok {
		return l.AssignGroupByStorage(skill, groupID)
	}
	record, err := l.Record(skill)
	if err != nil {
		return err
	}
	return l.AssignGroupByStorage(record.StorageName, groupID)
}

// DeleteGroup is SkillLibrary::delete_group.
func (l *SkillLibrary) DeleteGroup(id string, deleteSkills bool) ([]string, error) {
	var storageNames []string
	for storageName, record := range l.Registry.Skills {
		if record.GroupID != nil && *record.GroupID == id {
			if record.StorageName == "" {
				storageNames = append(storageNames, record.Name)
			} else {
				storageNames = append(storageNames, record.StorageName)
			}
			_ = storageName
		}
	}
	if deleteSkills {
		for _, storageName := range storageNames {
			if err := l.RemoveByStorage(storageName); err != nil {
				return nil, err
			}
		}
	} else {
		for storageName, record := range l.Registry.Skills {
			if record.GroupID != nil && *record.GroupID == id {
				record.GroupID = nil
				record.LastOperatedAt = operationStamp()
				l.Registry.Skills[storageName] = record
			}
		}
	}
	var kept []model.SkillGroup
	for _, group := range l.Registry.Groups {
		if group.ID != id {
			kept = append(kept, group)
		}
	}
	l.Registry.Groups = kept
	if err := l.Save(); err != nil {
		return nil, err
	}
	return storageNames, nil
}

func (l *SkillLibrary) newGroupID() string {
	stamp := operationStamp()
	id := fmt.Sprintf("group-%d", stamp)
	suffix := 1
	for groupIDExists(l.Registry.Groups, id) {
		id = fmt.Sprintf("group-%d-%d", stamp, suffix)
		suffix++
	}
	return id
}

func groupIDExists(groups []model.SkillGroup, id string) bool {
	for _, g := range groups {
		if g.ID == id {
			return true
		}
	}
	return false
}

// Record is SkillLibrary::record.
func (l *SkillLibrary) Record(name string) (model.SkillRecord, error) {
	summary, err := l.ResolveSkill(name)
	if err != nil {
		return model.SkillRecord{}, err
	}
	return summary.Record, nil
}

// RecordByStorage is SkillLibrary::record_by_storage.
func (l *SkillLibrary) RecordByStorage(storageName string) (model.SkillRecord, error) {
	skills, err := l.List()
	if err != nil {
		return model.SkillRecord{}, err
	}
	for _, skill := range skills {
		if skill.Record.StorageName == storageName {
			return skill.Record, nil
		}
	}
	return model.SkillRecord{}, errors.New("技能不存在")
}

// SetUpdateAvailable is SkillLibrary::set_update_available.
func (l *SkillLibrary) SetUpdateAvailable(name string, available bool) error {
	record, err := l.Record(name)
	if err != nil {
		return err
	}
	return l.SetUpdateAvailableByStorage(record.StorageName, available)
}

// SetUpdateAvailableByStorage is SkillLibrary::set_update_available_by_storage.
func (l *SkillLibrary) SetUpdateAvailableByStorage(storageName string, available bool) error {
	record, ok := l.Registry.Skills[storageName]
	if !ok {
		return errors.New("技能不存在")
	}
	record.UpdateAvailable = available
	l.Registry.Skills[storageName] = record
	return l.Save()
}

// AssignGroupByStorage is SkillLibrary::assign_group_by_storage.
func (l *SkillLibrary) AssignGroupByStorage(storageName string, groupID *string) error {
	if record, ok := l.Registry.Skills[storageName]; ok && record.Origin.IsBuiltin() {
		return errors.New("Kitter 内置 Skill 固定显示在列表顶部")
	}
	if groupID != nil && !groupIDExists(l.Registry.Groups, *groupID) {
		return errors.New("分组不存在")
	}
	record, ok := l.Registry.Skills[storageName]
	if !ok {
		return errors.New("技能不存在")
	}
	record.GroupID = groupID
	record.LastOperatedAt = operationStamp()
	l.Registry.Skills[storageName] = record
	return l.Save()
}

// SetKitterManualByStorage is SkillLibrary::set_kitter_manual_by_storage.
func (l *SkillLibrary) SetKitterManualByStorage(storageName string, enabled bool) error {
	if err := skillfile.ValidateName(storageName); err != nil {
		return err
	}
	path, err := l.SkillPathByStorage(storageName)
	if err != nil {
		return err
	}
	record, ok := l.Registry.Skills[storageName]
	if !ok {
		return errors.New("技能不存在")
	}
	if record.Origin.IsBuiltin() {
		return errors.New("这个技能本身就是仅手动触发")
	}
	if enabled {
		if !record.KitterManual && effective.HasDisableModelInvocation(path) {
			return errors.New("这个技能本身就是仅手动触发")
		}
	} else if !record.KitterManual {
		return errors.New("这个技能不是由 Kitter 设为仅手动触发")
	}
	if err := applyDisableModelInvocation(filepath.Join(path, "SKILL.md"), enabled); err != nil {
		return err
	}
	record.KitterManual = enabled
	record.LastOperatedAt = operationStamp()
	l.Registry.Skills[storageName] = record
	return l.Save()
}

func (l *SkillLibrary) storageNameFor(name, identity string) string {
	direct := filepath.Join(l.Config.LibraryDir, name)
	if _, err := os.Stat(direct); errors.Is(err, fs.ErrNotExist) {
		if _, taken := l.Registry.Skills[name]; !taken {
			return name
		}
	}
	var h maphash.Hash
	h.WriteString(identity)
	suffix := fmt.Sprintf("%08x", uint32(h.Sum64()))
	base := fmt.Sprintf("%s--%s", name, suffix)
	candidate := base
	index := 2
	for {
		if _, err := os.Stat(filepath.Join(l.Config.LibraryDir, candidate)); errors.Is(err, fs.ErrNotExist) {
			if _, taken := l.Registry.Skills[candidate]; !taken {
				return candidate
			}
		}
		candidate = fmt.Sprintf("%s-%d", base, index)
		index++
	}
}

func strPtr(s string) *string { return &s }

func contains(list []string, item string) bool {
	for _, s := range list {
		if s == item {
			return true
		}
	}
	return false
}

func dedup(sorted []string) []string {
	if len(sorted) == 0 {
		return sorted
	}
	out := sorted[:1]
	for _, s := range sorted[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

func operationStamp() uint64 {
	return uint64(time.Now().UnixMilli())
}

func normalizeGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("请输入分组名称")
	}
	return name, nil
}

func uniqueGroupName(name string, usedNames map[string]bool) string {
	base := name
	candidate := base
	suffix := 2
	for {
		taken := false
		for existing := range usedNames {
			if strings.EqualFold(existing, candidate) {
				taken = true
				break
			}
		}
		if !taken {
			break
		}
		candidate = fmt.Sprintf("%s %d", base, suffix)
		suffix++
	}
	usedNames[candidate] = true
	return candidate
}

func migratedGroupID(sourceKey string, groups []model.SkillGroup) string {
	var h maphash.Hash
	h.WriteString(sourceKey)
	base := fmt.Sprintf("source-%08x", uint32(h.Sum64()))
	candidate := base
	suffix := 2
	for groupIDExists(groups, candidate) {
		candidate = fmt.Sprintf("%s-%d", base, suffix)
		suffix++
	}
	return candidate
}
