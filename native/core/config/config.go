// Package config mirrors src/config.rs: the app config file, the data
// directory, and atomic JSON persistence shared by all state files.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Language mirrors config::Language (serde snake_case unit enum).
type Language string

const (
	LanguageSystem Language = "system"
	LanguageZhCn   Language = "zh_cn"
	LanguageEn     Language = "en"
)

var systemLanguageOnce struct {
	sync.Once
	value Language
}

// SystemLanguage is Language::system(): the detected OS language.
func SystemLanguage() Language {
	systemLanguageOnce.Once.Do(func() {
		systemLanguageOnce.value = languageFromLocale(detectLocale())
	})
	return systemLanguageOnce.value
}

// detectLocale reads the platform locale. On macOS AppleLanguages is
// authoritative; LANG is the portable fallback.
func detectLocale() string {
	if runtime.GOOS == "darwin" {
		if langs := os.Getenv("APPLE_LANGUAGES"); langs != "" {
			if first, _, ok := strings.Cut(langs, ","); ok {
				return strings.TrimSpace(first)
			}
			return strings.TrimSpace(langs)
		}
	}
	if v := os.Getenv("LC_ALL"); v != "" {
		return v
	}
	if v := os.Getenv("LC_MESSAGES"); v != "" {
		return v
	}
	return os.Getenv("LANG")
}

// LanguageFromLocale is Language::from_locale.
func LanguageFromLocale(locale string) Language { return languageFromLocale(locale) }

func languageFromLocale(locale string) Language {
	if strings.HasPrefix(strings.ToLower(locale), "zh") {
		return LanguageZhCn
	}
	return LanguageEn
}

// Theme mirrors config::Theme (serde snake_case unit enum).
type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// UnmarshalJSON rejects unknown variants, as serde does.
func (t *Theme) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch Theme(s) {
	case ThemeSystem, ThemeLight, ThemeDark:
		*t = Theme(s)
		return nil
	default:
		return fmt.Errorf("unknown Theme variant %q", s)
	}
}

// UnmarshalJSON rejects unknown variants, as serde does.
func (l *Language) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch Language(s) {
	case LanguageSystem, LanguageZhCn, LanguageEn:
		*l = Language(s)
		return nil
	default:
		return fmt.Errorf("unknown Language variant %q", s)
	}
}

// AppConfig mirrors config::AppConfig in config.json.
//
// serde note: recent_projects/project_activity are #[serde(default)],
// which tolerates a missing key but not an explicit null; marshal and
// unmarshal below keep nil off the wire and accept null when reading.
type AppConfig struct {
	Language        Language          `json:"language"`
	Theme           Theme             `json:"theme"`
	LibraryDir      string            `json:"library_dir"`
	RecentProjects  []string          `json:"recent_projects"`
	ProjectActivity map[string]uint64 `json:"project_activity"`
	// CollapsedSkillGroups mirrors collapsed_skill_groups (BTreeSet → a
	// sorted JSON array), added by feat/persist-collapsed.
	CollapsedSkillGroups map[string]bool `json:"-"`
}

type appConfigJSON struct {
	Language             Language          `json:"language"`
	Theme                Theme             `json:"theme"`
	LibraryDir           string            `json:"library_dir"`
	RecentProjects       []string          `json:"recent_projects"`
	ProjectActivity      map[string]uint64 `json:"project_activity"`
	CollapsedSkillGroups []string          `json:"collapsed_skill_groups"`
}

func (c *AppConfig) normalize() {
	if c.RecentProjects == nil {
		c.RecentProjects = []string{}
	}
	if c.ProjectActivity == nil {
		c.ProjectActivity = map[string]uint64{}
	}
	if c.CollapsedSkillGroups == nil {
		c.CollapsedSkillGroups = map[string]bool{}
	}
}

// MarshalJSON emits empty collections instead of null and the collapsed
// group set as a BTreeSet-style sorted array.
func (c AppConfig) MarshalJSON() ([]byte, error) {
	c.normalize()
	groups := make([]string, 0, len(c.CollapsedSkillGroups))
	for id := range c.CollapsedSkillGroups {
		groups = append(groups, id)
	}
	sort.Strings(groups)
	return json.Marshal(appConfigJSON{
		Language:             c.Language,
		Theme:                c.Theme,
		LibraryDir:           c.LibraryDir,
		RecentProjects:       c.RecentProjects,
		ProjectActivity:      c.ProjectActivity,
		CollapsedSkillGroups: groups,
	})
}

// UnmarshalJSON accepts missing or null collection fields.
func (c *AppConfig) UnmarshalJSON(data []byte) error {
	var j appConfigJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	c.Language = j.Language
	c.Theme = j.Theme
	c.LibraryDir = j.LibraryDir
	c.RecentProjects = j.RecentProjects
	c.ProjectActivity = j.ProjectActivity
	c.CollapsedSkillGroups = map[string]bool{}
	for _, id := range j.CollapsedSkillGroups {
		c.CollapsedSkillGroups[id] = true
	}
	c.normalize()
	return nil
}

// Default returns the serde defaults: language/theme "system", an empty
// project list, and the library under the data directory.
func Default() AppConfig {
	return AppConfig{
		Language:             LanguageSystem,
		Theme:                ThemeSystem,
		LibraryDir:           filepath.Join(AppDataDir(), "skills"),
		RecentProjects:       []string{},
		ProjectActivity:      map[string]uint64{},
		CollapsedSkillGroups: map[string]bool{},
	}
}

