// Command kitter is the standalone Kitter CLI — a port of
// src/bin/kitter.rs. It manages the same skill library as the desktop
// app: sources, installations, groups, tags, and update checks.
//
// Argument parsing uses only the standard library (the plan's "std
// flag + handwritten dispatch" approach): commands are dispatched by
// position, each leaf parses its flags with flag.FlagSet, and trailing
// positional args are collected manually. clap's "options before
// positionals" habit is honored by scanning args for flags first
// where the Rust CLI mixes flags with positional lists (install,
// uninstall, tag assign/unassign, adopt --source).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/saltand/kitter/native/core/adoption"
	"github.com/saltand/kitter/native/core/effective"
	"github.com/saltand/kitter/native/core/library"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/project"
	"github.com/saltand/kitter/native/core/source"
	"github.com/saltand/kitter/native/core/tags"
)

const version = "0.1.7"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		var exit *usageError
		if errors.As(err, &exit) {
			fmt.Fprintf(os.Stderr, "%s\n\n%s\n", err, exit.help)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", formatChain(err))
		os.Exit(1)
	}
}

// usageError is a clap-style argument failure (exit 2, help printed).
type usageError struct {
	msg  string
	help string
}

func (e *usageError) Error() string { return e.msg }

func usage(msg, help string) error { return &usageError{msg: msg, help: help} }

// formatChain mirrors anyhow's {error:#}: the outer message plus each
// wrapped cause joined by ": ".
func formatChain(err error) string {
	var parts []string
	for err != nil {
		parts = append(parts, err.Error())
		err = errors.Unwrap(err)
	}
	return strings.Join(parts, ": ")
}

func counted(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return usage("no subcommand given", rootHelp)
	}
	switch args[0] {
	case "--help", "-h", "help":
		fmt.Fprint(out, rootHelp)
		return nil
	case "--version", "-V", "version":
		fmt.Fprintf(out, "kitter %s\n", version)
		return nil
	}
	lib, err := library.Open()
	if err != nil {
		return err
	}
	ctx := context.Background()
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "list":
		return cmdList(lib, rest, out)
	case "show":
		return cmdShow(lib, rest, out)
	case "files":
		return cmdFiles(lib, rest, out)
	case "read":
		return cmdRead(lib, rest, out)
	case "add":
		return cmdAdd(ctx, lib, rest, out)
	case "adopt":
		return cmdAdopt(ctx, lib, rest, out)
	case "remove":
		return cmdRemove(lib, rest, out)
	case "install":
		return cmdInstall(lib, rest, out)
	case "uninstall":
		return cmdUninstall(lib, rest, out)
	case "project":
		return cmdProject(lib, rest, out)
	case "update":
		return cmdUpdate(ctx, lib, rest, out)
	case "check":
		return cmdCheck(ctx, lib, rest, out)
	case "group":
		return cmdGroup(lib, rest, out)
	case "tag":
		return cmdTag(lib, rest, out)
	case "library":
		return cmdLibrary(lib, rest, out)
	default:
		return usage(fmt.Sprintf("unrecognized subcommand %q", cmd), rootHelp)
	}
}

const rootHelp = `Manage Agent Skills across projects

Usage: kitter <COMMAND>

Commands:
  list       List skills saved in Kitter
  show       Show one skill by name or ID
  files      List files in one skill
  read       Read one file from a skill
  add        Add skills from a source
  adopt      Scan and adopt existing agent skill installations
  remove     Remove one or more skills from Kitter and their managed installations
  install    Install one or more skills into a project
  uninstall  Remove selected skill installations from a project
  project    Inspect direct and effective skills for a project
  update     Update selected skills using their recorded sources
  check      Check every skill for updates
  group      Manage skill groups
  tag        Manage skill tags
  library    Show or change where Kitter stores skills
  help       Print this message or the help of the given subcommand(s)

Options:
  -h, --help     Print help
  -V, --version  Print version
`

// argScanner splits args into flags (--name value, --flag) and
// positionals. Everything after "--" is positional. Unlike
// flag.FlagSet it tolerates options appearing after positionals, like
// clap.
type argScanner struct {
	args  []string
	flags map[string][]string
	pos   []string
	err   error
}

// scanArgs walks args. boolFlags are valueless flags; all other --name
// forms consume the next arg unless the value is attached via '='.
func scanArgs(args []string, boolFlags map[string]bool) *argScanner {
	s := &argScanner{args: args, flags: map[string][]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			s.pos = append(s.pos, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "--") {
			name, value, hasValue := a[2:], "", false
			if j := strings.IndexByte(name, '='); j >= 0 {
				name, value, hasValue = name[:j], name[j+1:], true
			}
			key := strings.ReplaceAll(name, "-", "_")
			if boolFlags[key] {
				if hasValue {
					s.err = usage(fmt.Sprintf("unexpected value for --%s", name), "")
					return s
				}
				s.flags[key] = append(s.flags[key], "true")
				continue
			}
			if !hasValue {
				if i+1 >= len(args) {
					s.err = usage(fmt.Sprintf("a value is required for --%s but none was supplied", name), "")
					return s
				}
				i++
				value = args[i]
			}
			s.flags[key] = append(s.flags[key], value)
			continue
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			s.err = usage(fmt.Sprintf("unexpected argument %q", a), "")
			return s
		}
		s.pos = append(s.pos, a)
	}
	return s
}

func (s *argScanner) has(name string) bool { return len(s.flags[name]) > 0 }

func (s *argScanner) values(name string) []string { return s.flags[name] }

func (s *argScanner) value(name string) string {
	if v := s.flags[name]; len(v) > 0 {
		return v[len(v)-1]
	}
	return ""
}

func writeJSON(out io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}

