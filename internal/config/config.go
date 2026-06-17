package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeyan00/uuagent/internal/paths"
	"gopkg.in/yaml.v3"
)

// Config is the complete UUAgent configuration. YAML is the canonical on-disk
// format because it is friendly for hand editing and can represent nested
// agent/tools/skills/mcp configuration without becoming noisy.
type Config struct {
	Port       int               `yaml:"port" json:"port"`
	Agent      AgentConfig       `yaml:"agent" json:"agent"`
	Agents     []AgentProfile    `yaml:"agents" json:"agents"`
	Skills     []SkillConfig     `yaml:"skills" json:"skills"`
	MCPServers []MCPServerConfig `yaml:"mcp_servers" json:"mcp_servers"`
}

// AgentConfig controls global agent behavior.
type AgentConfig struct {
	ProxyURL          string         `yaml:"proxy-url" json:"proxy_url"`
	Routing           RoutingConfig  `yaml:"routing" json:"routing"`
	Memory            MemoryConfig   `yaml:"memory" json:"memory"`
	Context           ContextConfig  `yaml:"context" json:"context"`
	Subagent          SubagentConfig `yaml:"subagent" json:"subagent"`
	MaxTurns          int            `yaml:"max_turns" json:"max_turns"`
	ReasoningEnabled  bool           `yaml:"reasoning_enabled" json:"reasoning_enabled"`
	ReasoningEffort   string         `yaml:"reasoning_effort" json:"reasoning_effort"`
	DefaultPermission string         `yaml:"default_permission" json:"default_permission"`
	UI                UIConfig       `yaml:"ui" json:"ui"`
}

// AgentProfile is the user/project configurable agent recipe exposed in Web UI.
type AgentProfile struct {
	ID                string   `yaml:"id" json:"id"`
	Name              string   `yaml:"name" json:"name"`
	Description       string   `yaml:"description" json:"description"`
	SystemPrompt      string   `yaml:"system_prompt" json:"system_prompt"`
	Model             string   `yaml:"model" json:"model"`
	EnabledTools      []string `yaml:"enabled_tools" json:"enabled_tools"`
	EnabledSkills     []string `yaml:"enabled_skills" json:"enabled_skills"`
	EnabledMCPServers []string `yaml:"enabled_mcp_servers" json:"enabled_mcp_servers"`
	PermissionMode    string   `yaml:"permission_mode" json:"permission_mode"`
	MaxTurns          int      `yaml:"max_turns" json:"max_turns"`
}

// SkillConfig describes a user or project skill. P0 treats Path/Prompt as local
// metadata; later versions can load SKILL.md and package manifests from Path.
type SkillConfig struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Path        string `yaml:"path" json:"path"`
	Prompt      string `yaml:"prompt" json:"prompt"`
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	Scope       string `yaml:"scope" json:"scope"` // global / project
}

// MCPServerConfig describes an MCP server configured by the user or project.
type MCPServerConfig struct {
	ID        string            `yaml:"id" json:"id"`
	Name      string            `yaml:"name" json:"name"`
	Command   string            `yaml:"command" json:"command"`
	Args      []string          `yaml:"args" json:"args"`
	Env       map[string]string `yaml:"env" json:"env,omitempty"`
	Transport string            `yaml:"transport" json:"transport"` // stdio / sse / http
	Enabled   bool              `yaml:"enabled" json:"enabled"`
	Scope     string            `yaml:"scope" json:"scope"` // global / project
}

// RoutingConfig controls model routing.
type RoutingConfig struct {
	Tiers    map[string][]string `yaml:"tiers" json:"tiers"`       // tier to model list
	Rules    []RouteRule         `yaml:"rules" json:"rules"`       // routing rules
	Fallback string              `yaml:"fallback" json:"fallback"` // default tier
}

// RouteRule describes one routing rule.
type RouteRule struct {
	Name      string   `yaml:"name" json:"name"`
	Patterns  []string `yaml:"patterns" json:"patterns"`
	Condition string   `yaml:"condition" json:"condition"` // e.g. "tokens > 50000"
	Tier      string   `yaml:"tier" json:"tier"`
}

