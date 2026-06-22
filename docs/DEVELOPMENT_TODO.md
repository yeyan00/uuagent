# UUAgent 开发 TODO 计划

> 基于当前仓库现状与新增产品需求整理。优先级：P0 必须先做，P1 核心体验，P2 增强能力，P3 长期优化。

## 0. 当前项目现状摘要

### 已有基础
- Go + Gin 后端入口：`cmd/uuagent/main.go`
- API 路由：`api/server/server.go`
- Agent 主结构：`internal/agent/agent.go`
- 模型路由：`internal/router/router.go`
- Session 内存存储：`internal/session/store.go`
- Memory 内存管理：`internal/memory/manager.go`
- Subagent 管理雏形：`internal/subagent/manager.go`
- 内置工具注册表：`internal/tools/registry.go`
- React Web Chat 雏形：`web/src/App.tsx`

### 关键缺口
- OpenAI-compatible Chat、stream、tool loop、run stop 已实现基础闭环。
- Session/Memory 已 JSON 持久化，但还不是项目级 SQLite/迁移架构。
- Skills 已支持 `~/.uuagent/skills`、项目 `.uuagent/skills`、项目 `.agents/skills`、项目 root `.md` 的 frontmatter 扫描；system prompt 默认只注入 name/description，完整内容通过 API 或 `/skill:name` 按需读取。
- MCP 仍是 mock seam：已有状态/API/启禁用过滤，但未实现真实 stdio/http/sse MCP client 生命周期。
- Subagent 已可委派、并发限制、blocked tools、context cancel、profile 约束、独立 session、任务树 JSON 持久化。
- 知识库、SQLite storage 仍未实现；手动 Compact Archive、history 与 restore/回滚已完成，自动/模型压缩策略仍可继续增强。

---

## P0：Agent/Subagent 可调用基础与架构修正

> 调整说明：用户明确 P0 目标应先实现 agent/subagent 调用。因此 P0 不只修编译，还要跑通 OpenAI-compatible Agent 调用、Subagent 基础委派、模拟 Skill、模拟 MCP 测试。`tests/models.txt` 仅用于本地/测试读取 baseUrl 与模型名，API key 必须通过环境变量传入，不能写入代码或提交配置。

### P0.1 修复 Go 包依赖与编译问题
- [x] 抽出公共类型包，例如 `internal/types` 或 `internal/chat`：
  - `Message`
  - `ToolCall`
  - `Event`
- [x] 修改 `session` 不再 import `agent`，避免 import cycle。
- [x] 处理 `subagent` 当前引用未实现方法：
  - 临时禁用未完成代码，或
  - 实现 `agent.NewWithModel` 与 `RunOnce`，或
  - 将 subagent 设计改为通过 `Runner` interface 调用。
- [x] 调整 embed 策略：
  - 开发模式允许未构建 `web/dist`。
  - release 构建前强制 `npm run build`。
- [ ] 增加 `make`/PowerShell 构建脚本：`build-web`, `build-go`, `build-windows`。（目前已有 `scripts/test.ps1` / `scripts/test.sh` 验证脚本，发布构建脚本仍待补齐。）

### P0.2 实现真实 Agent LLM 调用
- [x] 实现 OpenAI-compatible `/v1/chat/completions` 调用。
- [x] 支持使用 `tests/models.txt` 作为本地测试模型来源，但只读取 `baseUrl` 和 `models[].id`。
- [x] API key 仅从环境变量读取，例如 `UUAGENT_API_KEY` 或 `OPENAI_API_KEY`，禁止写入代码、日志、测试断言输出或持久化配置。
- [x] 支持 non-stream 与 stream 两种模式，优先先完成 non-stream，随后补 stream。
- [x] 将 `agent.ProxyURL` 与模型路由接入请求。
- [ ] 实现错误分类：配置缺失、模型不存在、网络失败、限流、上下文超限。
- [ ] SSE 输出统一 JSON encoder，替换手写字符串拼接。

### P0.3 Subagent、Skill、MCP P0 闭环
- [x] Subagent 能基于目标 prompt 路由模型并调用 child Agent。
- [x] Subagent 支持 blocked tools。
- [x] Subagent 并发限制生效。
- [x] 定义一个最小 Skill registry，并提供模拟 skill 用于注入 prompt 测试。
- [x] 定义一个最小 MCP client interface，并提供 mock MCP tool 用于测试。
- [x] Agent tool dispatcher 能区分内置 tool 与 mock MCP tool。
- [x] 增加 agent/subagent/mcp 的单元测试或 mock server 测试。
- [x] Agent run 支持 `run_id` 与 Stop API，取消能传递到上游 LLM request。
- [x] Subagent 支持父 context cancel 传播到 child Agent。