// ---- shared selectors -------------------------------------------------

func resolveSkills(lib *library.SkillLibrary, selectors []string) ([]model.SkillSummary, error) {
	seen := map[string]bool{}
	var skills []model.SkillSummary
	for _, selector := range selectors {
		skill, err := lib.ResolveSkill(selector)
		if err != nil {
			return nil, err
		}
		if !seen[skill.Record.StorageName] {
			seen[skill.Record.StorageName] = true
			skills = append(skills, *skill)
		}
	}
	return skills, nil
}

func matchingLibrarySkills(skills []model.SkillSummary, selector string) []*model.SkillSummary {
	if storage, ok := strings.CutPrefix(selector, "id:"); ok {
		var out []*model.SkillSummary
		for i := range skills {
			if skills[i].Record.StorageName == storage {
				out = append(out, &skills[i])
			}
		}
		return out
	}
	var out []*model.SkillSummary
	for i := range skills {
		if skills[i].Record.Name == selector {
			out = append(out, &skills[i])
		}
	}
	return out
}

func resolveGroup(lib *library.SkillLibrary, selector string) (*model.SkillGroup, error) {
	for _, group := range lib.Groups() {
		if group.ID == selector || strings.EqualFold(group.Name, selector) {
			g := group
			return &g, nil
		}
	}
	return nil, fmt.Errorf("没有找到分组：%s", selector)
}

func resolveTag(state *tags.TagState, selector string) (tags.TagID, error) {
	selector = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(selector), "#"))
	if id, ok := strings.CutPrefix(selector, "id:"); ok {
		parsed, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的标签 ID：%s", selector)
		}
		if state.GetTag(tags.TagID(parsed)) == nil {
			return 0, fmt.Errorf("没有找到标签：%s", selector)
		}
		return tags.TagID(parsed), nil
	}
	var matches []tags.TagID
	for _, tag := range state.TagsList() {
		if strings.EqualFold(tag.Name, selector) {
			matches = append(matches, tag.ID)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return 0, fmt.Errorf("没有找到标签：%s", selector)
	default:
		return 0, fmt.Errorf("存在多个名为 %s 的标签，请使用 list 中显示的 ID", selector)
	}
}

func tagKeys(lib *library.SkillLibrary, items []string) ([]string, error) {
	skills, err := resolveSkills(lib, items)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(skills))
	for _, skill := range skills {
		keys = append(keys, skill.Record.StorageName)
	}
	return keys, nil
}

func canonicalProject(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("项目文件夹不存在：%s", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("无法读取项目：%s", path)
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("无法读取项目：%s", path)
	}
	return abs, nil
}

func sameFile(left, right string) bool {
	l, err1 := filepath.EvalSymlinks(left)
	r, err2 := filepath.EvalSymlinks(right)
	if err1 != nil || err2 != nil {
		return false
	}
	return l == r
}

func finishBatch(out io.Writer, action, item string, succeeded int, failures []string) error {
	if len(failures) == 0 {
		fmt.Fprintf(out, "%s %s\n", action, counted(succeeded, item, item+"s"))
		return nil
	}
	if succeeded > 0 {
		fmt.Fprintf(os.Stderr, "%s %s\n", action, counted(succeeded, item, item+"s"))
	}
	return fmt.Errorf("%s failed: %s",
		counted(len(failures), "operation", "operations"), strings.Join(failures, "; "))
}

// ---- list / show ------------------------------------------------------

type skillOutput struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Origin            model.SkillOrigin `json:"origin"`
	Path              string            `json:"path"`
	Group             *string           `json:"group"`
	Tags              []string          `json:"tags"`
	InstalledProjects int               `json:"installed_projects"`
	ManualOnly        bool              `json:"manual_only"`
	UpdateAvailable   bool              `json:"update_available"`
}

func skillOutputOf(skill model.SkillSummary, groups map[string]string, state *tags.TagState) skillOutput {
	var names []string
	for _, tag := range state.AssignedTags(skill.Record.StorageName) {
		names = append(names, tag.Name)
	}
	if names == nil {
		names = []string{}
	}
	var group *string
	if skill.Record.GroupID != nil {
		if name, ok := groups[*skill.Record.GroupID]; ok {
			group = &name
		}
	}
	return skillOutput{
		ID:                "id:" + skill.Record.StorageName,
		Name:              skill.Record.Name,
		Description:       skill.Record.Description,
		Origin:            skill.Record.Origin,
		Path:              skill.Path,
		Group:             group,
		Tags:              names,
		InstalledProjects: skill.InstalledProjects,
		ManualOnly:        skill.ManualOnly,
		UpdateAvailable:   skill.Record.UpdateAvailable,
	}
}

func groupNames(lib *library.SkillLibrary) map[string]string {
	names := map[string]string{}
	for _, group := range lib.Groups() {
		names[group.ID] = group.Name
	}
	return names
}

const listHelp = `List skills saved in Kitter

Usage: kitter list [OPTIONS]

Options:
      --tag <TAG>  Only show skills carrying this tag
      --json
  -h, --help       Print help
`

