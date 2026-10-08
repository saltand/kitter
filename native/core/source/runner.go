// runner.go abstracts external tool invocations (npx, git, claude) so
// tests inject fakes and never touch the network. Rust's source.rs uses
// std::process::Command directly and, for its npx fixture test, re-execs
// the test binary via env::current_exe. The Go port replaces that with
// an in-process fake Runner because the Kitter CLI does not exist until
// M6 — there is no stable subcommand to re-exec.
package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Command is a prepared external invocation. Fields mirror
// exec.Cmd's inputs the port needs.
type Command struct {
	Program string
	Args    []string
	WorkDir string
	Env     []string // extra KEY=VALUE pairs appended to the environment
}

// Dir sets the working directory and returns the command for chaining.
func (c Command) Dir(dir string) Command {
	c.WorkDir = dir
	return c
}

// Runner executes a Command. It is the seam tests replace: assign a fake
// before exercising scans/updates and restore it afterwards.
var Runner = func(ctx context.Context, c Command) error {
	cmd := exec.CommandContext(ctx, c.Program, c.Args...)
	cmd.Dir = c.WorkDir
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	out, err := cmd.CombinedOutput()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("无法启动 %s，请确认已经安装并可在终端中使用", c.Program)
		}
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("命令执行失败：%s", detail)
	}
	return nil
}

// Run executes c through Runner.
func Run(ctx context.Context, c Command) error {
	return Runner(ctx, c)
}

// RunOutput executes c through the same PATH-augmented lookup as Run but
// returns stdout, like `git config --get`. Tests override OutputRunner.
var OutputRunner = func(ctx context.Context, c Command) (string, error) {
	cmd := exec.CommandContext(ctx, c.Program, c.Args...)
	cmd.Dir = c.WorkDir
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	out, err := cmd.Output()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// RunOutput invokes OutputRunner.
func RunOutput(ctx context.Context, c Command) (string, error) {
	return OutputRunner(ctx, c)
}

// ToolCommand builds a Command for an external tool, found like the
// Rust find_tool: PATH first, then well-known install locations
// (Homebrew, /usr/local, ~/.local/bin, nvm). When the tool lives outside
// PATH, its bin dir is prepended to the command's PATH so the tool's own
// child lookups still work.
func ToolCommand(name string, args ...string) Command {
	executable := FindTool(name)
	if executable == "" {
		executable = name
	}
	var env []string
	if dir := filepath.Dir(executable); dir != "." && dir != "" {
		if current := os.Getenv("PATH"); current != "" {
			env = append(env, "PATH="+dir+string(os.PathListSeparator)+current)
		} else {
			env = append(env, "PATH="+dir)
		}
	}
	return Command{Program: executable, Args: args, Env: env}
}

// FindTool is find_tool.
func FindTool(name string) string {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		candidate := filepath.Join(dir, name)
		if isExecutable(candidate) {
			return candidate
		}
	}
	var candidates []string
	if runtime.GOOS == "windows" {
		for _, variable := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			if base := os.Getenv(variable); base != "" {
				candidates = append(candidates,
					filepath.Join(base, "nodejs", name),
					filepath.Join(base, "Git", "cmd", name))
			}
		}
		if appData := os.Getenv("APPDATA"); appData != "" {
			candidates = append(candidates, filepath.Join(appData, "npm", name))
		}
	} else {
		candidates = append(candidates,
			filepath.Join("/opt/homebrew/bin", name),
			filepath.Join("/usr/local/bin", name),
			filepath.Join("/usr/bin", name))
	}
	if home := HomeDir(); home != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", name))
		if entries, err := os.ReadDir(filepath.Join(home, ".nvm", "versions", "node")); err == nil {
			for _, entry := range entries {
				candidates = append(candidates, filepath.Join(home, ".nvm", "versions", "node", entry.Name(), "bin", name))
			}
		}
	}
	best := ""
	var bestMod int64
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if mod := info.ModTime().UnixNano(); best == "" || mod > bestMod {
			best, bestMod = candidate, mod
		}
	}
	return best
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

// toolTimeout bounds a single external call so a hung npx/git cannot
// wedge a scan forever (Rust relies on the CLIs' own timeouts).
const toolTimeout = 10 * time.Minute

// WithTimeout derives a bounded context for a tool invocation.
func WithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, toolTimeout)
}
