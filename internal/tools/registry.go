package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	tools map[string]*Tool
}

// NewRegistry creates a tool registry.
func NewRegistry(workspace string) *Registry {
	r := &Registry{tools: make(map[string]*Tool)}

	// Register built-in tools.
	r.register(readFile(workspace))
	r.register(writeFile(workspace))
	r.register(shell(workspace))
	r.register(grep(workspace))
	r.register(listDir(workspace))

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

func readFile(ws string) *Tool {
	return &Tool{
		Name:        "read",
		Description: "Read the contents of a file. The path should be relative to the workspace root.",
		Execute: func(ctx context.Context, args map[string]any) (string, error) {
			path, ok := args["path"].(string)
			if !ok {
				return "", fmt.Errorf("path is required")
			}
			fullPath := safePath(ws, path)
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

func writeFile(ws string) *Tool {
	return &Tool{
		Name:        "write",
		Description: "Write content to a file. The path should be relative to the workspace root.",
		Execute: func(ctx context.Context, args map[string]any) (string, error) {
			path, _ := args["path"].(string)
			content, _ := args["content"].(string)
			if path == "" {
				return "", fmt.Errorf("path is required")
			}
			fullPath := safePath(ws, path)
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

func shell(ws string) *Tool {
	return &Tool{
		Name:        "shell",
		Description: "Execute a shell command in the workspace directory.",
		Execute: func(ctx context.Context, args map[string]any) (string, error) {
			command, _ := args["command"].(string)
			if command == "" {
				return "", fmt.Errorf("command is required")
			}
			// Timeout after 30 seconds.
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "sh", "-c", command)
			cmd.Dir = ws
			out, err := cmd.CombinedOutput()
			result := string(out)
			// Limit command output to 10KB.
			if len(result) > 10*1024 {
				result = result[:10*1024] + "\n... [truncated]"
			}
			if err != nil {
				return result + "\n[exit code: " + err.Error() + "]", nil
			}
			return result, nil
		},
	}
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

func listDir(ws string) *Tool {
	return &Tool{
		Name:        "list_dir",
		Description: "List files in a directory.",
		Execute: func(ctx context.Context, args map[string]any) (string, error) {
			path, _ := args["path"].(string)
			fullPath := safePath(ws, path)
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

// safePath prevents path traversal outside the workspace.
func safePath(ws, rel string) string {
	rel = filepath.Clean(rel)
	if strings.HasPrefix(rel, "..") {
		return ws // reject .. paths
	}
	return filepath.Join(ws, rel)
}