func cmdList(lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"json": true, "help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, listHelp)
		return nil
	}
	if len(s.pos) > 0 {
		return usage(fmt.Sprintf("unexpected argument %q", s.pos[0]), listHelp)
	}
	skillTags, _ := tags.LoadTagStates()
	var tagID *tags.TagID
	if s.has("tag") {
		id, err := resolveTag(skillTags, s.value("tag"))
		if err != nil {
			return err
		}
		tagID = &id
	}
	groups := groupNames(lib)
	list, err := lib.List()
	if err != nil {
		return err
	}
	var skills []skillOutput
	for _, skill := range list {
		if tagID != nil && !skillTags.MatchesFilter(skill.Record.StorageName, *tagID) {
			continue
		}
		skills = append(skills, skillOutputOf(skill, groups, skillTags))
	}
	if s.has("json") {
		if skills == nil {
			skills = []skillOutput{}
		}
		return writeJSON(out, skills)
	}
	if len(skills) == 0 {
		fmt.Fprintln(out, "No skills found")
		return nil
	}
	duplicates := map[string]int{}
	for _, skill := range skills {
		duplicates[skill.Name]++
	}
	for _, skill := range skills {
		var group string
		if skill.Group != nil {
			group = "  [" + *skill.Group + "]"
		}
		var tagList string
		if len(skill.Tags) > 0 {
			var rendered []string
			for _, tag := range skill.Tags {
				rendered = append(rendered, "#"+tag)
			}
			tagList = "  " + strings.Join(rendered, " ")
		}
		var id string
		if duplicates[skill.Name] > 1 {
			id = "  " + skill.ID
		}
		fmt.Fprintf(out, "%-28s %s%s%s%s\n",
			skill.Name, skill.Description, group, tagList, id)
	}
	return nil
}

const showHelp = `Show one skill by name or ID

Usage: kitter show [OPTIONS] <SKILL>

Arguments:
  <SKILL>

Options:
      --json
  -h, --help  Print help
`

func cmdShow(lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"json": true, "help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, showHelp)
		return nil
	}
	if len(s.pos) != 1 {
		return usage("the following required argument was not provided: <SKILL>", showHelp)
	}
	skill, err := lib.ResolveSkill(s.pos[0])
	if err != nil {
		return err
	}
	skillTags, _ := tags.LoadTagStates()
	output := skillOutputOf(*skill, groupNames(lib), skillTags)
	if s.has("json") {
		return writeJSON(out, output)
	}
	fmt.Fprintln(out, output.Name)
	if output.ID != output.Name {
		fmt.Fprintf(out, "ID: %s\n", output.ID)
	}
	fmt.Fprintln(out, output.Description)
	fmt.Fprintf(out, "Source: %s\n", output.Origin.Label())
	fmt.Fprintf(out, "Path: %s\n", output.Path)
	if output.Group != nil {
		fmt.Fprintf(out, "Group: %s\n", *output.Group)
	}
	if len(output.Tags) > 0 {
		fmt.Fprintf(out, "Tags: %s\n", strings.Join(output.Tags, ", "))
	}
	return nil
}

const filesHelp = `List files in one skill

Usage: kitter files <SKILL>

Arguments:
  <SKILL>

Options:
  -h, --help  Print help
`

func cmdFiles(lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, filesHelp)
		return nil
	}
	if len(s.pos) != 1 {
		return usage("the following required argument was not provided: <SKILL>", filesHelp)
	}
	skill, err := lib.ResolveSkill(s.pos[0])
	if err != nil {
		return err
	}
	files, err := lib.FilesByStorage(skill.Record.StorageName)
	if err != nil {
		return err
	}
	for _, file := range files {
		fmt.Fprintln(out, file)
	}
	return nil
}

const readHelp = `Read one file from a skill

Usage: kitter read <SKILL> <PATH>

Arguments:
  <SKILL>
  <PATH>

Options:
  -h, --help  Print help
`

func cmdRead(lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, readHelp)
		return nil
	}
	if len(s.pos) != 2 {
		return usage("the following required arguments were not provided: <SKILL> <PATH>", readHelp)
	}
	skill, err := lib.ResolveSkill(s.pos[0])
	if err != nil {
		return err
	}
	content, err := lib.ReadFileByStorage(skill.Record.StorageName, s.pos[1])
	if err != nil {
		return err
	}
	fmt.Fprint(out, content)
	return nil
}

// ---- add / adopt --------------------------------------------------------

const addHelp = `Add skills from a source

Usage: kitter add <COMMAND>

Commands:
  local   Recursively discover skills below a local folder
  npx     Discover skills from a skills.sh or GitHub source
  claude  Discover skills from a Claude plugin
  help    Print this message or the help of the given subcommand(s)

Options:
  -h, --help  Print help
`

func cmdAdd(ctx context.Context, lib *library.SkillLibrary, args []string, out io.Writer) error {
	if len(args) == 0 {
		return usage("no subcommand given", addHelp)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "local", "npx", "claude":
	default:
		if sub == "--help" || sub == "-h" || sub == "help" {
			fmt.Fprint(out, addHelp)
			return nil
		}
		return usage(fmt.Sprintf("unrecognized subcommand %q", sub), addHelp)
	}
	s := scanArgs(rest, map[string]bool{"help": true})
	if s.err != nil {
		return s.err
	}
	help := fmt.Sprintf(`Usage: kitter add %s <%s> [OPTIONS]

Options:
      --skill <SKILL>  Import only these discovered skill names; defaults to all
      --group <GROUP>
  -h, --help           Print help
`, sub, strings.ToUpper(sub[:1])+sub[1:])
	if s.has("help") {
		fmt.Fprint(out, help)
		return nil
	}
	if len(s.pos) != 1 {
		return usage(fmt.Sprintf("the following required argument was not provided: <%s>", strings.ToUpper(sub)), help)
	}
	var scan *source.SkillScan
	var err error
	switch sub {
	case "local":
		path, cErr := filepath.EvalSymlinks(s.pos[0])
		if cErr != nil {
			return fmt.Errorf("找不到来源目录：%s", s.pos[0])
		}
		scan, err = source.ScanLocal(path)
	case "npx":
		scan, err = source.ScanNpx(ctx, s.pos[0])
	case "claude":
		scan, err = source.ScanClaude(ctx, s.pos[0])
	}
	if err != nil {
		return err
	}
	available := map[string]bool{}
	for _, skill := range scan.Skills() {
		available[skill.Name] = true
	}
	selected := map[string]bool{}
	requested := s.values("skill")
	if len(requested) == 0 {
		for name := range available {
			selected[name] = true
		}
	} else {
		var missing []string
		for _, name := range requested {
			if !available[name] {
				missing = append(missing, name)
			}
			selected[name] = true
		}
		if len(missing) > 0 {
			return fmt.Errorf("没有从来源中找到：%s", strings.Join(missing, ", "))
		}
	}
	summary, err := scan.ImportSelected(lib, selected, s.value("group"))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Added %s, skipped %s already added\n",
		counted(summary.Added, "skill", "skills"),
		counted(summary.Skipped, "skill", "skills"))
	return nil
}