### P0.4 配置加载与初始化
- [ ] 配置搜索顺序：
  1. CLI 参数指定路径
  2. 当前目录 `config.yaml`
  3. 用户目录 `~/.uuagent/config.yaml`
  4. 默认配置
- [x] 实现 `uuagent --setup` 初始化配置。
- [x] 增加 `uuagent --project <path>` / `--port <port>`。
- [x] 配置热更新基础：保存后可重载 Agent/Profile 配置。（Agent/Profile 保存后可重载；完整全局热更新仍需继续完善。）

---

## P1：Web GUI 项目与 Session 主流程

### P1.1 项目管理（类似 opencode GUI）
- [x] Web 显示项目列表与项目相关入口。
- [x] 新建项目流程：
  - [x] 输入项目名。
  - [x] 输入/校验本地目录路径，错误提示清晰且失败时保留输入。（原生目录选择器可作为后续增强。）
  - [x] 如果不选择目录，在 `~/.uuagent/projects/<project-id>/workspace` 创建临时目录。
- [x] 后端支持注册/打开本地项目路径，例如用户提到的：
  - `C:\00_work\greenvalley\code\llm\opencode`
- [x] 设计项目配置文件：
  - 项目内优先：`<workspace>/.uuagent/project.yaml`
  - 临时项目：`~/.uuagent/projects/<project-id>/project.yaml`
- [x] 后端新增 Project API：
  - `GET /api/projects`
  - `POST /api/projects`
  - `GET /api/projects/:id`
  - `PATCH /api/projects/:id`
  - `DELETE /api/projects/:id`
  - `POST /api/projects/:id/open`
- [x] Project 数据字段建议：
  - `id`, `name`, `workspace_path`, `config_path`, `created_at`, `updated_at`, `last_session_id`

### P1.2 项目内 Chat Session
- [x] Chat 请求支持携带 `project_id` 与 `session_id`。
- [x] Web 左侧栏显示当前项目 Session 列表。
- [x] 支持新建 Session、重命名、删除、搜索。
- [x] Session 数据持久化到 JSON（`~/.uuagent/sessions/*.json`），SQLite 待迁移。
- [x] API：
  - `GET /api/projects/:projectId/sessions`
  - `POST /api/projects/:projectId/sessions`
  - [x] `GET /api/sessions/:id`
  - [x] `PATCH /api/sessions/:id`
  - [x] `DELETE /api/sessions/:id`
- [x] 支持 resume：打开旧 Session 继续对话。
- [x] 支持 fork：从某条消息或当前状态复制成新 Session。

### P1.3 Agent 选择与 Agent Profile
- [x] 定义 Agent Profile 数据结构：
  - `id`, `name`, `description`
  - `system_prompt`
  - `model_policy` / `routing`
  - `enabled_tools`
  - `enabled_skills`
  - `enabled_mcp_servers`
  - `permission_mode`
  - `max_turns`
  - `context_policy`
- [x] Web Chat 顶部支持选择 Agent。
- [x] 项目可覆盖全局 Agent 配置。
- [x] API：
  - `GET /api/agents`
  - `POST /api/agents`
  - `GET /api/agents/:id`
  - `PATCH /api/agents/:id`
  - `DELETE /api/agents/:id`
  - `POST /api/agents/:id/clone`

---

## P1：Memory 可视化与可编辑

### P1.4 Memory 管理
- [x] Memory 从内存改为持久化。
- [ ] Web 增加 Memory 面板：
  - [x] confirmed/当前项目 memory 基础显示与创建
  - [ ] draft 待确认
  - [ ] confirmed 生效中完整管理
  - [ ] deleted/archived 历史
- [ ] 支持显示、编辑、确认、删除、恢复。（后端能力较完整，Web 管理体验仍需补齐。）
- [x] Memory 支持 scope：
  - global
  - project
  - agent
  - session
- [x] `BuildSystemPrompt` 按 project/agent/session scope 注入。
- [x] Markdown-first memory 文件：
  - `~/.uuagent/user.md`
  - `~/.uuagent/memory.md`
  - `<project>/.uuagent/user.md`
  - `<project>/.uuagent/memory.md`
  - `<project>/.uuagent/memory.draft.md`
- [x] Markdown confirmed memory 注入 frozen session snapshot。
- [x] AI auto-draft 写入 project `memory.draft.md`，不注入 prompt。
- [x] 当前 session 的 memory snapshot 冻结；Markdown/JSON 后续变化需显式 refresh 才生效。
- [x] Memory API：
  - `GET /api/memories?project_id=&agent_id=&status=`
  - `POST /api/memories`
  - `PATCH /api/memories/:id`
  - `POST /api/memories/:id/confirm`
  - `DELETE /api/memories/:id`