// AppDataDir is config::app_data_dir: $KITTER_HOME when set, otherwise the
// per-user data directory (~/Library/Application Support/Kitter on macOS).
func AppDataDir() string {
	if path := os.Getenv("KITTER_HOME"); path != "" {
		return path
	}
	var base string
	switch runtime.GOOS {
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, "Library", "Application Support")
		}
	case "windows":
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = os.Getenv("APPDATA")
		}
	default:
		base = os.Getenv("XDG_DATA_HOME")
		if base == "" {
			if home, err := os.UserHomeDir(); err == nil {
				base = filepath.Join(home, ".local", "share")
			}
		}
	}
	if base == "" {
		base = "."
	}
	return filepath.Join(base, "Kitter")
}

// Path is AppConfig::path().
func Path() string { return filepath.Join(AppDataDir(), "config.json") }

// Load reads config.json from the data directory.
func Load() (*AppConfig, error) { return LoadFrom(AppDataDir()) }

// LoadFrom is AppConfig::load_from: a missing file yields defaults rooted
// at dataDir.
func LoadFrom(dataDir string) (*AppConfig, error) {
	path := filepath.Join(dataDir, "config.json")
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default()
			cfg.LibraryDir = filepath.Join(dataDir, "skills")
			return &cfg, nil
		}
		return nil, fmt.Errorf("读取配置失败：%s: %w", path, err)
	}
	cfg := Default()
	if err := json.Unmarshal(bytes, &cfg); err != nil {
		return nil, fmt.Errorf("配置格式无效: %w", err)
	}
	cfg.normalize()
	return &cfg, nil
}

// Save writes config.json under the data directory.
func (c *AppConfig) Save() error { return c.SaveTo(AppDataDir()) }

// SaveTo is AppConfig::save_to.
func (c *AppConfig) SaveTo(dataDir string) error {
	return SaveJSON(filepath.Join(dataDir, "config.json"), c)
}

// RememberProject is AppConfig::remember_project.
func (c *AppConfig) RememberProject(path string) {
	_, hasActivity := c.ProjectActivity[path]
	filtered := c.RecentProjects[:0]
	for _, item := range c.RecentProjects {
		if item != path {
			filtered = append(filtered, item)
		}
	}
	c.RecentProjects = append([]string{path}, filtered...)
	if len(c.RecentProjects) > 8 {
		c.RecentProjects = c.RecentProjects[:8]
	}
	if !hasActivity {
		c.TouchProject(path)
	} else {
		c.pruneProjectActivity()
	}
}

// ProjectPaths is AppConfig::project_paths: sorted by activity, recency as
// tie-breaker.
func (c *AppConfig) ProjectPaths() []string {
	type entry struct {
		index int
		path  string
	}
	projects := make([]entry, len(c.RecentProjects))
	for i, p := range c.RecentProjects {
		projects[i] = entry{i, p}
	}
	sort.SliceStable(projects, func(i, j int) bool {
		li, lj := c.ProjectActivity[projects[i].path], c.ProjectActivity[projects[j].path]
		if li != lj {
			return li > lj
		}
		return projects[i].index < projects[j].index
	})
	out := make([]string, len(projects))
	for i, e := range projects {
		out[i] = e.path
	}
	return out
}

// TouchProject is AppConfig::touch_project.
func (c *AppConfig) TouchProject(path string) {
	found := false
	for _, p := range c.RecentProjects {
		if p == path {
			found = true
			break
		}
	}
	if !found {
		c.RecentProjects = append([]string{path}, c.RecentProjects...)
	}
	now := uint64(time.Now().UnixMilli())
	var maxVal uint64
	for _, v := range c.ProjectActivity {
		if v > maxVal {
			maxVal = v
		}
	}
	next := maxVal + 1
	if now > next {
		next = now
	}
	if c.ProjectActivity == nil {
		c.ProjectActivity = map[string]uint64{}
	}
	c.ProjectActivity[path] = next
	if len(c.RecentProjects) > 8 {
		c.RecentProjects = c.RecentProjects[:8]
	}
	c.pruneProjectActivity()
}

// RemoveProject is AppConfig::remove_project.
func (c *AppConfig) RemoveProject(path string) {
	filtered := c.RecentProjects[:0]
	for _, p := range c.RecentProjects {
		if p != path {
			filtered = append(filtered, p)
		}
	}
	c.RecentProjects = filtered
	delete(c.ProjectActivity, path)
}

func (c *AppConfig) pruneProjectActivity() {
	for path := range c.ProjectActivity {
		found := false
		for _, p := range c.RecentProjects {
			if p == path {
				found = true
				break
			}
		}
		if !found {
			delete(c.ProjectActivity, path)
		}
	}
}

// SaveJSON is config::save_json: pretty JSON written atomically via a
// sibling .tmp file.
func SaveJSON(path string, value any) error {
	if parent := filepath.Dir(path); parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	data, err := marshalPretty(value)
	if err != nil {
		return err
	}
	// with_extension("json.tmp") replaces the last extension.
	temp := path[:len(path)-len(filepath.Ext(path))] + "json.tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// marshalPretty matches serde_json::to_vec_pretty: two-space indentation,
// no trailing newline, and no HTML escaping of the output.
func marshalPretty(value any) ([]byte, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	// Encoder adds a trailing newline; serde does not.
	return []byte(strings.TrimSuffix(b.String(), "\n")), nil
}