type adoptionOutput struct {
	Name           string                 `json:"name"`
	Source         string                 `json:"source"`
	Origin         model.SkillOrigin      `json:"origin"`
	References     []model.SkillReference `json:"references"`
	Issue          *string                `json:"issue"`
	Conflict       bool                   `json:"conflict"`
	AlreadyManaged bool                   `json:"already_managed"`
	Selectable     bool                   `json:"selectable"`
}

const adoptHelp = `Scan and adopt existing agent skill installations

Usage: kitter adopt [OPTIONS] [ROOTS]...

Arguments:
  [ROOTS]...  Folders to scan; defaults to the home folder

Options:
      --source <SOURCES>  Adopt these exact source folders from the scan
      --all               Adopt every unambiguous candidate
      --json
  -h, --help              Print help
`

func cmdAdopt(ctx context.Context, lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"all": true, "json": true, "help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, adoptHelp)
		return nil
	}
	if s.has("all") && s.has("source") {
		return usage("the argument '--all' cannot be used with '--source'", adoptHelp)
	}
	home := source.HomeDir()
	if home == "" {
		return fmt.Errorf("找不到用户目录")
	}
	roots := s.pos
	if len(roots) == 0 {
		roots = []string{home}
	}
	managed, err := lib.List()
	if err != nil {
		return err
	}
	scan, err := adoption.ScanRoots(ctx, home, roots, lib.Config.LibraryDir, managed)
	if err != nil {
		return err
	}
	selectable := scan.SelectableIDs()
	var output []adoptionOutput
	for _, candidate := range scan.Candidates {
		var issue *string
		if candidate.Issue != "" {
			i := candidate.Issue
			issue = &i
		}
		refs := candidate.References
		if refs == nil {
			refs = []model.SkillReference{}
		}
		output = append(output, adoptionOutput{
			Name:           candidate.Name,
			Source:         candidate.Source,
			Origin:         candidate.Origin,
			References:     refs,
			Issue:          issue,
			Conflict:       scan.HasConflict(candidate.Identity()),
			AlreadyManaged: candidate.ExistingStorage != "",
			Selectable:     selectable[candidate.ID],
		})
	}

	if !s.has("all") && !s.has("source") {
		if s.has("json") {
			if output == nil {
				output = []adoptionOutput{}
			}
			return writeJSON(out, output)
		}
		if len(output) == 0 {
			fmt.Fprintln(out, "No existing skill installations found")
			return nil
		}
		for _, candidate := range output {
			var status string
			switch {
			case candidate.Issue != nil:
				status = "unavailable"
			case candidate.Conflict:
				status = "choose-source"
			case candidate.AlreadyManaged:
				status = "managed"
			default:
				status = "ready"
			}
			fmt.Fprintf(out, "%-24s %-14s %s\n", candidate.Name, status, candidate.Source)
		}
		fmt.Fprintln(out, "Run with --all or one or more --source paths to adopt")
		return nil
	}

	selected := map[string]bool{}
	if s.has("all") {
		selected = scan.DefaultSelection()
	}
	for _, sourceArg := range s.values("source") {
		resolved, err := filepath.EvalSymlinks(sourceArg)
		if err != nil {
			resolved = sourceArg
		}
		var candidate *adoption.AdoptionCandidate
		for _, c := range scan.Candidates {
			if c.Source == resolved || c.Source == sourceArg {
				candidate = c
				break
			}
		}
		if candidate == nil {
			return fmt.Errorf("扫描结果中没有这个来源：%s", resolved)
		}
		if candidate.Issue != "" {
			return fmt.Errorf("无法托管 %s：%s", candidate.Source, candidate.Issue)
		}
		scan.Select(selected, candidate.ID)
	}
	if len(selected) == 0 {
		return fmt.Errorf("没有可托管的 skill；冲突来源需要使用 --source 明确选择")
	}

	adopted := 0
	var failures []string
	for _, candidate := range scan.Candidates {
		if !selected[candidate.ID] {
			continue
		}
		ok := true
		for _, variant := range scan.Variants(candidate) {
			if err := variant.Verify(); err != nil {
				failures = append(failures, fmt.Sprintf("%s：%s", candidate.Name, formatChain(err)))
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if _, err := lib.Adopt(candidate, scan.ReferencesFor(candidate)); err != nil {
			failures = append(failures, fmt.Sprintf("%s：%s", candidate.Name, formatChain(err)))
			continue
		}
		adopted++
	}
	return finishBatch(out, "Adopted", "skill", adopted, failures)
}

// ---- remove / install / uninstall --------------------------------------

const removeHelp = `Remove one or more skills from Kitter and their managed installations

Usage: kitter remove <SKILLS>...

Arguments:
  <SKILLS>...

Options:
  -h, --help  Print help
`

func cmdRemove(lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, removeHelp)
		return nil
	}
	if len(s.pos) == 0 {
		return usage("the following required argument was not provided: <SKILLS>", removeHelp)
	}
	skills, err := resolveSkills(lib, s.pos)
	if err != nil {
		return err
	}
	removed := 0
	var failures []string
	for _, skill := range skills {
		if err := lib.RemoveByStorage(skill.Record.StorageName); err != nil {
			failures = append(failures, fmt.Sprintf("%s：%s", skill.Record.Name, formatChain(err)))
			continue
		}
		removed++
	}
	return finishBatch(out, "Removed", "skill", removed, failures)
}

