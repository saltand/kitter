// scanner_test.go ports the scanner.rs tests plus coverage for the
// gitignore-aware Pi profile.
package effective

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDirectChildrenDoNotRecurse(t *testing.T) {
	temp := t.TempDir()
	writeFile(t, filepath.Join(temp, "direct/SKILL.md"), "---\n---")
	writeFile(t, filepath.Join(temp, "group/nested/SKILL.md"), "---\n---")
	root := NewSkillRoot(temp, ScopeUser)
	files := scan(root, DirectChildrenProfile())
	want := []string{filepath.Join(temp, "direct/SKILL.md")}
	if len(files) != 1 || files[0] != want[0] {
		t.Fatalf("got %v want %v", files, want)
	}
}

func TestPiScanHonorsIgnoreFiles(t *testing.T) {
	temp := t.TempDir()
	writeFile(t, filepath.Join(temp, "kept/SKILL.md"), "---\n---")
	writeFile(t, filepath.Join(temp, "ignored/SKILL.md"), "---\n---")
	writeFile(t, filepath.Join(temp, ".ignore"), "ignored/\n")
	root := NewSkillRoot(temp, ScopeUser)
	files := scan(root, PiIgnoredProfile())
	want := []string{filepath.Join(temp, "kept/SKILL.md")}
	if len(files) != 1 || files[0] != want[0] {
		t.Fatalf("got %v want %v", files, want)
	}
}
