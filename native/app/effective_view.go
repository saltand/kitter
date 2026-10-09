// effective_view.go ports src/ui/effective_view.rs: plugin_groups and
// skill_rows — pure data shaping for the project effective-skills view.
package app

import (
	"path/filepath"
	"sort"

	"github.com/saltand/kitter/native/core/agents"
	"github.com/saltand/kitter/native/core/effective"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/project"
)

// EffectivePluginGroup is effective_view::EffectivePluginGroup.
type EffectivePluginGroup struct {
	Key         string
	Agent       effective.AgentKind
	DisplayName string
	Skills      []string
}

// pluginGroups is effective_view::plugin_groups.
func pluginGroups(estimates []effective.AgentContextEstimate, selectedAgent *effective.AgentKind) []EffectivePluginGroup {
	groups := map[string]*EffectivePluginGroup{}
	var order []string
	for _, estimate := range estimates {
		if selectedAgent != nil && *selectedAgent != estimate.Agent {
			continue
		}
		for _, skill := range estimate.PluginSkills() {
			id, ok := skill.Source.PluginID()
			if !ok {
				continue
			}
			display := id
			if name, ok := skill.Source.PluginDisplayName(); ok {
				display = name
			}
			key := estimate.Agent.ID() + ":" + id
			group, ok := groups[key]
			if !ok {
				group = &EffectivePluginGroup{Key: key, Agent: estimate.Agent, DisplayName: display}
				groups[key] = group
				order = append(order, key)
			}
			if !containsString(group.Skills, skill.Name) {
				group.Skills = append(group.Skills, skill.Name)
			}
		}
	}
	sort.Strings(order)
	out := make([]EffectivePluginGroup, 0, len(order))
	for _, key := range order {
		g := groups[key]
		sort.Strings(g.Skills)
		out = append(out, *g)
	}
	return out
}

// EffectiveSkillRow is effective_view::EffectiveSkillRow.
type EffectiveSkillRow struct {
	Name                string
	Description         string
	Locations           []string
	BuiltIn             bool
	Agents              []effective.AgentKind
	ManualOnly          bool
	DirectInstallations []model.ProjectSkillInstallation
}

// skillRows is effective_view::skill_rows.
func skillRows(estimates []effective.AgentContextEstimate, projectSkills []model.ProjectSkill, selectedAgent *effective.AgentKind) []EffectiveSkillRow {
	var entries []effective.GroupEntry
	for i := range estimates {
		estimate := &estimates[i]
		if selectedAgent != nil && *selectedAgent != estimate.Agent {
			continue
		}
		for j := range estimate.Skills {
			skill := &estimate.Skills[j]
			if skill.IsPlugin() {
				continue
			}
			entries = append(entries, effective.GroupEntry{Agent: estimate.Agent, Skill: skill})
		}
	}
	groups := effective.GroupEffectiveSkills(entries)
	rows := make([]EffectiveSkillRow, 0, len(groups))
	for _, group := range groups {
		if len(group.Entries) == 0 {
			continue
		}
		first := group.Entries[0].Skill
		row := EffectiveSkillRow{Name: first.Name, Description: first.Description}
		for _, entry := range group.Entries {
			skill := entry.Skill
			if row.Description == "" && skill.Description != "" {
				row.Description = skill.Description
			}
			if skill.RootPath != "" && !containsString(row.Locations, skill.RootPath) {
				row.Locations = append(row.Locations, skill.RootPath)
			}
			if skill.Source.Kind == effective.SourceBuiltin {
				row.BuiltIn = true
			}
			if !agentKindContains(row.Agents, entry.Agent) {
				row.Agents = append(row.Agents, entry.Agent)
			}
			if skill.Visibility == effective.VisibilityManualOnly {
				row.ManualOnly = true
			}
			for _, installed := range projectSkills {
				if installed.Name != skill.Name {
					continue
				}
				for _, installation := range installed.Installations {
					if !sameFile(filepath.Join(installation.Path, "SKILL.md"), skill.Path) {
						continue
					}
					dup := false
					for _, existing := range row.DirectInstallations {
						if project.InstallationKey(existing.Path) == project.InstallationKey(installation.Path) {
							dup = true
							break
						}
					}
					if !dup {
						row.DirectInstallations = append(row.DirectInstallations, installation)
					}
				}
			}
		}
		rows = append(rows, row)
	}
	for i := range rows {
		sort.Slice(rows[i].Locations, func(x, y int) bool {
			return displayPath(rows[i].Locations[x]) < displayPath(rows[i].Locations[y])
		})
		sort.Slice(rows[i].Agents, func(x, y int) bool {
			return agentIconOrder(rows[i].Agents[x]) < agentIconOrder(rows[i].Agents[y])
		})
	}
	return rows
}

func agentKindContains(agents []effective.AgentKind, a effective.AgentKind) bool {
	for _, x := range agents {
		if x == a {
			return true
		}
	}
	return false
}

// agentIconOrder is the AGENT_ICON_ORDER position used for badge
// ordering in skill rows.
func agentIconOrder(agent effective.AgentKind) int {
	for i, info := range agents.AgentIconOrder {
		if info.ID == agent.ID() {
			return i
		}
	}
	return len(agents.AgentIconOrder)
}

// displayEffectiveRoot is display_effective_root.
func displayEffectiveRoot(path, project string) string {
	if rel, err := filepath.Rel(project, path); err == nil && rel != "." && rel != "" && rel[0] != '.' {
		return rel
	}
	return displayPath(path)
}