var targetNames = map[string]model.InstallTarget{
	"universal":   model.TargetUniversal,
	"codex":       model.TargetCodex,
	"claude":      model.TargetClaudeCode,
	"cursor":      model.TargetCursor,
	"opencode":    model.TargetOpenCode,
	"pi":          model.TargetPi,
	"grok":        model.TargetGrok,
	"antigravity": model.TargetAntigravity,
	"droid":       model.TargetDroid,
	"copilot":     model.TargetCopilot,
}

var agentNames = map[string]effective.AgentKind{
	"codex":       effective.AgentCodex,
	"claude":      effective.AgentClaudeCode,
	"cursor":      effective.AgentCursor,
	"opencode":    effective.AgentOpenCode,
	"copilot":     effective.AgentCopilot,
	"antigravity": effective.AgentAntigravity,
	"amp":         effective.AgentAmp,
	"droid":       effective.AgentDroid,
	"pi":          effective.AgentPi,
	"grok":        effective.AgentGrok,
	"openclaw":    effective.AgentOpenClaw,
	"hermes":      effective.AgentHermes,
}

func parseTargets(values []string) ([]model.InstallTarget, error) {
	var targets []model.InstallTarget
	for _, value := range values {
		target, ok := targetNames[value]
		if !ok {
			return nil, usage(fmt.Sprintf("invalid value %q for --target", value), "")
		}
		targets = append(targets, target)
	}
	if targets == nil {
		targets = []model.InstallTarget{}
	}
	return targets, nil
}

const installHelp = `Install one or more skills into a project

Usage: kitter install [OPTIONS] --project <PROJECT> --target <TARGET> <SKILLS>...

Arguments:
  <SKILLS>...

Options:
      --project <PROJECT>
      --target <TARGET>    [possible values: universal, codex, claude, cursor, opencode, pi, grok, antigravity, droid, copilot]
  -h, --help               Print help
`

func cmdInstall(lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, installHelp)
		return nil
	}
	if len(s.pos) == 0 {
		return usage("the following required argument was not provided: <SKILLS>", installHelp)
	}
	if !s.has("project") {
		return usage("the following required argument was not provided: --project <PROJECT>", installHelp)
	}
	if !s.has("target") {
		return usage("the following required argument was not provided: --target <TARGET>", installHelp)
	}
	projectPath, err := canonicalProject(s.value("project"))
	if err != nil {
		return err
	}
	skills, err := resolveSkills(lib, s.pos)
	if err != nil {
		return err
	}
	targets, err := parseTargets(s.values("target"))
	if err != nil {
		return err
	}
	installed := 0
	var failures []string
	for _, skill := range skills {
		if err := project.InstallFromPath(projectPath, skill.Path, skill.Record.Name, targets); err != nil {
			failures = append(failures, fmt.Sprintf("%s：%s", skill.Record.Name, formatChain(err)))
			continue
		}
		installed++
	}
	return finishBatch(out, "Installed", "skill", installed, failures)
}

const uninstallHelp = `Remove selected skill installations from a project

Usage: kitter uninstall [OPTIONS] --project <PROJECT> [SKILLS]...

Arguments:
  [SKILLS]...

Options:
      --path <PATHS>               Remove these exact installation paths
      --project <PROJECT>
      --target <TARGET>            Limit removal to these targets; defaults to every installed target
      --include-unmanaged          Also remove external links or directly stored skill folders
  -h, --help                       Print help
`

func cmdUninstall(lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"include_unmanaged": true, "help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, uninstallHelp)
		return nil
	}
	if !s.has("project") {
		return usage("the following required argument was not provided: --project <PROJECT>", uninstallHelp)
	}
	if len(s.pos) == 0 && !s.has("path") {
		return fmt.Errorf("请指定至少一个 skill 或 --path 安装位置")
	}
	projectPath, err := canonicalProject(s.value("project"))
	if err != nil {
		return err
	}
	installed, err := project.List(projectPath, lib.Config.LibraryDir)
	if err != nil {
		return err
	}
	librarySkills, err := lib.List()
	if err != nil {
		return err
	}
	targets, err := parseTargets(s.values("target"))
	if err != nil {
		return err
	}
	targetSet := map[model.InstallTarget]bool{}
	for _, t := range targets {
		targetSet[t] = true
	}
	targetAllowed := func(t model.InstallTarget) bool {
		return len(targetSet) == 0 || targetSet[t]
	}
	var selected []*model.ProjectSkillInstallation
	for _, requested := range s.values("path") {
		path := requested
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectPath, path)
		}
		key := project.InstallationKey(path)
		var found *model.ProjectSkillInstallation
		for i := range installed {
			for j := range installed[i].Installations {
				inst := &installed[i].Installations[j]
				if targetAllowed(inst.Target) && project.InstallationKey(inst.Path) == key {
					found = inst
				}
			}
		}
		if found == nil {
			return fmt.Errorf("这个项目中没有安装位置：%s", path)
		}
		selected = append(selected, found)
	}
	for _, selector := range s.pos {
		matches := matchingLibrarySkills(librarySkills, selector)
		if len(matches) > 1 {
			return fmt.Errorf("存在多个名为 %s 的 skill，请使用 list 中的 ID", selector)
		}
		var matched []*model.ProjectSkillInstallation
		for i := range installed {
			skill := &installed[i]
			if len(matches) > 0 {
				if skill.Name != matches[0].Record.Name {
					continue
				}
			} else if skill.Name != selector {
				continue
			}
			for j := range skill.Installations {
				inst := &skill.Installations[j]
				if !targetAllowed(inst.Target) {
					continue
				}
				if len(matches) > 0 && !sameFile(inst.Path, matches[0].Path) {
					continue
				}
				matched = append(matched, inst)
			}
		}
		if len(matched) == 0 {
			return fmt.Errorf("这个项目中没有匹配的安装：%s", selector)
		}
		selected = append(selected, matched...)
	}
	var unsafePaths []string
	for _, inst := range selected {
		if !inst.Managed {
			unsafePaths = append(unsafePaths, inst.Path)
		}
	}
	if len(unsafePaths) > 0 && !s.has("include_unmanaged") {
		return fmt.Errorf("所选位置包含外部链接或直接保存的文件；确认后使用 --include-unmanaged：%s",
			strings.Join(unsafePaths, ", "))
	}
	report := project.RemoveProjectSkills(selected)
	return finishBatch(out, "Removed", "installation", report.Removed, report.Failures)
}

