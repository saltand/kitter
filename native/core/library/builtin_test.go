package library

import (
	"io/fs"
	"strings"
	"testing"
)

// The embedded builtin/kitter tree is the single source of truth for
// the kitter skill (the repository-root copy was removed in the Rust
// cleanup). Guard that the embed is complete and parses.
func TestBuiltinEmbedIsComplete(t *testing.T) {
	var files []string
	err := fs.WalkDir(builtinFS, "builtin/kitter", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"builtin/kitter/SKILL.md",
		"builtin/kitter/references/install-cli.md",
	} {
		found := false
		for _, f := range files {
			if f == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("embedded builtin missing %s (have %v)", want, files)
		}
	}

	content, err := builtinFS.ReadFile("builtin/kitter/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.HasPrefix(text, "---") ||
		!strings.Contains(text, "name: kitter") {
		t.Fatal("builtin SKILL.md frontmatter missing name: kitter")
	}
	if !strings.Contains(text, "kitter list") ||
		!strings.Contains(text, "kitter install") {
		t.Fatal("SKILL.md lost its CLI examples")
	}
}
