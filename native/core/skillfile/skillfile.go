// Package skillfile holds the SKILL.md helpers shared by the library and
// project packages: name validation and frontmatter parsing (the Rust
// crate keeps these in library.rs and project.rs reaches across module
// boundaries, which Go does not allow cyclically).
package skillfile

import (
	"errors"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidateName is library::validate_name.
func ValidateName(name string) error {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || name == "." || name == ".." {
		return errors.New("技能名称无效")
	}
	return nil
}

type frontmatterMeta struct {
	Name        *string `yaml:"name"`
	Description *string `yaml:"description"`
}

// ReadFrontmatter is library::read_frontmatter: returns (name,
// description), each possibly empty.
func ReadFrontmatter(path string) (string, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", nil
	}
	var fm []string
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		fm = append(fm, line)
	}
	var meta frontmatterMeta
	if err := yaml.Unmarshal([]byte(strings.Join(fm, "\n")), &meta); err == nil {
		name := ""
		if meta.Name != nil {
			name = *meta.Name
		}
		desc := ""
		if meta.Description != nil {
			desc = trimDescription(*meta.Description)
		}
		return name, desc, nil
	}
	var name, desc string
	for _, line := range fm {
		if v, ok := strings.CutPrefix(line, "name:"); ok {
			name = unquote(v)
		}
		if v, ok := strings.CutPrefix(line, "description:"); ok {
			desc = unquote(v)
		}
	}
	return name, desc, nil
}

func trimDescription(description string) string {
	return strings.TrimRight(description, " \t\r\n")
}

func unquote(value string) string {
	return strings.Trim(strings.TrimSpace(value), "'\"")
}
