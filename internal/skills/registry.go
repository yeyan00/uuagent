package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/paths"
)

// Skill is a lightweight prompt extension. P0 keeps skills simple and local;
// later milestones can load SKILL.md files and package metadata.
type Skill struct {
	Name                   string `json:"name"`
	Description            string `json:"description"`
	Path                   string `json:"path,omitempty"`
	Prompt                 string `json:"prompt"`
	Content                string `json:"-"`
	Enabled                bool   `json:"enabled"`
	Scope                  string `json:"scope"`
	DisableModelInvocation bool   `json:"disable_model_invocation,omitempty"`
}

// Diagnostic records a skill discovery issue without failing the whole scan.
type Diagnostic struct {
	Path    string `json:"path"`
	Name    string `json:"name,omitempty"`
	Message string `json:"message"`
}

// Registry stores available skills by name.
type Registry struct {
	skills      map[string]Skill
	diagnostics []Diagnostic
}

// NewRegistry creates a registry with a mock skill for tests and demos.
func NewRegistry() *Registry {
	r := &Registry{skills: map[string]Skill{}}
	r.Register(Skill{
		Name:        "mock-planner",
		Description: "A tiny simulated skill that asks the agent to answer with a concise plan before acting.",
		Prompt:      "Skill mock-planner: first state a short plan, then execute the user's request concisely.",
		Enabled:     true,
		Scope:       "global",
	})
	return r
}

// NewRegistryFromConfig creates a registry from merged system/user/project config.
func NewRegistryFromConfig(cfg *config.Config) *Registry {
	r := NewRegistry()
	r.scanDir(filepath.Join(paths.UserDir(), "skills"), "global")
	for _, item := range cfg.Skills {
		r.Register(Skill{
			Name:        item.Name,
			Description: item.Description,
			Path:        item.Path,
			Prompt:      loadPrompt(item.Path, item.Prompt),
			Enabled:     item.Enabled,
			Scope:       item.Scope,
		})
	}
	return r
}

// ScanProject overlays project-local skills from <workspace>/.uuagent/skills.
func (r *Registry) ScanProject(workspace string) {
	if strings.TrimSpace(workspace) == "" {
		return
	}
	workspace = filepath.Clean(workspace)
	r.scanDir(filepath.Join(workspace, ".uuagent", "skills"), "project")
	r.scanDir(filepath.Join(workspace, ".agents", "skills"), "project")
	r.scanRootMarkdown(workspace, "project")
}

func (r *Registry) scanDir(root, scope string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(root, name, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		metaName, description, body, disableModelInvocation := parseSkillFile(name, string(data))
		if strings.TrimSpace(description) == "" {
			r.diagnostics = append(r.diagnostics, Diagnostic{Path: path, Name: metaName, Message: "missing description"})
			continue
		}
		r.Register(Skill{Name: metaName, Description: description, Path: path, Content: body, Enabled: true, Scope: scope, DisableModelInvocation: disableModelInvocation})
	}
}

func (r *Registry) scanRootMarkdown(root, scope string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			r.diagnostics = append(r.diagnostics, Diagnostic{Path: path, Message: err.Error()})
			continue
		}
		fallback := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		name, description, body, disableModelInvocation := parseSkillFile(fallback, string(data))
		if strings.TrimSpace(description) == "" {
			r.diagnostics = append(r.diagnostics, Diagnostic{Path: path, Name: name, Message: "missing description"})
			continue
		}
		r.Register(Skill{Name: name, Description: description, Path: path, Content: body, Enabled: true, Scope: scope, DisableModelInvocation: disableModelInvocation})
	}
}

// Register adds or replaces a skill.
func (r *Registry) Register(skill Skill) {
	if skill.Name == "" {
		return
	}
	if skill.Scope == "" {
		skill.Scope = "global"
	}
	r.skills[skill.Name] = skill
}

// Get returns one skill by name.
func (r *Registry) Get(name string) (Skill, bool) {
	skill, ok := r.skills[name]
	return skill, ok
}