// ---- project ------------------------------------------------------------

type projectOutput struct {
	Path                string                           `json:"path"`
	DirectInstallations []model.ProjectSkill             `json:"direct_installations"`
	Agents              []effective.AgentContextEstimate `json:"agents"`
}

const projectHelp = `Inspect direct and effective skills for a project

Usage: kitter project [OPTIONS] <PATH>

Arguments:
  <PATH>

Options:
      --agent <AGENT>  Limit effective discovery to one agent [possible values: codex, claude, cursor, opencode, copilot, antigravity, amp, droid, pi, grok, openclaw, hermes]
      --view <VIEW>    Select filesystem skills, plugin skills, or both [default: all] [possible values: all, skills, plugins]
      --json
  -h, --help           Print help
`

func cmdProject(lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"json": true, "help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, projectHelp)
		return nil
	}
	if len(s.pos) != 1 {
		return usage("the following required argument was not provided: <PATH>", projectHelp)
	}
	view := s.value("view")
	if view == "" {
		view = "all"
	}
	if view != "all" && view != "skills" && view != "plugins" {
		return usage(fmt.Sprintf("invalid value %q for --view", view), projectHelp)
	}
	var selected *effective.AgentKind
	if s.has("agent") {
		kind, ok := agentNames[s.value("agent")]
		if !ok {
			return usage(fmt.Sprintf("invalid value %q for --agent", s.value("agent")), projectHelp)
		}
		selected = &kind
	}
	path, err := canonicalProject(s.pos[0])
	if err != nil {
		return err
	}
	direct, err := project.List(path, lib.Config.LibraryDir)
	if err != nil {
		return err
	}
	var estimates []effective.AgentContextEstimate
	for _, estimate := range effective.EstimateProject(path) {
		if selected != nil && estimate.Agent != *selected {
			continue
		}
		var kept []effective.EffectiveSkill
		for _, skill := range estimate.Skills {
			switch view {
			case "skills":
				if skill.IsPlugin() {
					continue
				}
			case "plugins":
				if !skill.IsPlugin() {
					continue
				}
			}
			kept = append(kept, skill)
		}
		estimate.Skills = kept
		estimates = append(estimates, estimate)
	}
	if s.has("json") {
		if direct == nil {
			direct = []model.ProjectSkill{}
		}
		if estimates == nil {
			estimates = []effective.AgentContextEstimate{}
		}
		return writeJSON(out, projectOutput{
			Path:                path,
			DirectInstallations: direct,
			Agents:              estimates,
		})
	}

	fmt.Fprintf(out, "Project: %s\n", path)
	fmt.Fprintf(out, "Direct installations: %d\n", len(direct))
	for _, skill := range direct {
		var names []string
		allManaged := len(skill.Installations) > 0
		for _, inst := range skill.Installations {
			names = append(names, string(inst.Target))
			if !inst.Managed {
				allManaged = false
			}
		}
		origin := "external"
		if allManaged {
			origin = "Kitter"
		}
		fmt.Fprintf(out, "  %-28s %-12s %s\n", skill.Name, origin, strings.Join(names, ", "))
	}
	fmt.Fprintln(out, "Agents:")
	for _, estimate := range estimates {
		fmt.Fprintf(out, "  %-18s %3d discovered  %3d visible  ~%d tokens\n",
			estimate.Agent.Label(), estimate.DiscoveredCount,
			estimate.ModelVisibleCount, estimate.EstimatedTokens)
	}
	var entries []effective.GroupEntry
	for _, estimate := range estimates {
		for i := range estimate.Skills {
			entries = append(entries, effective.GroupEntry{
				Agent: estimate.Agent,
				Skill: &estimate.Skills[i],
			})
		}
	}
	grouped := effective.GroupEffectiveSkills(entries)
	fmt.Fprintf(out, "Effective skills: %d\n", len(grouped))
	for _, group := range grouped {
		agentSet := map[string]bool{}
		var agentList []string
		plugin := false
		for _, entry := range group.Entries {
			if !agentSet[entry.Agent.Label()] {
				agentSet[entry.Agent.Label()] = true
				agentList = append(agentList, entry.Agent.Label())
			}
			if entry.Skill.IsPlugin() {
				plugin = true
			}
		}
		sort.Strings(agentList)
		kind := "skill"
		if plugin {
			kind = "plugin"
		}
		fmt.Fprintf(out, "  %-28s %-7s %s\n", group.Name(), kind, strings.Join(agentList, ", "))
	}
	return nil
}

// ---- update / check -----------------------------------------------------

