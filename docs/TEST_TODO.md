# UUAgent 测试 TODO 计划

> 覆盖单元测试、API 测试、前端测试、集成测试、端到端测试、Windows 打包验证与跨平台兼容。

## 1. 测试基础设施

- [x] 增加 Go 测试命令：`go test ./...`。
- [x] 增加前端测试框架：Vitest + React Testing Library。
- [ ] 增加 E2E 测试框架：Playwright。
- [x] 增加 mock LLM server，用于稳定测试 Chat/Tool/压缩。
- [ ] 增加测试用临时目录工具，覆盖 Windows 路径、中文路径、空格路径。
- [ ] CI 最低矩阵：Windows latest、Ubuntu latest，后续加 macOS。

---

## 2. 后端单元测试

### 2.1 Config
- [x] 默认配置加载正确。
- [x] `config.example.yaml` 能成功解析。
- [ ] 配置搜索顺序正确：CLI path > cwd > `~/.uuagent` > default。
- [ ] 缺失配置时能给出可理解错误或自动生成默认配置。
- [ ] Windows 用户目录路径解析正确。

### 2.2 Router
- [ ] pattern 命中 simple_qa 返回 fast tier。
- [ ] pattern 命中 code_edit 返回 strong tier。
- [ ] `tokens > N` 条件命中 large_ctx。
- [ ] fallback tier 正常工作。
- [ ] tier 为空时最终 fallback 到安全默认模型。
- [ ] 中英文大小写匹配正常。

### 2.3 Session
- [x] `GetOrCreate` 创建新 session。
- [x] 同 ID 返回同 session。
- [ ] 并发 GetOrCreate 无数据竞争。
- [x] Append 顺序正确。
- [x] BuildMessages 正确追加当前 prompt。
- [x] 持久化后可 resume。
- [x] fork 后新旧 session 消息互不影响。
- [x] 从指定 message fork 时只复制之前上下文。

### 2.4 Memory
- [x] AddDraft 默认状态为 draft。
- [x] Confirm 只确认 draft。
- [x] Edit 修改内容与更新时间。
- [x] Delete 标记 deleted，不物理删除。
- [x] ListDrafts/ListConfirmed 按 project/scope 过滤。
- [x] BuildSystemPrompt 只注入 confirmed。
- [x] BuildSystemPrompt 注入 `user.md`/`memory.md` Markdown confirmed memory。
- [x] `memory.draft.md` 不注入 prompt。
- [x] AI draft memory 写入 project `memory.draft.md`。
- [x] confirmed project memory 写入 project `memory.md`。
- [x] ListFiltered 返回 Markdown confirmed/draft memory。
- [ ] Memory 内容超长时按配置截断或拒绝。
- [ ] 并发编辑无数据竞争。

### 2.5 Tools
- [x] read 限制在 workspace 内。
- [x] write 自动创建父目录。
- [ ] shell 在 workspace 内执行。
- [x] shell 超时生效。
- [x] grep 无结果时返回稳定文本。
- [ ] ls 正确区分文件/目录。
- [x] `..` 路径遍历被拒绝。
- [x] Windows 下 shell 策略测试：PowerShell。

### 2.6 Agent
- [x] callLLM 构造 OpenAI-compatible 请求正确。
- [x] 可从 `tests/models.txt` 读取 baseUrl 与模型名做本地测试。
- [x] API key 只从 `UUAGENT_API_KEY`/`OPENAI_API_KEY` 环境变量读取，代码与日志不泄漏。
- [x] mock LLM server 可验证请求路径、Authorization header、messages、tools。
- [x] LLM 错误能转成 SSE error event。
- [x] route event 在 content 前发送。
- [x] tool call 能触发 tool_start/tool_result。
- [x] 工具失败不会导致整个 server panic。
- [x] Agent 尊重 max_turns。
- [x] Agent 根据 Agent Profile 注入 system prompt/tools/memory。
- [x] Agent Stop API 能取消上游 LLM request，并检查 session JSON。
- [x] Skills 只注入 name/description，完整 SKILL.md 按需读取。
- [ ] Agent 根据 KB 注入知识片段。
- [x] Agent streaming/reasoning 事件正确输出。
- [x] Agent multimodal 内容构造正确。

