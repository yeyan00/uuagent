package skills

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yeyan00/uuagent/internal/config"
)

// Skill is a lightweight prompt extension. P0 keeps skills simple and local;
// later milestones can load SKILL.md files and package metadata.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path,omitempty"`
	Prompt      string `json:"prompt"`
	Enabled     bool   `json:"enabled"`
	Scope       string `json:"scope"`
}

// Registry stores available skills by name.
type Registry struct {
	skills map[string]Skill
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

// List returns all skills.
func (r *Registry) List() []Skill {
	out := make([]Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		out = append(out, skill)
	}
	return out
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
		if !skill.Enabled || strings.TrimSpace(skill.Prompt) == "" {
			continue
		}
		if len(wanted) > 0 && !wanted[skill.Name] {
			continue
		}
		parts = append(parts, skill.Prompt)
	}
	return strings.Join(parts, "\n")
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