---

## P2：Tools / Skills / MCP Web 可配置

### P2.1 Tools 配置
- [ ] 将内置 tools 注册表变成可配置 registry。（当前 Agent Profile 可过滤内置 tools，但 registry 仍是内置实现。）
- [x] Web Agent 配置页支持勾选 tools。
- [ ] 工具按权限分级：
  - [x] read-only / workspace-write / ask/approval 基础语义
  - [x] shell 基础支持
  - [ ] network
  - [ ] dangerous
- [ ] 支持项目级工具白名单/黑名单。（当前主要是 Agent/Profile 级过滤。）
- [ ] Windows shell 兼容：PowerShell/CMD/Git Bash 可选。（当前默认 PowerShell，尚未提供多 shell 选择。）

### P2.2 Skills 配置
- [x] 定义 Skill metadata：`name`, `description`, `path`, `enabled`, `scope`。
- [x] 支持扫描：
  - `~/.uuagent/skills`
  - 项目 `.uuagent/skills`
  - 内置 skills
- [ ] Web 端支持启用/禁用 skill。（当前支持 Agent/Subagent skill 白名单与 Skills 管理，独立 enable/disable 开关待完善。）
- [x] Agent 运行时按已启用 skill 注入 name/description；完整 `SKILL.md` 内容按需读取。
- [x] API：`GET /api/skills`、`GET /api/skills/:name/content`。
- [x] 兼容 `.agents/skills` 路径、root `.md` skill、diagnostics、`disable-model-invocation`、`/skill:name` 强制加载命令。
- [x] 支持 Skills 创建/上传/URL/删除。

### P2.3 MCP 配置
- [ ] MCP Server 配置结构：
  - `id`, `name`, `command`, `args`, `env`, `transport`, `enabled`
- [ ] Web 端支持新增/编辑/删除 MCP server。
- [ ] 实现 MCP client 生命周期管理。
- [x] Mock MCP tools 合并到 Agent tool registry，禁用 MCP 后不暴露/不可调用。
- [x] API：`GET /api/mcp/servers`、`GET /api/tools`。
- [ ] MCP 连接状态可视化与错误日志。
- [ ] 实现真实 stdio/http/sse MCP server 启动、停止、重启与 tools schema discovery。

### P2.4 Subagent 可配置
- [x] Subagent Profile 与 Agent Profile 共用基础运行字段。
- [x] API 端可配置 subagent：prompt/tools/skills/mcp/model/权限。
- [x] Web 端可配置 subagent：prompt/tools/skills/mcp/model/权限（Settings 页面）。
- [x] 支持在主 Agent 中选择可委派的 subagents（enabled_subagents 字段已实现）。
- [x] 子任务执行隔离：独立 session、受限 tools。
- [ ] 子任务执行隔离：受限 workspace。
- [x] API 端显示 subagent 任务树与执行结果。
- [x] Web 端显示 subagent 任务树与执行结果（Goal Activity Panel 已实现）。
- [x] Subagent context cancel 传播与 invalid concurrency 防死锁。

---

## P2：知识库与上下文压缩

### P2.5 知识库（Knowledge Base）
- [ ] 支持项目知识库与全局知识库。
- [ ] 文档来源：本地文件、目录、手动笔记、URL（后续）。
- [ ] 初版可先做全文索引/关键词检索，后续接 embedding 向量检索。
- [ ] API：
  - `GET /api/kb`
  - `POST /api/kb/sources`
  - `POST /api/kb/reindex`
  - `GET /api/kb/search?q=`
  - `DELETE /api/kb/sources/:id`
- [ ] Chat 时按 Agent 配置决定是否检索 KB。
- [ ] Web 端显示“本次注入的知识片段”。

### P2.6 上下文预算与自动压缩
- [ ] 实现 token 估算模块。
- [ ] Agent 每轮运行前计算上下文预算：
  - system prompt
  - memory
  - session messages
  - tool results
  - KB snippets
- [ ] 设置阈值：例如达到模型上下文 70%-80% 自动压缩。
- [ ] 压缩策略：
  - 保留最近 N 轮原文。
  - 较早消息压缩成 summary。
  - 工具结果可结构化摘要。
- [x] 压缩记录持久化：
  - `id`, `session_id`, `from_message_id`, `to_message_id`, `summary`, `model`, `created_at`, `token_before`, `token_after`（当前 JSON store 中为 summaries/archives 字段）
