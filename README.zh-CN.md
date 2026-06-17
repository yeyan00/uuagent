<p align="right">
  <a href="./README.md"><img src="https://img.shields.io/badge/Language-English-blue" alt="English"></a>
</p>

# UUAgent

UUAgent 是一个轻量级、Web 优先的本地 Coding Agent 平台。它支持可配置 Agent、持久化会话、Memory、工具调用、模拟 MCP/Skills，以及 OpenAI-compatible 模型接口。

## 核心特性

- **Web 优先工作流**：在浏览器里管理项目、会话、Agent、Memory 和聊天。
- **可配置 Agent**：可编辑 system prompt、模型、工具、skills、MCP servers、权限和 max turns。
- **持久化 Session**：会话以可读 JSON 存在 `~/.uuagent/sessions`，包含工具调用和工具结果。
- **完整工具循环**：模型返回 tool calls 后执行工具、保存工具结果，再把结果发回模型生成最终回答。
- **Memory 支持**：confirmed memory 持久化到 `~/.uuagent/memory.json`，运行时注入 system prompt。
- **OpenAI-compatible 模型调用**：支持 `/v1/chat/completions` 风格的 streaming 和 non-streaming 请求。
- **多模态后端基础**：支持 OpenAI-compatible `image_url` content parts。
- **Windows 优先开发体验**：脚本和路径优先兼容 Windows/Git Bash。

## 快速开始

```bash
# 发布后可从源码安装。
# main package 位于 cmd/uuagent。
go install github.com/yeyan00/uuagent/cmd/uuagent@latest

# 首次本地初始化：生成 ~/.uuagent/config.yaml 和相关目录。
uuagent --setup

# 启动后端 + Web UI，默认会自动打开浏览器。
uuagent
# Web UI: http://localhost:18463/ui/

# 不自动打开浏览器。
uuagent --no-browser

# 指定端口。
uuagent --port 19080
# Web UI: http://localhost:19080/ui/
```

从当前仓库本地开发运行：

```bash
go run ./cmd/uuagent --no-browser
# 打开 http://localhost:18463/ui/
```

## 配置

用户配置文件：

```text
~/.uuagent/config.yaml
```

密钥建议放在环境变量里，不要提交到仓库：

```bash
export UUAGENT_API_KEY="..."
export OPENAI_API_KEY="..."
export UUAGENT_PROXY_URL="https://api.openai.com/v1"
export UUAGENT_MODEL="gpt-4o-mini"
```

配置示例：

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
    enabled_skills: [] # 留空表示所有可用 skills；设置 ["mock-planner"] 表示只启用指定 skill。
    enabled_mcp_servers: ["mock"]
    permission_mode: "workspace-write"
    max_turns: 50
```

## 本地数据目录

UUAgent 默认把用户数据放在 `~/.uuagent`：

```text
~/.uuagent/
├── config.yaml          # 用户配置：agents、skills、MCP、routing
├── projects.json        # 项目列表
├── memory.json          # 持久化 memory
├── sessions/            # 每个 session 一个 JSON 文件
├── skills/              # 用户自定义 skills
└── mcp/                 # 后续 MCP 相关文件
```

每个 session JSON 会保存：

- user 和 assistant 消息
- assistant 的 `tool_calls`
- 带 `tool_call_id` 和 `tool_name` 的 `tool` 结果消息
- 工具执行后的最终 assistant 回答
- 每轮运行元信息，例如 agent、model、暴露的 tools 和 MCP servers
- 压缩摘要 summaries

## Web UI

先启动后端：

```bash
go run ./cmd/uuagent --no-browser
```

然后打开：

```text
http://localhost:18463/ui/
```

常用 API：

```text
http://localhost:18463/api/health
http://localhost:18463/api/agents
http://localhost:18463/api/sessions
http://localhost:18463/api/memory
```

## 开发

```bash
# 后端服务，提供 /api 和 /ui。
go run ./cmd/uuagent --no-browser

# 前端开发服务。Vite 会把 /api 代理到 http://localhost:18463。
cd web
npm install
npm run dev
# 打开 http://localhost:5173

# 全量验证：Go tests + Web Vitest + Web build。
bash scripts/test.sh

# 构建 Go 二进制。
go build -o uuagent ./cmd/uuagent
```

## VS Code

仓库包含 VS Code launch/tasks 配置：

- `UUAgent Server (go run)` 使用 `--no-browser` 启动后端。
- `UUAgent Setup` 执行首次初始化。
- `test: all` 执行完整测试脚本。

如果 Go 调试提示缺少 `dlv`，安装 Delve：

```bash
go install github.com/go-delve/delve/cmd/dlv@latest
```

## 项目状态

UUAgent 正在开发中。当前已具备 Web UI、Agent Profiles、OpenAI-compatible 模型调用、工具调用、持久化 Session、持久化 Memory、mock MCP、mock skills、多模态后端 content parts 和自动化测试。

后续计划包括真实 MCP client、项目级 sessions、知识库索引、模型驱动压缩、打包发布和更完整的 UI 测试。

## License

MIT
