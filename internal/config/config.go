package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config UUAgent 完整配置
type Config struct {
	Port int         `yaml:"port"`
	Agent AgentConfig `yaml:"agent"`
}

// AgentConfig Agent 行为配置
type AgentConfig struct {
	ProxyURL       string         `yaml:"proxy-url"`
	Routing        RoutingConfig  `yaml:"routing"`
	Memory         MemoryConfig   `yaml:"memory"`
	Subagent       SubagentConfig `yaml:"subagent"`
	MaxTurns       int            `yaml:"max_turns"`
	DefaultPermission string     `yaml:"default_permission"`
	UI             UIConfig       `yaml:"ui"`
}

// RoutingConfig 智能路由配置
type RoutingConfig struct {
	Tiers    map[string][]string `yaml:"tiers"`    // tier → 模型列表
	Rules    []RouteRule         `yaml:"rules"`    // 路由规则
	Fallback string             `yaml:"fallback"`  // 默认 tier
}

// RouteRule 单条路由规则
type RouteRule struct {
	Name      string   `yaml:"name"`
	Patterns  []string `yaml:"patterns"`
	Condition string   `yaml:"condition"` // e.g. "tokens > 50000"
	Tier      string   `yaml:"tier"`
}

// MemoryConfig Memory 配置
type MemoryConfig struct {
	AutoDraft      bool `yaml:"auto_draft"`
	MaxEntries     int  `yaml:"max_entries"`
	MaxCharsPerEntry int `yaml:"max_chars_per_entry"`
}

// SubagentConfig Subagent 配置
type SubagentConfig struct {
	MaxConcurrent int      `yaml:"max_concurrent"`
	MaxTurns      int      `yaml:"max_turns"`
	BlockedTools  []string `yaml:"blocked_tools"`
}

// UIConfig UI 配置
type UIConfig struct {
	Theme string `yaml:"theme"`
}

// Load 加载配置文件
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port: 8765,
		Agent: AgentConfig{
			ProxyURL: "http://localhost:8765",
			Routing: RoutingConfig{
				Fallback: "strong",
				Tiers: map[string][]string{
					"fast":       {"gpt-4o-mini", "deepseek-chat"},
					"strong":     {"claude-sonnet-4", "gpt-4o"},
					"large_ctx":  {"gemini-2.5-pro"},
				},
			},
			Memory: MemoryConfig{
				AutoDraft:      true,
				MaxEntries:     100,
				MaxCharsPerEntry: 2000,
			},
			Subagent: SubagentConfig{
				MaxConcurrent: 3,
				MaxTurns:      20,
				BlockedTools:  []string{"delegate", "memory"},
			},
			MaxTurns:         50,
			DefaultPermission: "workspace-write",
			UI:               UIConfig{Theme: "dark"},
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
