// catalog_test.go ports the catalog.rs tests plus fixed-input token
// estimate checks.
package effective

import (
	"strings"
	"testing"
)

// claude_falls_back_to_names_only_inside_budget
func TestClaudeFallsBackToNamesOnlyInsideBudget(t *testing.T) {
	entries := [][2]string{
		{"one", strings.Repeat("x", 200)},
		{"two", strings.Repeat("y", 200)},
	}
	rendered := renderClaudeListing(entries, 20)
	if rendered != "- one\n- two" {
		t.Fatalf("rendered %q", rendered)
	}
	if len(rendered) > 20 {
		t.Fatalf("over budget: %d", len(rendered))
	}
}

// codex_reports_only_entries_that_fit_the_model_visible_catalog
func TestCodexReportsOnlyEntriesThatFit(t *testing.T) {
	skills := []EffectiveSkill{}
	for _, name := range []string{"first-long-name", "second-long-name"} {
		skills = append(skills, EffectiveSkill{
			ID:          name,
			Name:        name,
			Description: strings.Repeat("description", 20),
			Path:        "/skills/" + name + "/SKILL.md",
			RootPath:    "/skills",
			PromptPath:  "/skills/" + name + "/SKILL.md",
			Scope:       ScopeUser,
			Visibility:  VisibilityAutomatic,
			Source:      SkillSource{Kind: SourceFilesystem},
		})
	}
	rendered := renderCodexListing(skills, 10)
	if rendered.IncludedCount != 0 {
		t.Fatalf("included %d", rendered.IncludedCount)
	}
	if strings.Contains(rendered.Text, "- first-long-name") {
		t.Fatal("overflow row leaked into catalog")
	}
}

// Fixed-input token-estimate checks (approx_token_count == len/4 ceil).
func TestApproxTokenCountFixedInputs(t *testing.T) {
	for input, want := range map[string]int{
		"":                          0,
		"abcd":                      1,
		"abcde":                     2,
		strings.Repeat("x", 8001):   2001,
		strings.Repeat("y", 400):     100,
		strings.Repeat("z", 399):     100,
		"<system-reminder>\n</system-reminder>": 9,
	} {
		if got := approxTokenCount(input); got != want {
			t.Fatalf("approxTokenCount(%d bytes) = %d, want %d", len(input), got, want)
		}
	}
}

// truncate_chars: multi-byte safety.
func TestTruncateChars(t *testing.T) {
	if got := truncateChars("hello", 10); got != "hello" {
		t.Fatal(got)
	}
	if got := truncateChars("hello world", 5); got != "hell…" {
		t.Fatalf("%q", got)
	}
	// 8 runes of CJK, limit 4 → 3 runes + ellipsis
	if got := truncateChars("你好世界你好世界", 4); got != "你好世…" {
		t.Fatalf("%q", got)
	}
}