const updateHelp = `Update selected skills using their recorded sources

Usage: kitter update [OPTIONS] [SKILLS]...

Arguments:
  [SKILLS]...

Options:
      --all     Update every skill with a managed source
  -h, --help    Print help
`

func cmdUpdate(ctx context.Context, lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"all": true, "help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, updateHelp)
		return nil
	}
	if s.has("all") && len(s.pos) > 0 {
		return usage("the argument '--all' cannot be used with '[SKILLS]'", updateHelp)
	}
	if len(s.pos) == 0 && !s.has("all") {
		return fmt.Errorf("请指定至少一个 skill，或使用 --all")
	}
	var skills []model.SkillSummary
	var err error
	if s.has("all") {
		skills, err = lib.List()
	} else {
		skills, err = resolveSkills(lib, s.pos)
	}
	if err != nil {
		return err
	}
	updated := 0
	var failures []string
	for _, skill := range skills {
		if err := source.UpdateByStorage(ctx, lib, skill.Record.StorageName); err != nil {
			failures = append(failures, fmt.Sprintf("%s：%s", skill.Record.Name, formatChain(err)))
			continue
		}
		updated++
	}
	return finishBatch(out, "Updated", "skill", updated, failures)
}

const checkHelp = `Check every skill for updates

Usage: kitter check [OPTIONS]

Options:
      --json
  -h, --help  Print help
`

func cmdCheck(ctx context.Context, lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"json": true, "help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, checkHelp)
		return nil
	}
	count, err := source.CheckUpdates(ctx, lib)
	if err != nil {
		return err
	}
	if s.has("json") {
		fmt.Fprintf(out, "{\"updates\":%d}\n", count)
	} else {
		fmt.Fprintf(out, "%s can be updated\n", counted(count, "skill", "skills"))
	}
	return nil
}

// ---- group ----------------------------------------------------------------

type groupOutput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Skills int    `json:"skills"`
}

const groupHelp = `Manage skill groups

Usage: kitter group <COMMAND>

Commands:
  list    List groups and their skill counts
  create  Create a group
  rename  Rename a group
  delete  Delete a group
  assign  Move skills into a group
  clear   Remove skills from their groups
  help    Print this message or the help of the given subcommand(s)

Options:
  -h, --help  Print help
`

func cmdGroup(lib *library.SkillLibrary, args []string, out io.Writer) error {
	if len(args) == 0 {
		return usage("no subcommand given", groupHelp)
	}
	sub, rest := args[0], args[1:]
	boolFlags := map[string]bool{"json": true, "delete_skills": true, "help": true}
	s := scanArgs(rest, boolFlags)
	if s.err != nil {
		return s.err
	}
	if s.has("help") || sub == "help" || sub == "--help" || sub == "-h" {
		fmt.Fprint(out, groupHelp)
		return nil
	}
	switch sub {
	case "list":
		skills, err := lib.List()
		if err != nil {
			return err
		}
		var groups []groupOutput
		for _, group := range lib.Groups() {
			count := 0
			for _, skill := range skills {
				if skill.Record.GroupID != nil && *skill.Record.GroupID == group.ID {
					count++
				}
			}
			groups = append(groups, groupOutput{ID: group.ID, Name: group.Name, Skills: count})
		}
		if s.has("json") {
			if groups == nil {
				groups = []groupOutput{}
			}
			return writeJSON(out, groups)
		}
		if len(groups) == 0 {
			fmt.Fprintln(out, "No groups")
			return nil
		}
		for _, group := range groups {
			fmt.Fprintf(out, "%-28s %s\n", group.Name, counted(group.Skills, "skill", "skills"))
		}
		return nil
	case "create":
		if len(s.pos) != 1 {
			return usage("the following required argument was not provided: <NAME>", groupHelp)
		}
		_, err := lib.CreateGroup(s.pos[0])
		return err
	case "rename":
		if len(s.pos) != 2 {
			return usage("the following required arguments were not provided: <GROUP> <NAME>", groupHelp)
		}
		group, err := resolveGroup(lib, s.pos[0])
		if err != nil {
			return err
		}
		return lib.RenameGroup(group.ID, s.pos[1])
	case "delete":
		if len(s.pos) != 1 {
			return usage("the following required argument was not provided: <GROUP>", groupHelp)
		}
		group, err := resolveGroup(lib, s.pos[0])
		if err != nil {
			return err
		}
		_, err = lib.DeleteGroup(group.ID, s.has("delete_skills"))
		return err
	case "assign":
		if len(s.pos) < 2 {
			return usage("the following required arguments were not provided: <GROUP> <SKILLS>...", groupHelp)
		}
		group, err := resolveGroup(lib, s.pos[0])
		if err != nil {
			return err
		}
		skills, err := resolveSkills(lib, s.pos[1:])
		if err != nil {
			return err
		}
		for _, skill := range skills {
			if err := lib.AssignGroupByStorage(skill.Record.StorageName, &group.ID); err != nil {
				return err
			}
		}
		return nil
	case "clear":
		if len(s.pos) == 0 {
			return usage("the following required argument was not provided: <SKILLS>", groupHelp)
		}
		skills, err := resolveSkills(lib, s.pos)
		if err != nil {
			return err
		}
		for _, skill := range skills {
			if err := lib.AssignGroupByStorage(skill.Record.StorageName, nil); err != nil {
				return err
			}
		}
		return nil
	default:
		return usage(fmt.Sprintf("unrecognized subcommand %q", sub), groupHelp)
	}
}

// ---- tag -------------------------------------------------------------------

type tagOutput struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Parent      *string `json:"parent"`
	Assignments int     `json:"assignments"`
}