// MemoryConfig controls memory behavior.
type MemoryConfig struct {
	AutoDraft        bool `yaml:"auto_draft" json:"auto_draft"`
	MaxEntries       int  `yaml:"max_entries" json:"max_entries"`
	MaxCharsPerEntry int  `yaml:"max_chars_per_entry" json:"max_chars_per_entry"`
}

// ContextConfig controls automatic local compression.
type ContextConfig struct {
	MaxTokens         int     `yaml:"max_tokens" json:"max_tokens"`
	CompressThreshold float64 `yaml:"compress_threshold" json:"compress_threshold"`
	KeepLastMessages  int     `yaml:"keep_last_messages" json:"keep_last_messages"`
	AutoCompress      bool    `yaml:"auto_compress" json:"auto_compress"`
}

// SubagentConfig controls delegated subagents.
type SubagentConfig struct {
	MaxConcurrent int               `yaml:"max_concurrent" json:"max_concurrent"`
	MaxTurns      int               `yaml:"max_turns" json:"max_turns"`
	BlockedTools  []string          `yaml:"blocked_tools" json:"blocked_tools"`
	Profiles      []SubagentProfile `yaml:"profiles" json:"profiles"`
}

// SubagentProfile configures a delegated child agent.
type SubagentProfile struct {
	ID                string   `yaml:"id" json:"id"`
	Name              string   `yaml:"name" json:"name"`
	Description       string   `yaml:"description" json:"description"`
	SystemPrompt      string   `yaml:"system_prompt" json:"system_prompt"`
	Model             string   `yaml:"model" json:"model"`
	EnabledTools      []string `yaml:"enabled_tools" json:"enabled_tools"`
	EnabledSkills     []string `yaml:"enabled_skills" json:"enabled_skills"`
	EnabledMCPServers []string `yaml:"enabled_mcp_servers" json:"enabled_mcp_servers"`
	BlockedTools      []string `yaml:"blocked_tools" json:"blocked_tools"`
	PermissionMode    string   `yaml:"permission_mode" json:"permission_mode"`
	MaxTurns          int      `yaml:"max_turns" json:"max_turns"`
	WorkspacePath     string   `yaml:"workspace_path" json:"workspace_path"`
}

// UIConfig controls UI preferences.
type UIConfig struct {
	Theme string `yaml:"theme" json:"theme"`
}

// Default returns a usable baseline config without reading files.
func Default() *Config {
	return &Config{
		Port: 18463,
		Agent: AgentConfig{
			ProxyURL: "http://localhost:18463/v1",
			Routing: RoutingConfig{
				Fallback: "strong",
				Tiers: map[string][]string{
					"fast":      {"gpt-4o-mini", "deepseek-chat"},
					"strong":    {"claude-sonnet-4", "gpt-4o"},
					"large_ctx": {"gemini-2.5-pro"},
				},
			},
			Memory: MemoryConfig{
				AutoDraft:        true,
				MaxEntries:       100,
				MaxCharsPerEntry: 2000,
			},
			Context: ContextConfig{
				MaxTokens:         32000,
				CompressThreshold: 0.75,
				KeepLastMessages:  12,
				AutoCompress:      true,
			},
			Subagent: SubagentConfig{
				MaxConcurrent: 3,
				MaxTurns:      20,
				BlockedTools:  []string{"delegate", "memory"},
			},
			MaxTurns:          50,
			ReasoningEffort:   "medium",
			DefaultPermission: "workspace-write",
			UI:                UIConfig{Theme: "dark"},
		},
		Agents: []AgentProfile{{
			ID:          "default",
			Name:        "Default Agent",
			Description: "General-purpose coding assistant",
			MaxTurns:    50,
		}},
		Skills: []SkillConfig{{
			Name:        "mock-planner",
			Description: "Built-in simulated planning skill",
			Prompt:      "First state a short plan, then execute concisely.",
			Enabled:     true,
			Scope:       "global",
		}},
		MCPServers: []MCPServerConfig{{
			ID:        "mock",
			Name:      "Mock MCP",
			Transport: "mock",
			Enabled:   true,
			Scope:     "global",
		}},
	}
}

// UserDir returns ~/.uuagent by default, overridable for tests/dev by UUAGENT_HOME.
func UserDir() string { return paths.UserDir() }