// Content returns the full skill body for explicit/on-demand skill loading.
func (r *Registry) Content(name string) (string, bool) {
	skill, ok := r.Get(name)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(skill.Content) != "" {
		return skill.Content, true
	}
	if strings.TrimSpace(skill.Path) != "" {
		data, err := os.ReadFile(filepath.Clean(skill.Path))
		if err == nil {
			_, _, body, _ := parseSkillFile(skill.Name, string(data))
			return body, true
		}
	}
	if strings.TrimSpace(skill.Prompt) != "" {
		return skill.Prompt, true
	}
	return "", false
}

// List returns all skills.
func (r *Registry) List() []Skill {
	out := make([]Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		out = append(out, skill)
	}
	return out
}

// Diagnostics returns skill discovery issues from the latest scans.
func (r *Registry) Diagnostics() []Diagnostic {
	return append([]Diagnostic(nil), r.diagnostics...)
}

// BuildPrompt concatenates enabled skill prompts. If names is empty, all enabled
// skills are included; otherwise only the named enabled skills are included.
func (r *Registry) BuildPrompt(names []string) string {
	var parts []string
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	for _, skill := range r.skills {
		if !skill.Enabled {
			continue
		}
		if skill.DisableModelInvocation {
			continue
		}
		if len(wanted) > 0 && !wanted[skill.Name] {
			continue
		}
		if strings.TrimSpace(skill.Description) != "" {
			parts = append(parts, fmt.Sprintf("<skill name=\"%s\" description=\"%s\" />", skill.Name, escapeAttr(skill.Description)))
			continue
		}
		if strings.TrimSpace(skill.Prompt) != "" {
			parts = append(parts, fmt.Sprintf("<skill name=\"%s\" description=\"%s\" />", skill.Name, escapeAttr(skill.Prompt)))
		}
	}
	if len(parts) > 0 {
		return "[Available skills]\n" + strings.Join(parts, "\n") + "\nLoad the full SKILL.md only when a selected skill is needed."
	}
	return strings.Join(parts, "\n")
}

// ExplicitContentsFromPrompt returns full skill bodies requested with /skill:name.
func (r *Registry) ExplicitContentsFromPrompt(prompt string) []string {
	names := explicitSkillNames(prompt)
	contents := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		content, ok := r.Content(name)
		if !ok || strings.TrimSpace(content) == "" {
			continue
		}
		contents = append(contents, fmt.Sprintf("[Skill %s]\n%s", name, strings.TrimSpace(content)))
	}
	return contents
}

func explicitSkillNames(prompt string) []string {
	var out []string
	for _, field := range strings.Fields(prompt) {
		value, ok := strings.CutPrefix(field, "/skill:")
		if !ok {
			continue
		}
		value = strings.Trim(value, " \t\r\n.,;!?)]}")
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func parseSkillFile(fallbackName, raw string) (string, string, string, bool) {
	name := fallbackName
	description := ""
	body := raw
	disableModelInvocation := false
	if strings.HasPrefix(raw, "---\n") || strings.HasPrefix(raw, "---\r\n") {
		text := strings.TrimPrefix(strings.TrimPrefix(raw, "---\r\n"), "---\n")
		end := strings.Index(text, "\n---")
		if end >= 0 {
			frontmatter := text[:end]
			body = strings.TrimLeft(text[end+len("\n---"):], "\r\n")
			for _, line := range strings.Split(frontmatter, "\n") {
				line = strings.TrimSpace(line)
				if value, ok := strings.CutPrefix(line, "name:"); ok {
					name = strings.Trim(strings.TrimSpace(value), "\"'")
				}
				if value, ok := strings.CutPrefix(line, "description:"); ok {
					description = strings.Trim(strings.TrimSpace(value), "\"'")
				}
				if value, ok := strings.CutPrefix(line, "disable-model-invocation:"); ok {
					disableModelInvocation = strings.EqualFold(strings.Trim(strings.TrimSpace(value), "\"'"), "true")
				}
			}
		}
	}
	return name, description, body, disableModelInvocation
}

func escapeAttr(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	return value
}

func loadPrompt(path, fallback string) string {
	if strings.TrimSpace(path) == "" {
		return fallback
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return fallback
	}
	return string(data)
}
