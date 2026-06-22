// Shared frontend types for Agent and Subagent profiles

export interface ExtensionStatus {
  id: string;
  name: string;
  description?: string;
  built_in: boolean;
  installed: boolean;
  enabled: boolean;
  status: string;
  binary_path?: string;
  config_path?: string;
  port?: number;
  proxy_url?: string;
  management_url?: string;
  management_path?: string;
  management_installed?: boolean;
  management_secret?: string;
  proxy_api_token?: string;
  last_error?: string;
}

export interface ModelsSettings {
  proxy_url: string;
  proxy_api_key?: string;
  fallback_tier: string;
  routing_tiers: Record<string, string[]>;
  model_ids: string[];
}

export interface ModelsTestResult {
  success: boolean;
  model_ids?: string[];
  error?: string;
}

export interface RoutePreviewResult {
  selected_model?: string;
  selected_tier?: string;
  source?: string;
  rule_name?: string;
  reason?: string;
}

export interface AgentProfile {
  id: string;
  name: string;
  description?: string;
  system_prompt?: string;
  model?: string;
  temperature?: number;
  max_tokens?: number;
  enabled_subagents?: string[];
  created_at?: string;
  updated_at?: string;
}

export interface SubagentProfile {
  id: string;
  name: string;
  description?: string;
  system_prompt?: string;
  model?: string;
  temperature?: number;
  max_tokens?: number;
  // Usage tracking
  usage_count?: number;
  last_used?: string;
  used_by_agents?: string[];
  created_at?: string;
  updated_at?: string;
}

export interface AgentSettingsProps {
  agents: AgentProfile[];
  subagents: SubagentProfile[];
  onSave: (agent: AgentProfile) => Promise<void>;
  onDelete: (agentId: string) => Promise<void>;
  onCreate: () => Promise<void>;
}

export interface SubagentSettingsProps {
  subagents: SubagentProfile[];
  agents: AgentProfile[];
  onSave: (subagent: SubagentProfile) => Promise<void>;
  onDelete: (subagentId: string) => Promise<void>;
  onCreate: () => Promise<void>;
}
