package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Tool defines an executable tool.
type Tool struct {
	Name        string
	Description string
	Execute     func(ctx context.Context, args map[string]any) (string, error)
}

// Registry stores available tools.
type Registry struct {
	tools   map[string]*Tool
	options Options
}

// PermissionMode controls access outside the workspace root.
type PermissionMode string

const (
	PermissionDeny  PermissionMode = "deny"
	PermissionAsk   PermissionMode = "ask"
	PermissionAllow PermissionMode = "allow"
)

// Options configures built-in tool behavior.
type Options struct {
	PermissionMode PermissionMode
}

// NewRegistry creates a tool registry.
func NewRegistry(workspace string) *Registry {
	return NewRegistryWithOptions(workspace, Options{PermissionMode: PermissionDeny})
}

// NewRegistryWithOptions creates a tool registry with explicit permission settings.
func NewRegistryWithOptions(workspace string, options Options) *Registry {
	if options.PermissionMode == "" {
		options.PermissionMode = PermissionDeny
	}
	r := &Registry{tools: make(map[string]*Tool), options: options}

	// Register built-in tools.
	r.register(readFile(workspace, options))
	r.register(writeFile(workspace, options))
	r.register(shell(workspace, options))
	r.register(grep(workspace))
	r.register(listDir(workspace, options))

	return r
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (*Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all tool names.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Definitions returns OpenAI function-calling tool definitions.
func (r *Registry) Definitions() []map[string]any {
	return r.DefinitionsFor(nil)
}

// DefinitionsFor returns OpenAI function calling definitions filtered by an
// optional allow-list. Empty allow-list means all tools are exposed.
func (r *Registry) DefinitionsFor(allowed map[string]bool) []map[string]any {
	defs := make([]map[string]any, 0, len(r.tools))
	for name, t := range r.tools {
		if len(allowed) > 0 && !allowed[name] {
			continue
		}
		defs = append(defs, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": t.Description,
			},
		})
	}
	return defs
}

func (r *Registry) register(t *Tool) {
	r.tools[t.Name] = t
}

// Built-in tools.

func readFile(ws string, options Options) *Tool {
	return &Tool{
		Name:        "read",
		Description: "Read the contents of a file. The path should be relative to the workspace root.",
		Execute: func(ctx context.Context, args map[string]any) (string, error) {
			path, ok := args["path"].(string)
			if !ok {
				return "", fmt.Errorf("path is required")
			}
			fullPath, err := resolveToolPath(ws, path, options, approved(args), "read")
			if approval, ok := approvalPayloadFor(err); ok {
				return approval, nil
			}
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(fullPath)
			if err != nil {
				return "", err
			}
			// Limit file reads to 100KB.
			if len(data) > 100*1024 {
				return string(data[:100*1024]) + "\n... [truncated]", nil
			}
			return string(data), nil
		},
	}
}

func writeFile(ws string, options Options) *Tool {
	return &Tool{
		Name:        "write",
		Description: "Write content to a file. The path should be relative to the workspace root.",
		Execute: func(ctx context.Context, args map[string]any) (string, error) {
			path, _ := args["path"].(string)
			content, _ := args["content"].(string)
			if path == "" {
				return "", fmt.Errorf("path is required")
			}
			fullPath, err := resolveToolPath(ws, path, options, approved(args), "write")
			if approval, ok := approvalPayloadFor(err); ok {
				return approval, nil
			}
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				return "", err
			}
			if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
				return "", err
			}
			return fmt.Sprintf("Written %d bytes to %s", len(content), path), nil
		},
	}
}

func shell(ws string, options Options) *Tool {
	return &Tool{
		Name:        "shell",
		Description: "Execute a shell command in the workspace directory.",
		Execute: func(ctx context.Context, args map[string]any) (string, error) {
			command, _ := args["command"].(string)
			if command == "" {
				return "", fmt.Errorf("command is required")
			}
			cwd, err := shellCWD(ws, args["cwd"], options, approved(args))
			if approval, ok := approvalPayloadFor(err); ok {
				return approval, nil
			}
			if err != nil {
				return "", err
			}
			timeout := shellTimeout(args["timeout_ms"])
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			started := time.Now()

			cmd := shellCommand(ctx, command)
			cmd.Dir = cwd
			stdout, stderr, err := runCommand(ctx, cmd)
			result := shellResult{
				Stdout:     truncateOutput(string(stdout)),
				Stderr:     truncateOutput(string(stderr)),
				ExitCode:   exitCode(err),
				TimedOut:   ctx.Err() != nil,
				DurationMS: time.Since(started).Milliseconds(),
				CWD:        cwd,
			}
			if result.TimedOut && result.Stderr == "" {
				result.Stderr = "timeout: command exceeded deadline"
			}
			data, err := json.Marshal(result)
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	}
}

type shellResult struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out"`
	DurationMS int64  `json:"duration_ms"`
	CWD        string `json:"cwd"`
}

