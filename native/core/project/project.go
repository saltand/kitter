// Package project mirrors src/project.rs: linked installation of library
// skills into per-project (or per-user) agent directories.
package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/saltand/kitter/native/core/agents"
	"github.com/saltand/kitter/native/core/fslink"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/skillfile"
)

// RemovalKind mirrors project::RemovalKind.
type RemovalKind int

const (
	RemovalManagedInstallation RemovalKind = iota
	RemovalExternalLink                    // Source holds the resolved link target
	RemovalSourceFiles
)

// RemovalReport mirrors project::RemovalReport.
type RemovalReport struct {
	Removed  int
	Failures []string
}

// Install is project::install.
func Install(project, libraryDir, name string, targets []model.InstallTarget) error {
	if err := skillfile.ValidateName(name); err != nil {
		return err
	}
	if info, err := os.Stat(project); err != nil || !info.IsDir() {
		return fmt.Errorf("项目文件夹不存在：%s", project)
	}
	source, err := filepath.EvalSymlinks(filepath.Join(libraryDir, name))
	if err != nil {
		return fmt.Errorf("技能不存在: %w", err)
	}
	return InstallFromPath(project, source, name, targets)
}

// InstallFromPath is project::install_from_path.
func InstallFromPath(project, source, name string, targets []model.InstallTarget) error {
	if err := skillfile.ValidateName(name); err != nil {
		return err
	}
	if info, err := os.Stat(project); err != nil || !info.IsDir() {
		return fmt.Errorf("项目文件夹不存在：%s", project)
	}
	var roots []string
	for _, t := range targets {
		roots = append(roots, agents.InstallationRoot(project, t))
	}
	return installToRoots(source, name, roots)
}

func installToRoots(source, name string, roots []string) error {
	if err := skillfile.ValidateName(name); err != nil {
		return err
	}
	source, err := filepath.EvalSymlinks(source)
	if err != nil {
		return fmt.Errorf("技能不存在: %w", err)
	}
	var pending [][2]string
	for _, parent := range roots {
		link := filepath.Join(parent, name)
		if _, err := os.Lstat(link); err == nil {
			if resolved, err := filepath.EvalSymlinks(link); err == nil && resolved == source {
				continue
			}
			return fmt.Errorf("安装位置已被占用：%s", link)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("无法检查安装位置：%s: %w", link, err)
		}
		dup := false
		for _, p := range pending {
			if InstallationKey(p[1]) == InstallationKey(link) {
				dup = true
				break
			}
		}
		if !dup {
			pending = append(pending, [2]string{parent, link})
		}
	}
	var created []string
	for _, p := range pending {
		parent, link := p[0], p[1]
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("创建安装链接失败：%s：%w%s", link, err, rollbackSuffix(rollbackLinks(created)))
		}
		if err := fslink.Create(source, link); err != nil {
			return fmt.Errorf("创建安装链接失败：%s：%w%s", link, err, rollbackSuffix(rollbackLinks(created)))
		}
		created = append(created, link)
	}
	return nil
}

func rollbackSuffix(failures []string) string {
	if len(failures) == 0 {
		return ""
	}
	return "；回滚失败：" + strings.Join(failures, "；")
}

// Uninstall is project::uninstall.
func Uninstall(project, libraryDir, name string, targets []model.InstallTarget) error {
	if err := skillfile.ValidateName(name); err != nil {
		return err
	}
	source, _ := filepath.EvalSymlinks(filepath.Join(libraryDir, name))
	var removable []struct {
		link string
		dl   *fslink.DirectoryLink
	}
	removableKeys := map[string]bool{}
	for _, target := range targets {
		link := filepath.Join(agents.InstallationRoot(project, target), name)
		if _, err := os.Lstat(link); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("无法检查安装位置：%s: %w", link, err)
		}
		dl, err := fslink.Inspect(link)
		if err != nil {
			return err
		}
		if dl == nil {
			return fmt.Errorf("不会删除非 Kitter 管理的目录：%s", link)
		}
		if source == "" {
			return fmt.Errorf("技能不存在，无法安全验证安装链接：%s", link)
		}
		if resolved, err := filepath.EvalSymlinks(link); err != nil || resolved != source {
			return fmt.Errorf("不会删除指向其他位置的链接：%s", link)
		}
		key := InstallationKey(link)
		if !removableKeys[key] {
			removableKeys[key] = true
			removable = append(removable, struct {
				link string
				dl   *fslink.DirectoryLink
			}{link, dl})
		}
	}
	for _, r := range removable {
		if err := fslink.Remove(r.link, *r.dl); err != nil {
			return err
		}
	}
	return nil
}