### 2.7 Context 压缩
- [x] token 估算函数稳定。
- [x] 未达到阈值不压缩。
- [x] 达到阈值自动压缩。
- [x] 最近 N 轮原文保留。
- [x] 压缩摘要写入 summaries。
- [x] 压缩后 BuildMessages token 下降。
- [x] 手动压缩 API 可调用且结果可追踪到 summaries/archives。
- [x] compact archive 归档旧消息并持久化到 session JSON。
- [x] compact restore/回滚归档消息。

### 2.8 Subagent
- [x] subagent 并发数限制生效。
- [x] subagent blocked tools 生效。
- [x] mock skill 能注入 system prompt。
- [x] mock MCP server/client tool 能被 Agent 调用。
- [x] subagent parent context cancel 能取消 child Agent。
- [x] subagent 独立 session 与任务树持久化。
- [ ] subagent task tree UI 可视化测试。
- [ ] goal/delegate mode 自动路由 subagent。

### 2.9 Approvals
- [x] approval required 工具调用触发审批流程。
- [x] approve/stream 继续执行被拦截的工具。
- [x] deny 拒绝执行并返回错误。
- [ ] deny 后继续对话不中断 session。

---

## 3. API 测试

### 3.1 Health/Config
- [x] `GET /api/health` 返回 200 和版本号。
- [x] `GET /api/config` 不泄漏敏感 API key。
- [ ] 配置更新接口校验非法值。

### 3.2 Chat SSE
- [x] `GET /api/chat` 返回 `text/event-stream`。
- [ ] 缺少 prompt 时返回合理错误。
- [x] 携带 project_id/session_id/agent_id 正常。
- [x] SSE event JSON 可解析。
- [x] Stop API 取消时后端和上游 LLM request 退出。
- [x] mock LLM stream 正常透传。
- [ ] 客户端直接断开时后端 goroutine 退出。
- [ ] malformed SSE/error stream 处理。

### 3.3 Projects
- [x] 创建项目：指定本地目录。
- [x] 创建项目：不指定目录时创建 `~/.uuagent/projects/.../workspace`。
- [x] 打开已存在项目。
- [x] existing-file workspace path 错误明确；无权限路径仍需后续补充平台相关测试。
- [ ] 删除项目不误删用户 workspace，除非明确确认临时目录。
- [x] 项目配置文件读写正确。

### 3.4 Sessions
- [x] 列出项目 sessions。
- [x] 新建 session。
- [x] resume 旧 session。
- [x] fork session。
- [x] 删除 session。
- [x] 重命名 session。
- [x] session 与 project 隔离。

### 3.5 Agents/Subagents
- [x] 列出全局与项目 agents。
- [x] 新建 agent profile。
- [x] 修改 prompt/tools/skills/mcp。
- [x] clone agent。
- [ ] 删除 agent 前检查是否被 session 使用。
- [x] subagent 并发数限制生效。
- [x] subagent blocked tools 生效。
- [x] mock skill 能注入 system prompt。
- [x] mock MCP server/client tool 能被 Agent 调用。
- [x] subagent parent context cancel 能取消 child Agent。
- [x] subagent 独立 session 与任务树持久化。

### 3.6 Memory
- [x] 列表按 status/scope 过滤。
- [x] 新增 user memory 直接 confirmed 或按策略。
- [ ] AI draft memory 可确认。
- [ ] memory 可编辑。
- [ ] memory 可删除/恢复。
- [x] Chat 注入 confirmed memory，不注入 draft。
- [x] Chat 按 project/agent/session scope 注入 memory 且不串 scope。
- [x] Chat 对 Markdown memory 使用 frozen snapshot，文件变化不影响当前 session，refresh 后才生效。

### 3.7 Knowledge Base
- [ ] 添加本地文件 source。
- [ ] 添加目录 source。
- [ ] reindex 成功。
- [ ] search 返回相关片段。
- [ ] 删除 source 后不再检索到片段。
- [ ] Chat 中能显示本次注入片段。