func shellCWD(ws string, raw any, options Options, isApproved bool) (string, error) {
	cwd, _ := raw.(string)
	if strings.TrimSpace(cwd) == "" {
		return filepath.Abs(filepath.Clean(ws))
	}
	return resolveToolPath(ws, cwd, options, isApproved, "shell")
}

func shellTimeout(raw any) time.Duration {
	const defaultTimeout = 30 * time.Second
	const maxTimeout = 2 * time.Minute
	var ms int64
	switch v := raw.(type) {
	case float64:
		ms = int64(v)
	case int:
		ms = int64(v)
	case int64:
		ms = v
	}
	if ms <= 0 {
		return defaultTimeout
	}
	d := time.Duration(ms) * time.Millisecond
	if d > maxTimeout {
		return maxTimeout
	}
	return d
}

func truncateOutput(value string) string {
	if len(value) > 10*1024 {
		return value[:10*1024] + "\n... [truncated]"
	}
	return value
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func grep(ws string) *Tool {
	return &Tool{
		Name:        "grep",
		Description: "Search for a pattern in files using ripgrep.",
		Execute: func(ctx context.Context, args map[string]any) (string, error) {
			pattern, _ := args["pattern"].(string)
			if pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}
			cmd := exec.CommandContext(ctx, "rg", "--max-count", "50", pattern, ws)
			out, err := cmd.CombinedOutput()
			if err != nil && len(out) == 0 {
				return "No matches found", nil
			}
			return string(out), nil
		},
	}
}

func listDir(ws string, options Options) *Tool {
	return &Tool{
		Name:        "list_dir",
		Description: "List files in a directory.",
		Execute: func(ctx context.Context, args map[string]any) (string, error) {
			path, _ := args["path"].(string)
			fullPath, err := resolveToolPath(ws, path, options, approved(args), "list_dir")
			if approval, ok := approvalPayloadFor(err); ok {
				return approval, nil
			}
			if err != nil {
				return "", err
			}
			entries, err := os.ReadDir(fullPath)
			if err != nil {
				return "", err
			}
			var lines []string
			for _, e := range entries {
				prefix := "  "
				if e.IsDir() {
					prefix = "📁"
				} else {
					prefix = "📄"
				}
				lines = append(lines, prefix+" "+e.Name())
			}
			return strings.Join(lines, "\n"), nil
		},
	}
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	}
	return exec.Command("sh", "-c", command)
}

func runCommand(ctx context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return stdout.Bytes(), stderr.Bytes(), err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		return stdout.Bytes(), stderr.Bytes(), err
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return stdout.Bytes(), stderr.Bytes(), ctx.Err()
	}
}

// safePath rejects paths that resolve outside the workspace.
func safePath(ws, rel string) (string, error) {
	root, err := filepath.Abs(filepath.Clean(ws))
	if err != nil {
		return "", err
	}
	var candidate string
	if filepath.IsAbs(rel) {
		candidate = filepath.Clean(rel)
	} else {
		candidate = filepath.Join(root, filepath.Clean(rel))
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	relToRoot, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) || filepath.IsAbs(relToRoot) {
		return "", fmt.Errorf("path is outside workspace: %s", rel)
	}
	return candidate, nil
}

type approvalRequiredError struct {
	Tool   string
	Path   string
	Reason string
}

func (e approvalRequiredError) Error() string { return e.Reason }

func resolveToolPath(ws, rel string, options Options, isApproved bool, tool string) (string, error) {
	root, err := filepath.Abs(filepath.Clean(ws))
	if err != nil {
		return "", err
	}
	var candidate string
	if filepath.IsAbs(rel) {
		candidate = filepath.Clean(rel)
	} else {
		candidate = filepath.Join(root, filepath.Clean(rel))
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	if pathInside(root, candidate) {
		return candidate, nil
	}
	if options.PermissionMode == PermissionAllow || isApproved {
		return candidate, nil
	}
	if options.PermissionMode == PermissionAsk {
		return "", approvalRequiredError{Tool: tool, Path: candidate, Reason: "path is outside workspace and requires approval"}
	}
	return "", fmt.Errorf("path is outside workspace: %s", rel)
}

func pathInside(root, candidate string) bool {
	relToRoot, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relToRoot != ".." && !strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relToRoot)
}

func approved(args map[string]any) bool {
	v, _ := args["approved"].(bool)
	return v
}

func approvalPayloadFor(err error) (string, bool) {
	approval, ok := err.(approvalRequiredError)
	if !ok {
		return "", false
	}
	data, marshalErr := json.Marshal(map[string]any{
		"approval_required": true,
		"tool":              approval.Tool,
		"path":              approval.Path,
		"reason":            approval.Reason,
	})
	if marshalErr != nil {
		return "", false
	}
	return string(data), true
}
