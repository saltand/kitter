package fslink

import (
	"os"
	"path/filepath"
	"testing"
)

// Mirrors directory_link.rs tests (macOS/unix symlink flavor only).

func TestCreatesInspectsAndRemovesLinkWithoutTouchingTarget(t *testing.T) {
	temp := t.TempDir()
	target := filepath.Join(temp, "target")
	link := filepath.Join(temp, "link")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "canary"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Create(target, link); err != nil {
		t.Fatal(err)
	}
	inspected, err := Inspect(link)
	if err != nil || inspected == nil {
		t.Fatalf("inspect: %v %v", inspected, err)
	}
	want, _ := filepath.EvalSymlinks(target)
	got, err := filepath.EvalSymlinks(inspected.Target)
	if err != nil || got != want {
		t.Fatalf("target %s want %s", got, want)
	}

	if err := Remove(link, *inspected); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("link still exists")
	}
	data, err := os.ReadFile(filepath.Join(target, "canary"))
	if err != nil || string(data) != "keep" {
		t.Fatal("target was touched")
	}
}

func TestOrdinaryDirectoryIsNotADirectoryLink(t *testing.T) {
	temp := t.TempDir()
	dir := filepath.Join(temp, "directory")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	link, err := Inspect(dir)
	if err != nil || link != nil {
		t.Fatalf("got %v, want nil", link)
	}
}

func TestResolvesRelativeSymlinkTargetsFromLinkParent(t *testing.T) {
	temp := t.TempDir()
	parent := filepath.Join(temp, "parent")
	target := filepath.Join(temp, "target")
	os.Mkdir(parent, 0o755)
	os.Mkdir(target, 0o755)
	link := filepath.Join(parent, "link")
	if err := os.Symlink("../target", link); err != nil {
		t.Fatal(err)
	}
	inspected, err := Inspect(link)
	if err != nil || inspected == nil {
		t.Fatal("no link found")
	}
	if inspected.Target != filepath.Join(parent, "../target") {
		t.Fatalf("got %s", inspected.Target)
	}
}