### 3.8 MCP
- [ ] 新增 MCP server 配置。
- [ ] 启动/停止 MCP server。
- [ ] MCP 连接失败错误可见。
- [x] Mock MCP tools 合并到 tools 列表。
- [x] 禁用 MCP 后 tool 不再可调用。
- [ ] 真实 MCP stdio/http/sse server lifecycle 测试。

### 3.9 Skills
- [x] 扫描 `~/.uuagent/skills/<name>/SKILL.md` frontmatter。
- [x] 扫描项目 `.uuagent/skills/<name>/SKILL.md`。
- [x] 项目 skill 覆盖同名用户 skill。
- [x] system prompt 只注入 skill name/description，不注入 body。
- [x] `GET /api/skills/:name/content` 按需返回完整内容。
- [x] `.agents/skills` 兼容路径测试。
- [x] root `.md` skill 发现测试。
- [x] invalid frontmatter diagnostics 测试。
- [x] `disable-model-invocation` 测试。
- [x] `/skill:name` 强制加载完整内容测试。

### 3.10 Settings API
- [ ] cliproxyapi/models settings API 完整测试。
- [ ] cliproxyapi/models settings UI 联动测试。

---

## 4. 前端测试

### 4.1 Chat UI
- [x] 初始页面能加载。
- [x] 输入消息并发送。
- [ ] route info 显示模型与 tier。
- [x] stream content 增量更新。
- [x] error event 显示为系统消息。
- [x] 输入框在 streaming 时的交互策略符合产品设计。
- [x] SSE events split across network chunks 解析正确。
- [x] reasoning/thinking 内容渲染。
- [x] markdown 表格/列表渲染。
- [x] tool activity grouping 显示。

### 4.2 Project UI
- [x] 项目列表显示。
- [x] 新建项目表单校验。
- [x] 本地目录路径输入/trim/错误提示流程。
- [ ] 无目录时创建临时目录提示明确。
- [x] 切换项目后 session 列表刷新。
- [x] project path UX 改进测试。

### 4.3 Session UI
- [x] 新建 session。
- [x] resume session。
- [x] fork session。
- [x] 重命名/删除。
- [x] 当前 session 高亮。
- [ ] 长 session 滚动与搜索正常。

### 4.4 Agent 配置 UI
- [x] Agent 下拉选择。
- [x] Prompt 编辑保存。
- [x] Tools 多选保存。
- [x] Skills 多选保存。
- [ ] MCP server 配置保存。
- [x] 权限模式选择保存。
- [ ] 配置错误有校验提示。

### 4.5 Memory UI
- [x] draft/confirmed 分类显示。
- [x] 编辑 memory。
- [x] 确认 draft。
- [x] 删除/恢复。
- [x] 按 scope 过滤。

### 4.6 Knowledge Base UI
- [ ] 添加文件/目录来源。
- [ ] 触发 reindex。
- [ ] 查看索引状态。
- [ ] 搜索知识库。
- [ ] 查看 Chat 注入片段。

### 4.7 压缩信息 UI
- [x] 显示 token 预算。
- [x] 自动压缩后显示提示。
- [x] 压缩记录列表。
- [x] 查看 summary 内容。
- [x] 手动触发压缩与 archive history。
- [x] compact archive restore/回滚 UI。

### 4.8 Activity Panel
- [ ] activity panel 显示思考过程。
- [ ] activity panel 展开/折叠交互。

### 4.9 Attachments
- [x] attachments/paste image 支持。
- [x] multimodal 输入渲染。

---

## 5. 端到端测试场景

### 5.1 首次启动
- [ ] Windows 双击/命令行启动 `uuagent.exe`。
- [ ] 未配置时进入 setup。
- [ ] 自动打开浏览器。
- [ ] 创建第一个项目。
- [ ] 创建第一个 Agent。
- [ ] 发起第一轮 Chat。

### 5.2 本地项目工作流
- [ ] 打开本地目录项目。
- [x] 项目配置写入 `.uuagent/project.yaml`。
- [ ] Session 中询问项目问题。
- [x] Agent 使用 read/grep 工具读取 workspace。
- [x] fork session 后继续不同方向对话。

### 5.3 临时项目工作流
- [x] 不选择目录创建临时项目。
- [x] workspace 位于 `~/.uuagent/projects/<id>/workspace`。
- [x] 文件写入限制在临时 workspace。
- [ ] 删除临时项目时提示是否删除临时文件。

