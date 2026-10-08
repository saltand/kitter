package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltand/kitter/native/core/agents"
	"github.com/saltand/kitter/native/core/fslink"
	"github.com/saltand/kitter/native/core/model"
)

// Mirrors project.rs tests (unix symlink flavor only).

type fixture struct {
	temp    string
	project string
	library string
	source  string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	temp := t.TempDir()
	f := fixture{
		temp:    temp,
		project: filepath.Join(temp, "project"),
		library: filepath.Join(temp, "library"),
	}
	f.source = filepath.Join(f.library, "example")
	if err := os.MkdirAll(f.project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(f.source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.source, "SKILL.md"), []byte("# Example"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fixture) installation(target model.InstallTarget) string {
	return filepath.Join(f.project, filepath.FromSlash(agents.TargetDirectory(target)), "example")
}

func TestGlobalInstallationUsesUserRootsAndKeepsSourceIntact(t *testing.T) {
	f := newFixture(t)
	home := filepath.Join(f.temp, "home")
	// These targets have no environment overrides.
	targets := []model.InstallTarget{model.TargetUniversal, model.TargetAntigravity, model.TargetCopilot}
	var roots []string
	for _, target := range targets {
		roots = append(roots, agents.GlobalTargetRoot(home, target))
	}
	for i := 0; i < 2; i++ {
		if err := installToRoots(f.source, "example", roots); err != nil {
			t.Fatal(err)
		}
	}
	want, _ := filepath.EvalSymlinks(f.source)
	for _, root := range roots {
		got, err := filepath.EvalSymlinks(filepath.Join(root, "example"))
		if err != nil || got != want {
			t.Fatalf("%s → %s, want %s", root, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".agent/skills")); !os.IsNotExist(err) {
		t.Fatal("unexpected .agent/skills")
	}
	if _, err := os.Stat(filepath.Join(home, ".github/skills")); !os.IsNotExist(err) {
		t.Fatal("unexpected .github/skills")
	}
	if _, err := os.Stat(filepath.Join(f.source, "SKILL.md")); err != nil {
		t.Fatal("source touched")
	}
}

func TestInstallsIdempotentlyAndUninstallsWithoutTouchingSource(t *testing.T) {
	f := newFixture(t)
	targets := []model.InstallTarget{model.TargetUniversal, model.TargetClaudeCode}

	if err := Install(f.project, f.library, "example", targets); err != nil {
		t.Fatal(err)
	}
	if err := Install(f.project, f.library, "example", targets); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(f.source)
	for _, target := range targets {
		path := f.installation(target)
		got, err := filepath.EvalSymlinks(path)
		if err != nil || got != want {
			t.Fatalf("%s → %s", path, got)
		}
		if dl, _ := fslink.Inspect(path); dl == nil {
			t.Fatalf("%s is not a link", path)
		}
	}

	if err := Uninstall(f.project, f.library, "example", targets); err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if _, err := os.Lstat(f.installation(target)); !os.IsNotExist(err) {
			t.Fatal("link still present")
		}
	}
	data, _ := os.ReadFile(filepath.Join(f.source, "SKILL.md"))
	if string(data) != "# Example" {
		t.Fatal("source touched")
	}
}

func TestListsAndRemovesEverySupportedProjectInstallTarget(t *testing.T) {
	f := newFixture(t)
	targets := agents.ProjectInstallTargets

	if err := Install(f.project, f.library, "example", targets); err != nil {
		t.Fatal(err)
	}
	listed, err := List(f.project, f.library)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("got %d skills", len(listed))
	}
	if len(listed[0].Installations) != len(targets) {
		t.Fatalf("got %d installations", len(listed[0].Installations))
	}
	for _, target := range targets {
		if _, err := os.Lstat(f.installation(target)); err != nil {
			t.Fatalf("%s missing", target)
		}
		found := false
		for _, inst := range listed[0].Installations {
			if inst.Target == target && inst.Path == f.installation(target) && inst.Managed {
				found = true
			}
		}
		if !found {
			t.Fatalf("target %s not listed as managed", target)
		}
	}
	if err := Uninstall(f.project, f.library, "example", targets); err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if _, err := os.Lstat(f.installation(target)); !os.IsNotExist(err) {
			t.Fatalf("%s still present", target)
		}
	}
}

