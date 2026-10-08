// adapters.go ports src/effective_skills/adapters.rs: the inspect entry
// point that runs each agent's scan+metadata profile, and the public
// estimate_project(_with_home).
package effective

import "os"

// inspectProject is adapters::inspect_project.
func inspectProject(ctx *DiscoveryContext, includeGlobalOnly bool) []AgentContextEstimate {
	estimates := []AgentContextEstimate{
		estimateWithProfile(newCodexAdapter(), ctx,
			RecursiveProfile(6, 2000, 20000), ProfileStrictFrontmatter),
		estimateWithProfile(newClaudeCodeAdapter(), ctx,
			DirectChildrenProfile(), ProfileBodyFallback),
		estimateWithPolicy(newCursorPolicy(), ctx),
		estimateWithPolicy(newOpenCodePolicy(), ctx),
		estimateWithPolicy(newCopilotPolicy(), ctx),
		estimateWithPolicy(newAntigravityPolicy(), ctx),
		estimateWithPolicy(newAmpPolicy(), ctx),
		estimateWithPolicy(newDroidPolicy(), ctx),
		estimateWithProfile(newPiAdapter(), ctx,
			PiIgnoredProfile(), ProfileStrictFrontmatter),
		inspectGrok(ctx),
	}
	if includeGlobalOnly {
		estimates = append(estimates,
			estimateWithPolicy(newOpenClawPolicy(), ctx),
			estimateWithPolicy(newHermesPolicy(), ctx),
		)
	}
	return estimates
}

// inspectGrok is GrokAdapter::inspect — Recursive(5) + BodyFallback +
// collision rekeying.
func inspectGrok(ctx *DiscoveryContext) AgentContextEstimate {
	skills := discover(newGrokAdapter(), ctx,
		RecursiveProfile(5, maxIntValue, maxIntValue), ProfileBodyFallback)
	return estimateSkills(newGrokAdapter(), resolveGrokCollisions(skills))
}

// EstimateProject is estimate_project.
func EstimateProject(project string) []AgentContextEstimate {
	home := project
	if h, err := homeDirEffective(); err == nil && h != "" {
		home = h
	}
	return EstimateProjectWithHome(project, home)
}

// EstimateProjectWithHome is estimate_project_with_home.
func EstimateProjectWithHome(project, home string) []AgentContextEstimate {
	ctx := &DiscoveryContext{
		Cwd:            project,
		Home:           home,
		RepositoryRoot: findRepositoryRoot(project),
	}
	return inspectProject(ctx, project == home)
}

// homeDirEffective is dirs::home_dir (overridable in tests).
var homeDirEffective = os.UserHomeDir