const tagHelp = `Manage skill tags

Usage: kitter tag <COMMAND>

Commands:
  list      List tags and assignment counts
  create    Create a root or child tag
  rename    Rename a tag
  delete    Delete a tag and its child tags
  assign    Assign a tag to one or more skills
  unassign  Remove a tag from one or more skills
  move      Reorder a tag among its siblings
  help      Print this message or the help of the given subcommand(s)

Options:
  -h, --help  Print help
`

func cmdTag(lib *library.SkillLibrary, args []string, out io.Writer) error {
	if len(args) == 0 {
		return usage("no subcommand given", tagHelp)
	}
	sub, rest := args[0], args[1:]
	s := scanArgs(rest, map[string]bool{"json": true, "help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") || sub == "help" || sub == "--help" || sub == "-h" {
		fmt.Fprint(out, tagHelp)
		return nil
	}
	skillTags, projectTags := tags.LoadTagStates()
	state := skillTags
	switch sub {
	case "list":
		var output []tagOutput
		for _, root := range state.Roots() {
			output = append(output, tagOutput{
				ID: fmt.Sprintf("id:%d", root.ID), Name: root.Name,
				Assignments: state.Count(root.ID),
			})
			for _, child := range state.Children(root.ID) {
				name := root.Name
				output = append(output, tagOutput{
					ID: fmt.Sprintf("id:%d", child.ID), Name: child.Name,
					Parent: &name, Assignments: state.Count(child.ID),
				})
			}
		}
		if s.has("json") {
			if output == nil {
				output = []tagOutput{}
			}
			return writeJSON(out, output)
		}
		if len(output) == 0 {
			fmt.Fprintln(out, "No tags")
			return nil
		}
		duplicates := map[string]int{}
		for _, tag := range output {
			duplicates[strings.ToLower(tag.Name)]++
		}
		for _, tag := range output {
			indent := ""
			if tag.Parent != nil {
				indent = "  "
			}
			var id string
			if duplicates[strings.ToLower(tag.Name)] > 1 {
				id = "  " + tag.ID
			}
			fmt.Fprintf(out, "%s#%-26s %s%s\n", indent, tag.Name,
				counted(tag.Assignments, "assignment", "assignments"), id)
		}
		return nil
	case "create":
		if len(s.pos) != 1 {
			return usage("the following required argument was not provided: <NAME>", tagHelp)
		}
		var parent *tags.TagID
		if s.has("parent") {
			id, err := resolveTag(state, s.value("parent"))
			if err != nil {
				return err
			}
			parent = &id
		}
		if _, err := state.Add(s.pos[0], parent); err != nil {
			return err
		}
	case "rename":
		if len(s.pos) != 2 {
			return usage("the following required arguments were not provided: <TAG> <NAME>", tagHelp)
		}
		id, err := resolveTag(state, s.pos[0])
		if err != nil {
			return err
		}
		if err := state.Rename(id, s.pos[1]); err != nil {
			return err
		}
	case "delete":
		if len(s.pos) != 1 {
			return usage("the following required argument was not provided: <TAG>", tagHelp)
		}
		id, err := resolveTag(state, s.pos[0])
		if err != nil {
			return err
		}
		state.Delete(id)
	case "assign":
		if len(s.pos) < 2 {
			return usage("the following required arguments were not provided: <TAG> <ITEMS>...", tagHelp)
		}
		id, err := resolveTag(state, s.pos[0])
		if err != nil {
			return err
		}
		keys, err := tagKeys(lib, s.pos[1:])
		if err != nil {
			return err
		}
		for _, key := range keys {
			state.SetAssignment(key, id, true)
		}
	case "unassign":
		if len(s.pos) < 2 {
			return usage("the following required arguments were not provided: <TAG> <ITEMS>...", tagHelp)
		}
		id, err := resolveTag(state, s.pos[0])
		if err != nil {
			return err
		}
		keys, err := tagKeys(lib, s.pos[1:])
		if err != nil {
			return err
		}
		for _, key := range keys {
			state.SetAssignment(key, id, false)
		}
	case "move":
		if len(s.pos) != 1 {
			return usage("the following required argument was not provided: <TAG>", tagHelp)
		}
		if s.has("before") == s.has("after") {
			return usage("one of --before or --after is required", tagHelp)
		}
		id, err := resolveTag(state, s.pos[0])
		if err != nil {
			return err
		}
		var moved bool
		if s.has("before") {
			target, err := resolveTag(state, s.value("before"))
			if err != nil {
				return err
			}
			moved = state.MoveBefore(id, target)
		} else {
			target, err := resolveTag(state, s.value("after"))
			if err != nil {
				return err
			}
			moved = state.MoveAfter(id, target)
		}
		if !moved {
			return fmt.Errorf("只能在同一级标签之间调整顺序")
		}
	default:
		return usage(fmt.Sprintf("unrecognized subcommand %q", sub), tagHelp)
	}
	return tags.SaveTagStates(skillTags, projectTags)
}

// ---- library --------------------------------------------------------------

const libraryHelp = `Show or change where Kitter stores skills

Usage: kitter library [OPTIONS]

Options:
      --set <PATH>  Change the skill library folder
  -h, --help        Print help
`

func cmdLibrary(lib *library.SkillLibrary, args []string, out io.Writer) error {
	s := scanArgs(args, map[string]bool{"help": true})
	if s.err != nil {
		return s.err
	}
	if s.has("help") {
		fmt.Fprint(out, libraryHelp)
		return nil
	}
	if s.has("set") {
		path := s.value("set")
		if !filepath.IsAbs(path) {
			return fmt.Errorf("请使用绝对路径")
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
		lib.Config.LibraryDir = path
		if err := lib.Save(); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, lib.Config.LibraryDir)
	return nil
}
