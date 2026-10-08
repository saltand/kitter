// Package fslink mirrors src/directory_link.rs: directory links are plain
// symbolic links on macOS and Linux.
package fslink

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// DirectoryLinkKind mirrors DirectoryLinkKind; only SymbolicLink exists
// off Windows.
type DirectoryLinkKind int

const (
	SymbolicLink DirectoryLinkKind = iota
)

// DirectoryLink describes an inspected link.
type DirectoryLink struct {
	Kind   DirectoryLinkKind
	Target string
}

// Inspect returns the link at path, or nil when path is missing or a
// regular file/directory.
func Inspect(path string) (*DirectoryLink, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		raw, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}
		return &DirectoryLink{Kind: SymbolicLink, Target: resolveTarget(path, raw)}, nil
	}
	return nil, nil
}

// Create links link → target.
func Create(target, link string) error {
	return os.Symlink(target, link)
}

// Remove deletes the link itself, never its target.
func Remove(path string, link DirectoryLink) error {
	return os.Remove(path)
}

func resolveTarget(link, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(filepath.Dir(link), target)
}