- [x] Web 端可查看压缩信息：
  - 压缩时间
  - 覆盖消息范围
  - 压缩前后 token
  - summary 内容
- [x] 支持手动触发压缩与 Compact Archive restore/回滚。
- [ ] 支持重新压缩/更换模型摘要。

---

## P2：数据存储与审计

### P2.7 持久化方案
- [ ] 优先 SQLite：适合单二进制、Windows、本地应用。
- [ ] 数据库存放：
  - 全局：`~/.uuagent/uuagent.db`
  - 项目：`<project>/.uuagent/project.db` 或全局 DB 引用 workspace。
- [ ] 表建议：projects, agents, sessions, messages, memories, kb_sources, kb_chunks, summaries, tool_runs, mcp_servers。
- [ ] 增加 migration 机制。

### P2.8 操作日志与可观测性
- [x] 记录每轮 Agent run：模型、路由原因、token、耗时、tool calls。
- [ ] Web 端显示 route decision 与 tool execution（Activity Panel 待实现）。
- [ ] 错误日志可导出，便于用户反馈。

---

## P3：打包、分发与跨平台

### P3.1 Windows 优先打包
- [ ] Windows 单二进制：`uuagent.exe`。
- [ ] 支持嵌入 Web dist。
- [ ] 提供 PowerShell install 脚本。
- [ ] 可选 Wails 桌面壳。
- [ ] 处理 Windows 路径、空格、中文路径。
- [ ] 处理 Windows shell 差异：PowerShell 默认，Git Bash 可选。

### P3.2 跨平台兼容
- [ ] Linux/macOS 构建脚本同步维护。
- [ ] 路径统一使用 `filepath`，避免硬编码分隔符。
- [ ] 浏览器打开逻辑保持 `windows/darwin/linux` 分支。
- [ ] CI matrix：Windows + Linux + macOS。

### P3.3 发布流程
- [ ] GitHub Actions 构建 release artifacts。
- [ ] 前端 build + 后端 embed + Go build。
- [ ] 版本号注入：commit、build time、version。
- [ ] smoke test：启动服务、访问 `/api/health`、加载 `/ui/`。

---

## 建议里程碑

### Milestone 1：能跑通最小 Chat
- 修复编译问题。
- 完成 LLM 调用。
- 完成基础 Session 持久化。
- Web 可发送并流式显示回答。

### Milestone 2：项目化 Web GUI
- 项目创建/打开。
- 项目配置保存。
- Session 列表、resume、fork。
- Agent 选择。

### Milestone 3：可配置 Agent 平台
- Web 配置 prompt/tools/skills/mcp。
- Subagent 配置与执行显示。
- Memory 可视化编辑。

### Milestone 4：知识库与上下文治理
- KB 初版检索注入。
- 自动压缩与压缩记录查看。
- Token 预算可视化。

### Milestone 5：Windows 发布
- Windows 打包脚本。
- 安装/升级流程。
- 跨平台 CI。

---

## 状态矩阵 2026-06-18

