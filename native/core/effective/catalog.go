// catalog.go ports src/effective_skills/catalog.rs: the per-agent
// initial catalog renderers (Codex, Grok, Claude) plus shared helpers
// (truncate_chars, names_only, system-reminder wrapper).
package effective

import (
	"fmt"
	"sort"
	"strings"
)

// CatalogRender mirrors catalog::CatalogRender.
type CatalogRender struct {
	Text          string
	IncludedCount int
}

const codexIntro = "A skill is a set of instructions provided through a `SKILL.md` source. Below is the list of skills that can be used. Each entry includes a name, description, and source locator. `file` locators are on the host filesystem, `executor package` locators are owned by their execution environment, `orchestrator package` locators are opaque package identifiers, and `custom resource` locators use their provider's access mechanism."
const codexUsage = `- Discovery: The list above is the skills available in this session (name + description + source locator). ` + "`file`" + ` entries live on the host filesystem, ` + "`executor package`" + ` and ` + "`orchestrator package`" + ` entries are accessed directly through ` + "`skills.read`" + `, and ` + "`custom resource`" + ` entries use their provider's access mechanism.
- Trigger rules: If the user names a skill (with ` + "`$SkillName`" + ` or plain text) OR the task clearly matches a skill's description shown above, you must use that skill for that turn. Multiple mentions mean use them all. Do not carry skills across turns unless re-mentioned.
- Missing/blocked: If a named skill isn't in the list or its source can't be read, say so briefly and continue with the best fallback.
- How to use a skill (progressive disclosure):
  1) After deciding to use a skill, the main agent must read its ` + "`SKILL.md`" + ` completely before taking task actions. For a ` + "`file`" + ` entry, open the listed path. For an ` + "`executor package`" + ` or ` + "`orchestrator package`" + `, pass the listed locator directly to ` + "`skills.read`" + ` as ` + "`package`" + `; root aliases are resolved automatically. Omit ` + "`resource`" + ` to read ` + "`SKILL.md`" + ` directly without calling ` + "`skills.list`" + `. If a read is paginated, follow ` + "`next_cursor`" + ` until EOF.
  2) When ` + "`SKILL.md`" + ` references another resource, use the same access mechanism. For executor and orchestrator skills, pass the complete package-contained resource identifier with the same package to ` + "`skills.read`" + `; do not treat ` + "`skill://`" + ` identifiers as filesystem paths.
  3) If ` + "`SKILL.md`" + ` points to extra folders such as ` + "`references/`" + `, use its routing instructions to identify the resources required for the task. The main agent must read each required instruction or reference file itself before acting on it. Do not delegate reading, summarizing, or interpreting skill instructions to a subagent. Subagents may still perform task work when the selected skill allows it.
  4) For filesystem-backed skills, prefer running or patching provided scripts instead of retyping large code blocks. For executor and orchestrator skills, use ` + "`skills.read`" + ` and the available tools; do not invent a local path.
  5) Reuse provided assets or templates through the same source access mechanism instead of recreating them.
- Coordination and sequencing:
  - If multiple skills apply, choose the minimal set that covers the request and state the order you'll use them.
  - Announce which skill(s) you're using and why (one short line). If you skip an obvious skill, say why.
- Context hygiene:
  - Progressive disclosure applies to selecting relevant files, not partially reading a selected instruction file. Do not load unrelated references, scripts, or assets.
  - Avoid deep reference-chasing: prefer opening only files directly linked from ` + "`SKILL.md`" + ` unless you're blocked.
  - When variants exist (frameworks, providers, domains), pick only the relevant reference file(s) and note that choice.
- Safety and fallback: If a skill can't be applied cleanly (missing files, unclear instructions), state the issue, pick the next-best approach, and continue.`

// renderCodexListing is catalog::render_codex_listing.
func renderCodexListing(skills []EffectiveSkill, budget int) CatalogRender {
	if len(skills) == 0 {
		return CatalogRender{}
	}
	sorted := append([]EffectiveSkill(nil), skills...)
	sort.Slice(sorted, func(i, j int) bool {
		li, lj := codexScopeOrder(sorted[i].Scope), codexScopeOrder(sorted[j].Scope)
		if li != lj {
			return li < lj
		}
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Path < sorted[j].Path
	})
	render := func(skill *EffectiveSkill, description string) string {
		return fmt.Sprintf("- %s: %s (file: %s)", skill.Name, description, skill.PromptPath)
	}
	var fullRows []string
	for i := range sorted {
		fullRows = append(fullRows, render(&sorted[i], truncateChars(sorted[i].Description, 1024)))
	}
	full := strings.Join(fullRows, "\n")
	var rows string
	includedCount := len(sorted)
	if len(full) <= budget {
		rows = full
	} else {
		overhead := 0
		for i := range sorted {
			overhead += len(render(&sorted[i], "")) + 1
		}
		perDescription := (budget - overhead)
		if perDescription < 0 {
			perDescription = 0
		}
		perDescription /= maxInt(len(sorted), 1)
		if perDescription >= 20 {
			var shortened []string
			for i := range sorted {
				shortened = append(shortened, render(&sorted[i], truncateChars(sorted[i].Description, perDescription)))
			}
			rows = strings.Join(shortened, "\n")
		} else {
			var names []string
			for i := range sorted {
				names = append(names, sorted[i].Name)
			}
			rows, includedCount = namesOnly(names, budget)
		}
	}
	return CatalogRender{
		Text: "<skills_instructions>\n## Skills\n" + codexIntro + "\n### Available skills\n" + rows + "\n### How to use skills\n" + codexUsage + "\n</skills_instructions>",
		IncludedCount: includedCount,
	}
}

