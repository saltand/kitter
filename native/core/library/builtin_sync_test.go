package library

import (
	"os"
	"path/filepath"
	"testing"
)

// The embedded builtin/kitter tree must stay identical to the canonical
// resources/skills/kitter at the repository root. Run
// scripts/sync_builtin.sh to update the copy.
func TestBuiltinMatchesRepositorySource(t *testing.T) {
	src := filepath.Join("..", "..", "..", "resources", "skills", "kitter")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("repository source not available: %v", err)
	}
	var srcFiles []string
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(src, path)
			srcFiles = append(srcFiles, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var dstFiles []string
	err = filepath.WalkDir("builtin/kitter", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel("builtin/kitter", path)
			dstFiles = append(dstFiles, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(srcFiles) != len(dstFiles) {
		t.Fatalf("file count differs: %v vs %v (run scripts/sync_builtin.sh)", srcFiles, dstFiles)
	}
	for i, rel := range srcFiles {
		if rel != dstFiles[i] {
			t.Fatalf("file lists differ: %q vs %q", srcFiles, dstFiles)
		}
		s, err := os.ReadFile(filepath.Join(src, rel))
		if err != nil {
			t.Fatal(err)
		}
		d, err := builtinFS.ReadFile(filepath.ToSlash(filepath.Join("builtin/kitter", rel)))
		if err != nil {
			t.Fatal(err)
		}
		if string(s) != string(d) {
			t.Fatalf("%s differs from resources/skills/kitter/%s (run scripts/sync_builtin.sh)", rel, rel)
		}
	}
}