| 模块 | 状态 | 说明 |
|------|------|------|
| OpenAI Chat/Stream | 已实现 | 支持 /v1/chat/completions，流式与非流式 |
| Tool Loop | 已实现 | 工具调用后自动追加结果并继续对话 |
| Run Stop/Cancel | 已实现 | Stop API 可取消上游 LLM 请求 |
| Approvals | 已实现 | 危险操作需用户确认 |
| Session | 已实现 | JSON 持久化，resume/fork |
| Project | 已实现 | 项目注册表持久化到 ~/.uuagent/projects.json |
| Skills | 已实现 | 扫描 ~/.uuagent/skills、项目 .uuagent/skills、.agents/skills、root .md；支持创建/上传/URL/删除；渐进式披露 |
| Settings | 已实现 | Web UI Skills/Subagents/Agents 配置页 |
| Subagents | 已实现 | 后端管理器，支持并发限制、blocked tools、context cancel、独立 session、任务树持久化 |
| Memory | 已实现 | Markdown-first，支持 global/project/agent/session scope，frozen snapshot |
| Context Compression | 已实现基础闭环 | 本地确定性压缩逻辑；已支持手动 compact archive/history 与 restore/回滚；自动/模型摘要策略可继续增强 |
| cliproxyapi/models | MVP (built-in extension) | CLIProxyAPI backend extension lifecycle endpoints 已实现；Extensions UI 支持 missing/installed 状态、二进制路径提示、Start/Stop/Restart 控制与 logs/status；Models Settings API/UI 已支持 proxy URL、/models 连接测试、模型列表与 routing tiers；CLIProxyAPI 二进制需放置于 plugins/cliproxyapi/cli-proxy-api.exe |
| Chat Navigation | 已实现 | Chat 已提升为与 Projects 同级的主导航入口；Projects 保留项目/session 浏览，Chat 无 active project 时提示选择或创建项目 |
| Session Token 显示 | 已实现 | active session workspace header 显示 Input/Output/Total；Project Settings context 仍显示详细 token |
| Compact Archive | 已实现 | 手动 Compact 会归档被压缩消息并显示 Compact Archives；支持 restore/回滚并刷新当前 session/context |
| Project Path UX | 已实现 | 创建项目时路径 trim、错误提示、existing-file path 校验、中文/空格路径测试已覆盖 |
| Attachments | 已实现 | 后端支持 image_url；Web 支持图片选择/粘贴、预览/删除、附件-only 发送与消息渲染 |
| Goal/Delegate Mode | MVP | GoalRun JSON store、Goal API、顺序 runner、内置 planner/explorer/builder/tester/reviewer profiles、delegate_task 工具、Web Goal mode 与 plan/todo/activity 展示、subagent 独立 session 与任务树持久化、context cancel 传播、并发限制防死锁均已实现；验证：go test ./...、cd web && npm test 通过 |
| Activity Panel | MVP (Goal activity) | Goal Activity Panel 已展示 Goal plan/todos/subagent delegate activities；通用工具调用/思考过程统一面板仍待完善 |
| Real MCP | 未实现 | 仅 mock，无真实 stdio/http/sse client |
| Knowledge Base | 未实现 | 无文档索引与检索 |
| Agent enabled_subagents | MVP | Agent Profile 支持 enabled_subagents 字段，可限制子代理调用范围 |

---

## Task 8 发布状态 (2026-06-22)

本次 Windows-first test release MVP 已完成：

**已完成 MVP：**
1. **Agent/Subagent Settings** - Agent Profile 支持 enabled_subagents 限制子代理范围；Subagent 支持并发限制、blocked tools、context cancel、独立 session、任务树持久化。
2. **CLIProxyAPI Extension** - 作为默认内置扩展 MVP 实现：backend 支持 lifecycle 端点，Extensions UI 支持 missing/installed 状态、`plugins/cliproxyapi/cli-proxy-api.exe` 路径提示、Start/Stop/Restart 控制与查看 logs/status。
3. **Top-level Chat Navigation** - Chat 已提升为与 Projects 同级的主导航入口；Projects 保留项目/session 浏览，Chat 无 active project 时显示选择或创建项目的空状态。
4. **Model Routing** - 部分实现但明确可测试：Models Settings API/UI 支持 proxy URL、/models 连接测试、模型列表与 routing tiers/fallback。
5. **Goal Mode** - MVP 实现：GoalRun store/API/runner、内置 profiles、delegate_task 工具、Web Goal mode 与 plan/todo/activity 展示、subagent 委派执行。

**仍为未来工作：**
- Real MCP client 生命周期 (stdio/http/sse)
- Knowledge Base 索引与检索
- Windows 打包与发布流程
- E2E/Playwright 测试框架
- Windows release smoke tests

## 下一阶段计划 (2026-06-22)

按优先级排序：

**P1 - 近期重点：**
1. **Real MCP** - 实现真实 MCP client 生命周期 (stdio/http/sse)。
2. **Knowledge Base** - 项目/全局知识库，文档索引与检索注入。
3. **Activity Panel 完善** - 通用工具调用/思考过程统一展示面板。
4. **模型路由策略增强** - 基于任务类型/上下文长度的自动路由决策。

**P2 - 后续规划：**
5. **Windows 打包** - 单二进制发布、安装脚本、CI 构建。
6. **E2E 测试** - Playwright 框架搭建与核心流程覆盖。
7. **大文件策略** - 自动截断与流式读取完善。
8. **SQLite 存储迁移** - 从 JSON 持久化迁移到 SQLite。

---

## 历史进度

### 2026-06-16
- [x] AgentProfile enabled_tools and enabled_mcp_servers affect runtime.
- [x] Tool definitions are filtered by profile and tool execution is re-checked.
- [x] Project registry is persisted to the UUAgent user directory projects.json.
- [x] Web UI includes Projects, Sessions, Agent selection, Memory, and Compression summaries.
- [x] OpenAI-compatible streaming delta parsing and SSE forwarding added.
- [x] Test scripts added: scripts/test.sh and scripts/test.ps1.
- [x] Web build verified in current environment.
- [ ] Run go test ./... in an environment with Go installed and fix any compile/gofmt issues.