func TestRefusesAnOccupiedDirectoryBeforeCreatingAnyLinks(t *testing.T) {
	f := newFixture(t)
	occupied := f.installation(model.TargetClaudeCode)
	if err := os.MkdirAll(occupied, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(occupied, "keep"), []byte("user data"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Install(f.project, f.library, "example",
		[]model.InstallTarget{model.TargetUniversal, model.TargetClaudeCode})
	if err == nil || !strings.Contains(err.Error(), "安装位置已被占用") {
		t.Fatalf("want occupied error, got %v", err)
	}
	if _, err := os.Lstat(f.installation(model.TargetUniversal)); !os.IsNotExist(err) {
		t.Fatal("partial install happened")
	}
	data, _ := os.ReadFile(filepath.Join(occupied, "keep"))
	if string(data) != "user data" {
		t.Fatal("user data lost")
	}
}

func TestRefusesToUninstallALinkToAnotherSource(t *testing.T) {
	f := newFixture(t)
	other := filepath.Join(f.library, "other")
	os.MkdirAll(other, 0o755)
	os.WriteFile(filepath.Join(other, "SKILL.md"), []byte("# Other"), 0o644)
	link := f.installation(model.TargetUniversal)
	os.MkdirAll(filepath.Dir(link), 0o755)
	if err := fslink.Create(other, link); err != nil {
		t.Fatal(err)
	}
	err := Uninstall(f.project, f.library, "example", []model.InstallTarget{model.TargetUniversal})
	if err == nil || !strings.Contains(err.Error(), "不会删除指向其他位置的链接") {
		t.Fatalf("want foreign-link error, got %v", err)
	}
	got, _ := filepath.EvalSymlinks(link)
	want, _ := filepath.EvalSymlinks(other)
	if got != want {
		t.Fatal("link modified")
	}
}

func TestRefusesToUninstallWhenManagedSourceIsMissing(t *testing.T) {
	f := newFixture(t)
	link := f.installation(model.TargetUniversal)
	if err := Install(f.project, f.library, "example", []model.InstallTarget{model.TargetUniversal}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(f.source); err != nil {
		t.Fatal(err)
	}
	err := Uninstall(f.project, f.library, "example", []model.InstallTarget{model.TargetUniversal})
	if err == nil || !strings.Contains(err.Error(), "无法安全验证安装链接") {
		t.Fatalf("want verification error, got %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("link removed")
	}
}

func TestRemovesAnAliasedInstallationOnlyOnce(t *testing.T) {
	f := newFixture(t)
	if err := Install(f.project, f.library, "example", []model.InstallTarget{model.TargetUniversal}); err != nil {
		t.Fatal(err)
	}
	claudeRoot := filepath.Join(f.project, ".claude")
	os.MkdirAll(claudeRoot, 0o755)
	if err := os.Symlink(filepath.Join(f.project, ".agents/skills"), filepath.Join(claudeRoot, "skills")); err != nil {
		t.Fatal(err)
	}
	listed, err := List(f.project, f.library)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed[0].Installations) != 2 {
		t.Fatalf("got %d installations", len(listed[0].Installations))
	}
	if InstallationKey(listed[0].Installations[0].Path) != InstallationKey(listed[0].Installations[1].Path) {
		t.Fatal("alias produced different keys")
	}
	var ptrs []*model.ProjectSkillInstallation
	for i := range listed[0].Installations {
		ptrs = append(ptrs, &listed[0].Installations[i])
	}
	report := RemoveProjectSkills(ptrs)
	if report.Removed != 1 || len(report.Failures) != 0 {
		t.Fatalf("report %+v", report)
	}
	if _, err := os.Lstat(f.installation(model.TargetUniversal)); !os.IsNotExist(err) {
		t.Fatal("link still present")
	}
}

func TestRemovesADirectUnmanagedInstallation(t *testing.T) {
	f := newFixture(t)
	path := f.installation(model.TargetUniversal)
	os.MkdirAll(path, 0o755)
	os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("# External"), 0o644)
	inst := &model.ProjectSkillInstallation{
		Target:  model.TargetUniversal,
		Path:    path,
		Managed: false,
	}
	report := RemoveProjectSkills([]*model.ProjectSkillInstallation{inst})
	if report.Removed != 1 || len(report.Failures) != 0 {
		t.Fatalf("report %+v", report)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("directory still present")
	}
}

func TestRefusesToRemoveAnInstallationOutsideSupportedSkillRoot(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(f.project, "external", "example")
	os.MkdirAll(path, 0o755)
	os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("# External"), 0o644)
	inst := &model.ProjectSkillInstallation{
		Target:  model.TargetUniversal,
		Path:    path,
		Managed: false,
	}
	report := RemoveProjectSkills([]*model.ProjectSkillInstallation{inst})
	if report.Removed != 0 || len(report.Failures) != 1 {
		t.Fatalf("report %+v", report)
	}
	if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
		t.Fatal("files removed")
	}
}

func TestRejectsSkillNamesThatCanEscapeTheTargetDirectory(t *testing.T) {
	f := newFixture(t)
	err := Install(f.project, f.library, "../example", []model.InstallTarget{model.TargetUniversal})
	if err == nil || !strings.Contains(err.Error(), "技能名称无效") {
		t.Fatalf("want invalid-name error, got %v", err)
	}
}
