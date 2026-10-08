package effective

import (
	"os"
	"path/filepath"
	"testing"
)

// Mirrors the effective_skills.rs tests that cover the M1-exported
// helpers; the full adapter/portfolio tests land with M4.

func writeSkill(t *testing.T, root, name, extra string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: Test\n" + extra + "---\nBody"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// codex_openai_policy_marks_manual_only
func TestCodexOpenaiPolicyMarksManualOnly(t *testing.T) {
	temp := t.TempDir()
	skill := writeSkill(t, temp, "demo", "")
	if err := os.MkdirAll(filepath.Join(skill, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(skill, "agents", "openai.yaml"),
		[]byte("policy:\n  allow_implicit_invocation: false\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if !IsManualSkill(skill) {
		t.Fatal("expected manual-only via openai.yaml")
	}
	if HasDisableModelInvocation(skill) {
		t.Fatal("frontmatter has no disable-model-invocation")
	}
}

// frontmatter_flag_is_detected_independently_of_codex_policy
func TestFrontmatterFlagDetectedIndependentlyOfCodexPolicy(t *testing.T) {
	temp := t.TempDir()
	writeSkill(t, temp, "manual", "disable-model-invocation: true\n")
	if !HasDisableModelInvocation(filepath.Join(temp, "manual")) {
		t.Fatal("frontmatter flag not detected")
	}
	writeSkill(t, temp, "automatic", "")
	if HasDisableModelInvocation(filepath.Join(temp, "automatic")) {
		t.Fatal("unexpected manual detection")
	}
}

// parse_visibility: paths → conditional, flag → manual.
func TestParseVisibility(t *testing.T) {
	if got := ParseVisibility("no frontmatter"); got != VisibilityAutomatic {
		t.Fatal(got)
	}
	if got := ParseVisibility("---\nname: x\ndisable-model-invocation: true\n---\n"); got != VisibilityManualOnly {
		t.Fatal(got)
	}
	if got := ParseVisibility("---\nname: x\npaths:\n  - src/**\n---\n"); got != VisibilityConditional {
		t.Fatal(got)
	}
	if got := ParseVisibility("---\nname: x\npaths: []\n---\n"); got != VisibilityAutomatic {
		t.Fatal(got)
	}
}
