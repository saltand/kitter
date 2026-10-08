// helpers.go ports text.rs and the free functions of ui/mod.rs used by
// the skills page: display_path, skill_storage_name,
// unique_installation_paths, unique_external_sources and same_file.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/project"
)

// Counted formats an English count with the appropriate singular or
// plural noun (text::counted).
func Counted(count int, singular, plural string) string {
	noun := plural
	if count == 1 {
		noun = singular
	}
	return fmt.Sprintf("%d %s", count, noun)
}

// SkillStorageName is skill_storage_name.
func SkillStorageName(skill *model.SkillSummary) string {
	if skill.Record.StorageName == "" {
		return skill.Record.Name
	}
	return skill.Record.StorageName
}

// displayPath is display_path: collapse the home directory to "~".
func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "~/" + rel
	}
	return path
}

// uniqueInstallationPaths is unique_installation_paths: deduplicate by
// the physical installation key.
func uniqueInstallationPaths(installations []model.ProjectSkillInstallation) []string {
	seen := map[string]bool{}
	var out []string
	for _, inst := range installations {
		key := project.InstallationKey(inst.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, inst.Path)
	}
	return out
}

// uniqueExternalSources is unique_external_sources.
func uniqueExternalSources(kinds []project.RemovalKind, installations []model.ProjectSkillInstallation) []string {
	seen := map[string]bool{}
	var out []string
	for i, kind := range kinds {
		if kind != project.RemovalExternalLink {
			continue
		}
		source := project.ExternalLinkSource(&installations[i])
		if source == "" || seen[source] {
			continue
		}
		seen[source] = true
		out = append(out, displayPath(source))
	}
	return out
}

// sameFile is effective_view::same_file: compare canonicalized paths.
func sameFile(left, right string) bool {
	if resolved, err := filepath.EvalSymlinks(left); err == nil {
		left = resolved
	}
	if resolved, err := filepath.EvalSymlinks(right); err == nil {
		right = resolved
	}
	return left == right
}