### 5.4 Agent 自定义组合
- [x] 新建 Agent A，只启用 read/grep。
- [x] 新建 Agent B，启用 shell/write。
- [ ] 同一 session 切换不同 agent 或新 session 选择 agent。
- [x] 权限受限的工具调用被拒绝并展示原因。

### 5.5 Memory 与知识库
- [x] 对话产生 draft memory。
- [ ] 用户编辑并确认 memory。
- [x] 下一轮对话注入该 memory。
- [ ] 添加知识库文件。
- [ ] Chat 检索并引用知识片段。

### 5.6 上下文压缩
- [x] 构造长对话超过阈值。
- [x] 自动压缩触发。
- [x] 旧消息摘要可查看。
- [ ] resume 后压缩摘要仍生效。
- [x] fork 后压缩记录正确继承或复制。

---

## 6. Windows 打包与兼容测试

- [x] `npm install && npm run build` 成功。
- [ ] `go build -o dist/uuagent.exe ./cmd/uuagent` 成功。
- [ ] `dist/uuagent.exe --version` 正常。
- [ ] `dist/uuagent.exe --setup` 正常。
- [ ] `dist/uuagent.exe --project <path>` 正常。
- [ ] 启动后访问 `/api/health` 正常。
- [ ] 访问 `/ui/` 能加载前端资源。
- [ ] 中文路径 workspace 正常。
- [ ] 带空格路径 workspace 正常。
- [ ] Windows Defender/杀软不误报或给出说明。
- [ ] 无 Node.js 环境的用户机器可运行 release 包。
- [ ] Windows release smoke tests。

---

## 7. 安全与权限测试

- [x] API 不返回明文模型 API key。
- [x] workspace 外路径读取被拒绝。
- [x] workspace 外路径写入被拒绝。
- [ ] shell dangerous command 根据权限拦截或二次确认。（当前有 ask/approval 和 workspace 边界，dangerous command 分类仍待完善。）
- [ ] MCP env secret 不在 UI 普通展示中泄漏。
- [x] 多项目数据隔离。
- [ ] 删除操作有确认和可恢复策略。

---

## 8. 性能与稳定性测试

- [ ] 1000 条 message 的 session 加载性能。
- [ ] 1000 条 memory 列表分页。
- [ ] 大文件 read 截断。（需要专门的大文件阈值/截断回归测试。）
- [ ] 大目录 KB reindex 进度显示。
- [x] 多 subagent 并发不超过配置上限。
- [ ] 长时间 SSE 连接无内存泄漏。
- [x] Agent 运行取消/浏览器断开后清理资源。

---

## 9. 回归测试清单

每次发布前至少执行：

- [x] `go test ./...`
- [x] `cd web && npm test` 或 Vitest 测试
- [x] `cd web && npm run build`
- [ ] Windows release build
- [x] `/api/health` smoke test
- [x] 新建项目 smoke test
- [x] 新建 session + chat smoke test
- [ ] Memory 编辑 smoke test
- [x] Session fork smoke test
- [x] 打开已有本地目录 smoke test

---

## 10. 测试覆盖率摘要

### 已覆盖 (21 Go test files + 19 web tests)

**Go 测试文件：**
1. `tests/agent/foundation_api_test.go` - Agent LLM 调用基础
2. `tests/agent/skills_test.go` - Skills 注入与调用
3. `tests/tools/registry_test.go` - 工具注册表
4. `tests/agent/tool_persistence_test.go` - 工具持久化
5. `tests/agent/tool_loop_test.go` - 工具循环
6. `tests/agent/stream_test.go` - SSE 流处理
7. `tests/session/api_test.go` - Session API
8. `tests/session/context_test.go` - Session 上下文
9. `tests/agent/docx_skill_real_test.go` - docx skill 真实调用
10. `tests/subagent/manager_test.go` - Subagent 管理
11. `tests/agent/profile_test.go` - Agent Profile
12. `tests/agent/policy_test.go` - 工具策略
13. `tests/agent/multimodal_test.go` - 多模态支持
14. `tests/agent/agent_test.go` - Agent 核心功能
15. `tests/project/project_test.go` - 项目管理
16. `tests/memory/persistence_test.go` - Memory 持久化
17. `tests/agent/memory_tool_test.go` - Memory 工具
18. `tests/agent/memory_snapshot_test.go` - Memory 快照
19. `tests/agent/stop_test.go` - Stop API
20. `tests/session/persistence_test.go` - Session 持久化
21. `internal/config/config_test.go` - 配置加载

