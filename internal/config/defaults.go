package config

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
				Rules: []RouteRule{
					{Name: "large-context", Condition: "tokens > 24000", Tier: "large_ctx"},
					{Name: "fast-simple", Patterns: []string{"typo", "rename", "format", "explain"}, Tier: "fast"},
					{Name: "coding-strong", Patterns: []string{"implement", "fix", "debug", "refactor", "test"}, Tier: "strong"},
				},
			},
			Memory: MemoryConfig{
				AutoDraft:        true,
				MaxEntries:       100,
				MaxCharsPerEntry: 2000,
			},
			Context: ContextConfig{
				MaxTokens:             32000,
				CompressThreshold:     0.75,
				KeepLastMessages:      12,
				AutoCompress:          true,
				CompactReservedTokens: 10000,
				CompactAutoContinue:   true,
			},
			Subagent: SubagentConfig{
				MaxConcurrent: 3,
				MaxTurns:      45,
				BlockedTools:  []string{"delegate", "memory"},
				Profiles:      defaultGoalSubagentProfiles(),
			},
			MaxTurns:          90,
			ReasoningEffort:   "medium",
			DefaultPermission: "workspace-write",
			UI:                UIConfig{Theme: "dark"},
		},
		Agents: []AgentProfile{{
			ID:          "default",
			Name:        "Default Agent",
			Description: "General-purpose coding assistant",
			MaxTurns:    0,
		}},
		Goal: GoalConfig{MaxTurns: 20},
		Hooks: HookConfig{
			TimeoutMS:  5000,
			FailPolicy: "warn",
			Events:     map[string][]HookCommand{},
		},
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

func defaultGoalSubagentProfiles() []SubagentProfile {
	commonTools := []string{"read", "grep", "ls"}
	writeTools := []string{"read", "write", "edit", "grep", "ls", "bash"}
	return []SubagentProfile{
		{ID: "planner", Name: "Planner", Description: "Breaks goals into executable plans.", SystemPrompt: "Plan the goal in isolation. Return concise steps and risks.", EnabledTools: commonTools, PermissionMode: "read-only"},
		{ID: "explorer", Name: "Explorer", Description: "Finds relevant code and constraints.", SystemPrompt: "Explore the repository for facts. Report file paths and constraints only.", EnabledTools: commonTools, PermissionMode: "read-only"},
		{ID: "builder", Name: "Builder", Description: "Implements focused production changes.", SystemPrompt: "Implement the assigned slice with small cohesive changes and report changed files.", EnabledTools: writeTools, PermissionMode: "workspace-write"},
		{ID: "tester", Name: "Tester", Description: "Runs and interprets focused verification.", SystemPrompt: "Run targeted tests, explain failures exactly, and avoid changing behavior unless asked.", EnabledTools: writeTools, PermissionMode: "workspace-write"},
		{ID: "reviewer", Name: "Reviewer", Description: "Reviews correctness, safety, and completeness.", SystemPrompt: "Review the work against the goal, tests, and constraints. Report pass/fail evidence.", EnabledTools: commonTools, PermissionMode: "read-only"},
	}
}
