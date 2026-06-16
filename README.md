# UUAgent

> 轻量级智能 Coding Agent — 自动路由模型，透明 Memory，Web UI 优先

## 快速开始

```bash
# 安装 (单二进制，零依赖)
go install github.com/uuagent/uuagent@latest

# 或下载预编译版本
curl -fsSL https://uuagent.dev/install.sh | bash

# 首次配置：生成 ~/.uuagent/config.yaml 等本地文件
uuagent --setup

# 启动服务端 + Web UI，默认会自动打开浏览器
uuagent              # → http://localhost:18463/ui/

# 如果不想自动打开浏览器
uuagent --no-browser # 然后手动访问 http://localhost:18463/ui/

# 指定端口
uuagent --port 19080 # → http://localhost:19080/ui/

# 终端模式
uuagent --tui
```

## 核心特性

- 🧠 **智能模型路由** — 简单问题用便宜模型，复杂任务自动升级，实时展示路由决策
- 📝 **透明 Memory** — AI 记忆标记为 draft，用户审核确认后才生效
- 🌐 **Web UI 优先** — 浏览器即界面，输入框永不锁定，同时支持桌面模式(Wails)
- ⚡ **极轻量** — 单二进制 15-20MB，零依赖，3秒启动
- 🔌 **CLIProxyAPI 集成** — 模型管理面板开箱即用，支持 OpenAI/Claude/Gemini 全家桶

## 架构

```
UUAgent 二进制
├── CLIProxyAPI (embed)     — 模型代理 + 管理面板
│   ├── /v1/chat/completions  LLM 代理
│   ├── /v0/management/*      40+ 管理 API
│   └── /management.html      管理面板 UI
├── LightAgent 业务层
│   ├── /api/chat             SSE 聊天
│   ├── /api/memory           Memory CRUD
│   ├── /api/route            路由决策展示
│   └── /                     Agent 前端 SPA
```

## 配置

`~/.uuagent/config.yaml`:

```yaml
# 模型代理 (CLIProxyAPI)
port: 18463
remote-management:
  secret-key: "admin"
  disable-control-panel: false

claude-api-key:
  - api-key: "sk-ant-..."
openai-api-key:
  - api-key: "sk-..."
gemini-api-key:
  - api-key: "AIzaSy..."

# 智能路由
agent:
  routing:
    tiers:
      fast: ["gpt-4o-mini", "gemini-2.5-flash", "deepseek-chat"]
      strong: ["claude-sonnet-4", "gpt-4o", "deepseek-coder"]
      large_ctx: ["gemini-2.5-pro"]
    rules:
      - name: simple_qa
        patterns: ["什么意思", "解释", "what is", "explain"]
        tier: fast
      - name: code_edit
        patterns: ["修改", "重构", "fix", "implement"]
        tier: strong
      - name: long_context
        condition: "tokens > 50000"
        tier: large_ctx
  memory:
    auto_draft: true
    max_entries: 100
```

## 运行与访问

Web UI 不是单独的静态网页服务，默认由 Go 后端一起提供。因此需要先启动 UUAgent 服务端：

```bash
go run ./cmd/uuagent --no-browser
```

然后访问：

```text
http://localhost:18463/ui/
```

常用 API：

```text
http://localhost:18463/api/health
http://localhost:18463/api/agents
http://localhost:18463/api/sessions
```

## 开发

```bash
# 后端开发，提供 /api 和 /ui
go run ./cmd/uuagent --no-browser

# 前端开发模式：Vite 会把 /api 代理到 http://localhost:18463
cd web
npm install
npm run dev
# 访问 http://localhost:5173

# 全量测试：Go tests + Web Vitest + Web build
bash scripts/test.sh

# 构建 Go 二进制
go build -o uuagent ./cmd/uuagent

# 桌面版 (需要 wails)
wails build
```

## License

MIT