// UninstallAll is project::uninstall_all.
func UninstallAll(project, name, libraryDir string) error {
	if err := skillfile.ValidateName(name); err != nil {
		return err
	}
	source, _ := filepath.EvalSymlinks(filepath.Join(libraryDir, name))
	return uninstallAllFromSource(project, name, source)
}

// UninstallAllFromPath is project::uninstall_all_from_path.
func UninstallAllFromPath(project, name, source string) error {
	if err := skillfile.ValidateName(name); err != nil {
		return err
	}
	resolved, _ := filepath.EvalSymlinks(source)
	return uninstallAllFromSource(project, name, resolved)
}

func uninstallAllFromSource(project, name, source string) error {
	for _, target := range agents.ProjectInstallTargets {
		link := filepath.Join(agents.InstallationRoot(project, target), name)
		dl, err := fslink.Inspect(link)
		if err != nil {
			return err
		}
		if dl == nil {
			continue
		}
		if source != "" {
			if resolved, err := filepath.EvalSymlinks(link); err == nil && resolved == source {
				if err := fslink.Remove(link, *dl); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// RemovalKindOf is project::removal_kind.
func RemovalKindOf(installation *model.ProjectSkillInstallation) RemovalKind {
	if installation.Managed {
		return RemovalManagedInstallation
	}
	dl, err := fslink.Inspect(installation.Path)
	if err != nil || dl == nil {
		return RemovalSourceFiles
	}
	if resolved, err := filepath.EvalSymlinks(dl.Target); err == nil {
		_ = resolved // ExternalLink{source} uses the resolved target
	}
	return RemovalExternalLink
}

// ExternalLinkSource resolves the target for RemovalExternalLink.
func ExternalLinkSource(installation *model.ProjectSkillInstallation) string {
	dl, err := fslink.Inspect(installation.Path)
	if err != nil || dl == nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(dl.Target); err == nil {
		return resolved
	}
	return dl.Target
}

func isSupportedInstallationPath(installation *model.ProjectSkillInstallation) bool {
	parent := filepath.Dir(installation.Path)
	if strings.HasSuffix(parent, filepath.FromSlash(agents.TargetDirectory(installation.Target))) {
		return true
	}
	if home, err := os.UserHomeDir(); err == nil {
		return parent == agents.GlobalTargetRoot(home, installation.Target)
	}
	return false
}

// RemoveProjectSkill is project::remove_project_skill.
func RemoveProjectSkill(installation *model.ProjectSkillInstallation) error {
	if !isSupportedInstallationPath(installation) {
		return fmt.Errorf("不会删除 skills 目标目录之外的技能：%s", installation.Path)
	}
	info, err := os.Lstat(installation.Path)
	if err != nil {
		return fmt.Errorf("找不到：%s: %w", installation.Path, err)
	}
	if dl, err := fslink.Inspect(installation.Path); err != nil {
		return err
	} else if dl != nil {
		return fslink.Remove(installation.Path, *dl)
	} else if info.Mode().IsRegular() {
		return os.Remove(installation.Path)
	} else if info.IsDir() {
		return os.RemoveAll(installation.Path)
	}
	return errors.New("无法删除这个技能")
}

// InstallationKey is project::installation_key: the physical entry, with
// the parent directory resolved but the link itself not.
func InstallationKey(path string) string {
	parent := filepath.Dir(path)
	if resolved, err := filepath.EvalSymlinks(parent); err == nil {
		parent = resolved
	}
	base := filepath.Base(path)
	if base == "." || base == "/" {
		return parent
	}
	return filepath.Join(parent, base)
}

func rollbackLinks(links []string) []string {
	var failures []string
	for i := len(links) - 1; i >= 0; i-- {
		dl, err := fslink.Inspect(links[i])
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s：%s", links[i], err))
			continue
		}
		if dl == nil {
			continue
		}
		if err := fslink.Remove(links[i], *dl); err != nil {
			failures = append(failures, fmt.Sprintf("%s：%s", links[i], err))
		}
	}
	return failures
}

// RemoveProjectSkills is project::remove_project_skills.
func RemoveProjectSkills(installations []*model.ProjectSkillInstallation) RemovalReport {
	var report RemovalReport
	removedKeys := map[string]bool{}
	for _, installation := range installations {
		key := InstallationKey(installation.Path)
		if removedKeys[key] {
			continue
		}
		removedKeys[key] = true
		if _, err := os.Lstat(installation.Path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				report.Failures = append(report.Failures, fmt.Sprintf("找不到：%s", installation.Path))
			} else {
				report.Failures = append(report.Failures, fmt.Sprintf("%s：%s", installation.Path, err))
			}
			continue
		}
		if err := RemoveProjectSkill(installation); err != nil {
			report.Failures = append(report.Failures, fmt.Sprintf("%s：%s", installation.Path, err))
		} else {
			report.Removed++
		}
	}
	return report
}

// List is project::list.
func List(project, libraryDir string) ([]model.ProjectSkill, error) {
	grouped := map[string][]model.ProjectSkillInstallation{}
	var order []string
	canonicalLibrary, err := filepath.EvalSymlinks(libraryDir)
	if err != nil {
		canonicalLibrary = libraryDir
	}
	linkedSources := map[string]bool{}
	if entries, err := os.ReadDir(libraryDir); err == nil {
		for _, entry := range entries {
			path := filepath.Join(libraryDir, entry.Name())
			dl, err := fslink.Inspect(path)
			if err != nil || dl == nil {
				continue
			}
			if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
				continue
			}
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				linkedSources[resolved] = true
			}
		}
	}
	for _, target := range agents.ProjectInstallTargets {
		parent := agents.InstallationRoot(project, target)
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			path := filepath.Join(parent, entry.Name())
			if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
				continue
			}
			managed := false
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				if resolved == canonicalLibrary || strings.HasPrefix(resolved, canonicalLibrary+string(os.PathSeparator)) {
					managed = true
				} else if linkedSources[resolved] {
					if dl, err := fslink.Inspect(path); err == nil && dl != nil {
						managed = true
					}
				}
			}
			name, _, err := skillfile.ReadFrontmatter(filepath.Join(path, "SKILL.md"))
			if err != nil || name == "" {
				name = entry.Name()
			}
			if _, ok := grouped[name]; !ok {
				order = append(order, name)
			}
			grouped[name] = append(grouped[name], model.ProjectSkillInstallation{
				Target:  target,
				Path:    path,
				Managed: managed,
			})
		}
	}
	sort.Strings(order)
	out := make([]model.ProjectSkill, 0, len(order))
	for _, name := range order {
		out = append(out, model.ProjectSkill{Name: name, Installations: grouped[name]})
	}
	return out, nil
}

// IsInstalledAny is project::is_installed_any.
func IsInstalledAny(project, name, libraryDir string) bool {
	return IsInstalledAnyFromPath(project, name, filepath.Join(libraryDir, name))
}

// IsInstalledAnyFromPath is project::is_installed_any_from_path.
func IsInstalledAnyFromPath(project, name, source string) bool {
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		return false
	}
	for _, target := range agents.ProjectInstallTargets {
		parent := agents.InstallationRoot(project, target)
		path := filepath.Join(parent, name)
		if r, err := filepath.EvalSymlinks(path); err == nil && r == resolved {
			return true
		}
		if entries, err := os.ReadDir(parent); err == nil {
			for _, entry := range entries {
				if r, err := filepath.EvalSymlinks(filepath.Join(parent, entry.Name())); err == nil && r == resolved {
					return true
				}
			}
		}
	}
	return false
}