// UserConfigPath returns ~/.uuagent/config.yaml.
func UserConfigPath() string { return filepath.Join(UserDir(), "config.yaml") }

// ProjectConfigPath returns <workspace>/.uuagent/project.yaml.
func ProjectConfigPath(workspace string) string {
	if strings.TrimSpace(workspace) == "" {
		return ""
	}
	return filepath.Join(filepath.Clean(workspace), ".uuagent", "project.yaml")
}

// CandidatePaths returns config layers in increasing precedence:
// defaults < user config < cwd config < project config < explicit UUAGENT_CONFIG.
func CandidatePaths(projectWorkspace string) []string {
	paths := []string{UserConfigPath(), "config.yaml"}
	if p := ProjectConfigPath(projectWorkspace); p != "" {
		paths = append(paths, p)
	}
	if explicit := strings.TrimSpace(os.Getenv("UUAGENT_CONFIG")); explicit != "" {
		paths = append(paths, explicit)
	}
	return paths
}

// Load loads a single config file over defaults.
func Load(path string) (*Config, error) {
	cfg := Default()
	if err := OverlayFile(cfg, path); err != nil {
		return nil, err
	}
	ApplyEnv(cfg)
	return cfg, nil
}

// OverlayFile overlays one YAML file onto cfg.
func OverlayFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, cfg)
}

// LoadAuto loads layered configs, falling back to defaults. Missing files are
// skipped; syntax errors are returned with the offending path in sources.
func LoadAuto(candidates ...string) (*Config, []string, error) {
	cfg := Default()
	var sources []string
	if len(candidates) == 0 {
		candidates = CandidatePaths("")
	}
	for _, path := range candidates {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := OverlayFile(cfg, path); err != nil {
			return cfg, append(sources, path), err
		}
		sources = append(sources, path)
	}
	ApplyEnv(cfg)
	return cfg, sources, nil
}

// EnsureUserLayout creates ~/.uuagent plus common subdirectories and a starter
// config file if it does not exist. It never writes API keys.
func EnsureUserLayout() (string, error) {
	root := UserDir()
	for _, dir := range []string{root, filepath.Join(root, "projects"), filepath.Join(root, "skills"), filepath.Join(root, "mcp"), filepath.Join(root, "agents")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return root, err
		}
	}
	cfgPath := UserConfigPath()
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		data, err := yaml.Marshal(Default())
		if err != nil {
			return root, err
		}
		starter := append([]byte("# UUAgent user config. Secrets/API keys should be supplied by env vars, not committed.\n# Common env vars: UUAGENT_API_KEY, OPENAI_API_KEY, UUAGENT_PROXY_URL, UUAGENT_MODEL.\n# Default Web UI: http://localhost:18463/ui/\n\n"), data...)
		if err := os.WriteFile(cfgPath, starter, 0600); err != nil {
			return root, err
		}
	}
	return root, nil
}

// ApplyEnv overlays non-secret runtime settings. API keys are intentionally not
// stored in Config; LLM callers read them directly from environment variables.
func ApplyEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("UUAGENT_PROXY_URL")); v != "" {
		cfg.Agent.ProxyURL = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("UUAGENT_MODEL")); v != "" {
		if cfg.Agent.Routing.Tiers == nil {
			cfg.Agent.Routing.Tiers = map[string][]string{}
		}
		fallback := cfg.Agent.Routing.Fallback
		if fallback == "" {
			fallback = "strong"
			cfg.Agent.Routing.Fallback = fallback
		}
		cfg.Agent.Routing.Tiers[fallback] = []string{v}
	}
}

// Save writes cfg to YAML. API keys are not part of Config and are not written.
func Save(path string, cfg *Config) error {
	if cfg == nil {
		cfg = Default()
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// SaveUser writes the active config to ~/.uuagent/config.yaml.
func SaveUser(cfg *Config) error {
	if _, err := EnsureUserLayout(); err != nil {
		return err
	}
	return Save(UserConfigPath(), cfg)
}

// Safe returns a JSON-friendly copy with secrets redacted. Config currently
// stores no API keys, but this method centralizes future redaction.
func (c *Config) Safe() map[string]any {
	data, _ := json.Marshal(c)
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	out["secrets"] = "redacted"
	return out
}