func codexScopeOrder(scope SkillScope) int {
	switch scope {
	case ScopeSystem:
		return 0
	case ScopeRepository, ScopeLocal:
		return 1
	default:
		return 2
	}
}

// renderGrokListing is catalog::render_grok_listing.
func renderGrokListing(skills []EffectiveSkill, budget int) CatalogRender {
	if len(skills) == 0 {
		return CatalogRender{}
	}
	header := "The following skills are available for use:\n\n"
	render := func(skill *EffectiveSkill, combinedBudget int) string {
		whenToUse := ""
		if skill.HasWhenToUse {
			whenToUse = skill.WhenToUse
		}
		var descriptionBudget, whenBudget int
		if whenToUse == "" {
			descriptionBudget = combinedBudget
			whenBudget = 0
		} else {
			dLen := maxInt(len(skill.Description), 1)
			wLen := maxInt(len(whenToUse), 1)
			descriptionBudget = combinedBudget * dLen / (dLen + wLen)
			whenBudget = combinedBudget - descriptionBudget
			if whenBudget < 0 {
				whenBudget = 0
			}
		}
		description := truncateChars(skill.Description, descriptionBudget)
		path := skill.PromptPath
		if whenToUse == "" {
			return fmt.Sprintf("- %s: %s\n  Absolute path: %s", skill.Name, description, path)
		}
		return fmt.Sprintf("- %s: %s\n  Use when: %s\n  Absolute path: %s",
			skill.Name, description, truncateChars(whenToUse, whenBudget), path)
	}
	var fullRows []string
	for i := range skills {
		fullRows = append(fullRows, render(&skills[i], 400))
	}
	full := strings.Join(fullRows, "\n")
	if len(header)+len(full) <= budget {
		return CatalogRender{
			Text:          wrapSystemReminder(header + full),
			IncludedCount: len(skills),
		}
	}
	perEntry := budget - len(header)
	if perEntry < 0 {
		perEntry = 0
	}
	perEntry /= maxInt(len(skills), 1)
	if perEntry >= 20 {
		var shortened []string
		for i := range skills {
			n := perEntry - 2
			if n < 0 {
				n = 0
			}
			shortened = append(shortened, render(&skills[i], n))
		}
		return CatalogRender{
			Text:          wrapSystemReminder(header + strings.Join(shortened, "\n")),
			IncludedCount: len(skills),
		}
	}
	var names []string
	for i := range skills {
		names = append(names, skills[i].Name)
	}
	rows, included := namesOnly(names, budget-len(header))
	return CatalogRender{
		Text:          wrapSystemReminder(strings.TrimRight(header+rows, " \t\n")),
		IncludedCount: included,
	}
}

func wrapSystemReminder(content string) string {
	return "<system-reminder>\n" + content + "\n</system-reminder>"
}

// renderClaudeListing is catalog::render_claude_listing.
func renderClaudeListing(entries [][2]string, budget int) string {
	render := func(name, description string) string {
		if description == "" {
			return "- " + name
		}
		return fmt.Sprintf("- %s: %s", name, description)
	}
	var full []string
	for _, e := range entries {
		full = append(full, render(e[0], e[1]))
	}
	total := 0
	for _, row := range full {
		total += len(row)
	}
	if total+maxInt(len(full)-1, 0) <= budget {
		return strings.Join(full, "\n")
	}
	overhead := 0
	for _, e := range entries {
		overhead += len(e[0]) + 4
	}
	overhead += maxInt(len(entries)-1, 0)
	descriptionBudget := budget - overhead
	if descriptionBudget < 0 {
		descriptionBudget = 0
	}
	descriptionBudget /= maxInt(len(entries), 1)
	var rows []string
	for _, e := range entries {
		if descriptionBudget < 20 || e[1] == "" {
			rows = append(rows, "- "+e[0])
		} else {
			rows = append(rows, render(e[0], truncateChars(e[1], descriptionBudget)))
		}
	}
	return strings.Join(rows, "\n")
}

// namesOnly is catalog::names_only.
func namesOnly(names []string, budget int) (string, int) {
	var rows []string
	used := 0
	for _, name := range names {
		row := "- " + name
		next := len(row)
		if len(rows) > 0 {
			next++
		}
		if used+next > budget {
			break
		}
		used += next
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n"), len(rows)
}

// truncateChars is catalog/effective_skills::truncate_chars.
func truncateChars(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	n := limit - 1
	if n < 0 {
		n = 0
	}
	return string(runes[:n]) + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