**Web 测试 (`web/src/App.test.tsx` - 19 tests)：**
1. rail navigation 和 agent settings 打开
2. Stop API 取消 agent run
3. Settings 与 streaming chat 共存
4. Approval-required tool results 显示
5. Reasoning 和 markdown 渲染
6. SSE events split across chunks 解析
7. Session tool history grouping
8. Project memory 加载与创建
9. Project drawers 与 project-scoped sessions
10. Skills discovery 和 diagnostics
11. Agent skill checkbox 选择
12. Subagent skill 选择
13. `/skill:name` 强制加载 skill
14. Settings navigation 和 skill content preview
15. Skills bulk delete
16. Skills grid cards 和 modals 管理
17. Skills create/delete
18. Subagents create/edit/delete
19. Project settings context token usage

---

## 11. 测试缺口与优先级

### P0 - 阻塞发布
- [ ] E2E/Playwright 基础框架搭建
- [ ] Windows release smoke tests
- [ ] 真实 MCP stdio/http/sse 生命周期测试
- [ ] malformed SSE/error stream 处理

### P1 - 核心功能缺口
- [ ] Knowledge Base 完整测试 (source/reindex/search/inject)
- [ ] attachments/paste image 多模态输入测试
- [ ] activity panel 思考过程显示测试
- [x] models settings API+UI 测试（proxy URL、/models 连接测试、routing tiers）
- [x] compact archive/history 归档与展示测试
- [ ] compact restore/回滚归档消息测试
- [x] goal/delegate mode 自动路由测试（Goal Store/API/Runner、delegate_task、Web Goal Mode 已覆盖）
- [ ] deny approval 后继续对话不中断

### P2 - 体验优化
- [ ] 中文路径/空格路径 workspace 测试
- [ ] 1000+ message session 性能测试
- [ ] 长时间 SSE 内存泄漏测试
- [ ] project path UX 改进测试
- [ ] Windows Defender 兼容性测试

### P3 - 完善覆盖
- [ ] Router pattern matching 测试
- [ ] Config 搜索顺序测试
- [ ] Memory 并发编辑测试
- [ ] Session 并发 GetOrCreate 测试
- [ ] 客户端断开时 goroutine 清理测试

---

## 测试命令

```bash
# Go 测试
go test ./...

# Web 测试
cd web && npm test

# Web 构建
cd web && npm run build

# 完整验证 (PowerShell)
.\scripts\test.ps1

# 完整验证 (Bash)
bash scripts/test.sh
```

---

## Progress update 2026-06-18

### 新增测试覆盖
- [x] Agent streaming/reasoning 事件测试
- [x] Agent multimodal 内容构造测试
- [x] Agent Stop API 取消测试
- [x] Subagent 并发限制/context cancel 测试
- [x] Subagent 独立 session/任务树持久化测试
- [x] Approval flow (approve/deny) 测试
- [x] SSE split chunks 解析测试
- [x] Tool activity grouping 测试
- [x] Project-scoped sessions 测试
- [x] Context token usage 显示测试
- [x] Skills grid/modal/bulk delete 测试
- [x] Subagent create/edit/delete 测试

### 新增测试缺口
- [x] models settings API+UI（proxy URL、/models mock、Settings Models UI）
- [x] compact archive/history 归档与展示
- [ ] compact restore/回滚归档消息
- [ ] cliproxyapi 嵌入/托管代理流程
- [ ] goal/delegate mode 自动路由
- [ ] activity panel 思考过程显示
- [ ] 真实 MCP stdio/http/sse
- [ ] attachments/paste image
- [ ] Knowledge Base 完整流程
- [ ] malformed SSE/error streams
- [ ] deny approval 后继续对话
- [ ] project path UX
- [ ] E2E/Playwright 框架
- [ ] Windows release smoke tests
