<p align="right">
  <a href="./README.zh-CN.md"><img src="https://img.shields.io/badge/Language-%E4%B8%AD%E6%96%87-blue" alt="中文"></a>
</p>

# UUAgent

A lightweight, Web-first coding agent platform for local projects. UUAgent provides configurable agents, persistent sessions, memory, tool calling, mock MCP/skills, and an OpenAI-compatible model interface.

## Highlights

- **Web-first workflow**: manage projects, sessions, agents, memory, and chat from a browser UI.
- **Configurable agents**: edit system prompt, model, tools, skills, MCP servers, permissions, and max turns.
- **Persistent sessions**: readable JSON session history under `~/.uuagent/sessions`, including tool calls and tool results.
- **Tool loop support**: assistant tool calls are executed, tool results are saved, and the model is called again for the final answer.
- **Memory support**: confirmed memory is persisted under `~/.uuagent/memory.json` and injected into the system prompt.
- **OpenAI-compatible models**: uses `/v1/chat/completions` style requests with streaming and non-streaming support.
- **Multimodal-ready backend**: supports OpenAI-compatible `image_url` content parts.
- **Windows-first development**: scripts and paths are tested with Windows/Git Bash in mind.

## Quick Start

```bash
# Install from source once published
# Main package lives under cmd/uuagent.
go install github.com/yeyan00/uuagent/cmd/uuagent@latest

# First-time local setup: creates ~/.uuagent/config.yaml and local directories.
uuagent --setup

# Start backend + Web UI. The browser opens automatically by default.
uuagent
# Web UI: http://localhost:18463/ui/

# Start without opening a browser.
uuagent --no-browser

# Use a custom port.
uuagent --port 19080
# Web UI: http://localhost:19080/ui/
```

For local development from this repository:

```bash
go run ./cmd/uuagent --no-browser
# Open http://localhost:18463/ui/
```

## Configuration

The user config is stored at:

```text
~/.uuagent/config.yaml
```

Secrets should be provided through environment variables and should not be committed:

```bash
export UUAGENT_API_KEY="..."
export OPENAI_API_KEY="..."
export UUAGENT_PROXY_URL="https://api.openai.com/v1"
export UUAGENT_MODEL="gpt-4o-mini"
```

Example config:

```yaml
port: 18463
agent:
  proxy-url: "http://localhost:18463/v1"
  routing:
    fallback: strong
    tiers:
      fast: ["gpt-4o-mini", "deepseek-chat"]
      strong: ["claude-sonnet-4", "gpt-4o"]
      large_ctx: ["gemini-2.5-pro"]
  context:
    max_tokens: 32000
    compress_threshold: 0.8
    keep_last_messages: 12
    auto_compress: true
  max_turns: 50
  default_permission: "workspace-write"

agents:
  - id: default
    name: Default Agent
    description: General-purpose coding assistant
    system_prompt: ""
    enabled_tools: ["read", "write", "grep", "ls"]
    enabled_skills: [] # Empty means all available skills; set ["mock-planner"] to restrict.
    enabled_mcp_servers: ["mock"]
    permission_mode: "workspace-write"
    max_turns: 50
```

## Runtime Data Layout

UUAgent stores user data under `~/.uuagent` by default:

```text
~/.uuagent/
├── config.yaml          # User configuration: agents, skills, MCP, routing
├── projects.json        # Project registry
├── memory.json          # Persistent memory entries
├── sessions/            # One JSON file per chat session
├── skills/              # User skills
└── mcp/                 # Future MCP-related files
```

Each session JSON stores:

- user and assistant messages
- assistant `tool_calls`
- `tool` result messages with `tool_call_id` and `tool_name`
- final assistant answers after tool execution
- per-run metadata such as agent, model, exposed tools, and MCP servers
- compression summaries

## Web UI

Start the backend first:

```bash
go run ./cmd/uuagent --no-browser
```

Then open:

```text
http://localhost:18463/ui/
```

Useful API endpoints:

```text
http://localhost:18463/api/health
http://localhost:18463/api/agents
http://localhost:18463/api/sessions
http://localhost:18463/api/memory
```

## Development

```bash
# Backend server serving /api and /ui.
go run ./cmd/uuagent --no-browser

# Frontend dev server. Vite proxies /api to http://localhost:18463.
cd web
npm install
npm run dev
# Open http://localhost:5173

# Full validation: Go tests + Web Vitest + Web build.
bash scripts/test.sh

# Build Go binary.
go build -o uuagent ./cmd/uuagent
```

## VS Code

The repository includes VS Code launch/tasks files:

- `UUAgent Server (go run)` starts the backend with `--no-browser`.
- `UUAgent Setup` runs first-time setup.
- `test: all` runs the full test script.

If Go debugging reports that `dlv` is missing, install Delve:

```bash
go install github.com/go-delve/delve/cmd/dlv@latest
```

## Windows-First Test Release Notes

This release is a usable MVP for Windows testing. Key features:

- **CLIProxyAPI Extension**: Place `cli-proxy-api.exe` at `~/.uuagent/plugins/cliproxyapi/cli-proxy-api.exe`. The Extensions page reports missing/installed state, enables Start only when the binary exists, and provides Start/Stop/Restart plus logs/status.
- **Top-level Chat Navigation**: Chat now appears beside Projects in the main rail; Projects remains the project/session browser, and Chat prompts users to choose or create a project when none is active.
- **Built-in Proxy URL**: Models can use the built-in proxy URL configured in Settings > Models.
- **Agent Subagent Allow-list**: Agents can restrict which subagents are enabled via the `enabled_subagents` field.
- **Goal Mode**: Supports delegated activity with subagent task execution and plan/todo tracking in the Web UI.

**Note**: Windows packaging is in progress. Real MCP client support and knowledge base features are planned for future releases.

## Project Status

UUAgent is under active development. Current capabilities include the Web UI, agent profiles, OpenAI-compatible model calls, tool calls, persistent sessions, persistent memory, mock MCP, mock skills, multimodal backend content parts, automated tests, CLIProxyAPI extension lifecycle management, and Goal Mode with delegated subagent execution.

Planned work includes real MCP client support, project-scoped sessions, knowledge base indexing, model-based compression, Windows packaging, E2E testing, and richer UI testing.

## License

MIT
