// Shared frontend types for Agent and Subagent profiles

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
