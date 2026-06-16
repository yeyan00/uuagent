package agent_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/api/server"
	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/memory"
)

func TestChatInjectsProjectMemoryAndScannedSkill(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("UUAGENT_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "skills", "local-reviewer"), 0755); err != nil {
		t.Fatal(err)
	}
	skillFile := `---
name: local-reviewer
description: Reviews project context and persisted memory before answering.
---

# Local Reviewer

DO NOT LOAD THIS BODY UNTIL THE SKILL IS INVOKED.
`
	if err := os.WriteFile(filepath.Join(home, "skills", "local-reviewer", "SKILL.md"), []byte(skillFile), 0600); err != nil {
		t.Fatal(err)
	}

	var systemPrompt string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode llm request: %v", err)
		}
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
			systemPrompt = req.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer llm.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{ID: "scoped", Name: "Scoped", EnabledSkills: []string{"local-reviewer"}}}
	agt := agent.New(cfg)
	agt.Memories().Add("project-a confirmed memory", "project-a", "project", "user", memory.StatusConfirmed)
	agt.Memories().Add("project-b private memory", "project-b", "project", "user", memory.StatusConfirmed)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agt)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chat?prompt=hello&session_id=scoped-session&agent_id=scoped&project_id=project-a", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(systemPrompt, "local-reviewer") || !strings.Contains(systemPrompt, "Reviews project context") {
		t.Fatalf("scanned skill metadata was not injected: %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "DO NOT LOAD THIS BODY") {
		t.Fatalf("skill body should not be injected before invocation: %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "project-a confirmed memory") {
		t.Fatalf("project memory was not injected: %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "project-b private memory") {
		t.Fatalf("memory from another project leaked: %q", systemPrompt)
	}
}

func TestFoundationRegistryAPIsExposeSkillsMCPAndTools(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("UUAGENT_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "skills", "local-planner"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "skills", "local-planner", "SKILL.md"), []byte("---\nname: local-planner\ndescription: Plans work before acting.\n---\n\n# Plan\n"), 0600); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agent.New(config.Default()))

	for _, path := range []string{"/api/skills", "/api/mcp/servers", "/api/tools"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
		body := w.Body.String()
		switch path {
		case "/api/skills":
			if !strings.Contains(body, "local-planner") || !strings.Contains(body, "mock-planner") {
				t.Fatalf("skills api missing scanned or built-in skill: %s", body)
			}
		case "/api/mcp/servers":
			if !strings.Contains(body, "mock") || !strings.Contains(body, "connected") {
				t.Fatalf("mcp api missing mock server status: %s", body)
			}
		case "/api/tools":
			if !strings.Contains(body, "read") || !strings.Contains(body, "mcp_echo") {
				t.Fatalf("tools api missing built-in or MCP tool: %s", body)
			}
		}
	}
}

func TestProjectSkillOverridesUserSkillAndCanLoadFullContent(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := filepath.Join(t.TempDir(), "workspace")
	t.Setenv("UUAGENT_HOME", home)
	for _, dir := range []string{filepath.Join(home, "skills", "reviewer"), filepath.Join(workspace, ".uuagent", "skills", "reviewer")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "skills", "reviewer", "SKILL.md"), []byte("---\nname: reviewer\ndescription: User reviewer.\n---\n\nUSER BODY"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".uuagent", "skills", "reviewer", "SKILL.md"), []byte("---\nname: reviewer\ndescription: Project reviewer.\n---\n\nPROJECT BODY"), 0600); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	agt := agent.New(config.Default())
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agt)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/projects?unused=1", strings.NewReader(`{"name":"P","workspace_path":"`+strings.ReplaceAll(workspace, `\`, `\\`)+`"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("create project status=%d body=%s", w.Code, w.Body.String())
	}
	var projectResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &projectResp); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/projects/"+projectResp.ID+"/open", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("open project status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/skills", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("skills status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Project reviewer") || strings.Contains(body, "User reviewer") {
		t.Fatalf("project skill should override user skill: %s", body)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/skills/reviewer/content", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("skill content status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "PROJECT BODY") || strings.Contains(w.Body.String(), "USER BODY") {
		t.Fatalf("expected project skill content, got %s", w.Body.String())
	}
}

func TestDisabledMCPServerIsNotExposedInToolsAPI(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	cfg := config.Default()
	cfg.MCPServers[0].Enabled = false
	gin.SetMode(gin.TestMode)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agent.New(cfg))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tools", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("tools status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "mcp_echo") {
		t.Fatalf("disabled MCP tool should not be exposed: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/mcp/servers", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("mcp status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "disabled") {
		t.Fatalf("disabled MCP status not visible: %s", w.Body.String())
	}
}

func TestChatInjectsAgentAndSessionScopedMemoryWithoutLeakingOthers(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	var systemPrompt string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
			systemPrompt = req.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer llm.Close()
	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{ID: "agent-a", Name: "Agent A"}, {ID: "agent-b", Name: "Agent B"}}
	agt := agent.New(cfg)
	agt.Memories().Add("project memory", "project-a", "project", "user", memory.StatusConfirmed)
	agt.Memories().Add("agent-a memory", "agent-a", "agent", "user", memory.StatusConfirmed)
	agt.Memories().Add("agent-b memory", "agent-b", "agent", "user", memory.StatusConfirmed)
	agt.Memories().Add("session-a memory", "session-a", "session", "user", memory.StatusConfirmed)
	agt.Memories().Add("session-b memory", "session-b", "session", "user", memory.StatusConfirmed)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/chat?prompt=hello&session_id=session-a&agent_id=agent-a&project_id=project-a", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", w.Code, w.Body.String())
	}
	for _, want := range []string{"project memory", "agent-a memory", "session-a memory"} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("expected %q in system prompt: %q", want, systemPrompt)
		}
	}
	for _, leak := range []string{"agent-b memory", "session-b memory"} {
		if strings.Contains(systemPrompt, leak) {
			t.Fatalf("unexpected leaked memory %q in system prompt: %q", leak, systemPrompt)
		}
	}
}

func TestProjectSkillsScanAgentsPathRootMarkdownDiagnosticsAndExplicitLoad(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := filepath.Join(t.TempDir(), "workspace")
	t.Setenv("UUAGENT_HOME", home)
	t.Setenv("UUAGENT_PROXY_URL", "")
	t.Setenv("UUAGENT_MODEL", "")
	if err := os.MkdirAll(filepath.Join(workspace, ".agents", "skills", "agent-reviewer"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".agents", "skills", "agent-reviewer", "SKILL.md"), []byte("---\nname: agent-reviewer\ndescription: Reviews via .agents skills path.\n---\n\nAGENTS BODY"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "root-helper.md"), []byte("---\nname: root-helper\ndescription: Root markdown skill.\n---\n\nROOT BODY"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "broken-skill.md"), []byte("---\nname: broken-skill\n---\n\nBROKEN BODY"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "manual-only.md"), []byte("---\nname: manual-only\ndescription: Explicitly loaded only.\ndisable-model-invocation: true\n---\n\nMANUAL ONLY BODY"), 0600); err != nil {
		t.Fatal(err)
	}

	var systemPrompt string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode llm request: %v", err)
		}
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
			systemPrompt = req.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer llm.Close()
	if err := os.MkdirAll(filepath.Join(workspace, ".uuagent"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".uuagent", "project.yaml"), []byte("agent:\n  proxy-url: "+llm.URL+"/v1\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	agt := agent.New(cfg)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agt)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(`{"name":"P","workspace_path":"`+strings.ReplaceAll(workspace, `\`, `\\`)+`"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("create project status=%d body=%s", w.Code, w.Body.String())
	}
	var projectResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &projectResp); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/projects/"+projectResp.ID+"/open", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("open project status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/skills", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("skills status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"agent-reviewer", "root-helper", "broken-skill", "missing description"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in skills/diagnostics response: %s", want, body)
		}
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/chat?prompt=hello&session_id=skill-scan", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(systemPrompt, "agent-reviewer") || !strings.Contains(systemPrompt, "root-helper") {
		t.Fatalf("expected discoverable skill metadata in system prompt: %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "manual-only") || strings.Contains(systemPrompt, "MANUAL ONLY BODY") {
		t.Fatalf("manual-only skill should not be advertised without explicit invocation: %q", systemPrompt)
	}

	systemPrompt = ""
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/chat?prompt="+urlQueryEscape("/skill:manual-only use it")+"&session_id=skill-explicit", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("explicit skill chat status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(systemPrompt, "MANUAL ONLY BODY") {
		t.Fatalf("explicit /skill:name should load full skill content: %q", systemPrompt)
	}
}

func TestSubagentProfileAPIsPersistAndListProfiles(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agent.New(config.Default()))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/subagents", strings.NewReader(`{"id":"reviewer","name":"Reviewer","system_prompt":"Review carefully","model":"sub-model","enabled_tools":["read"]}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("create subagent status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "reviewer") || !strings.Contains(w.Body.String(), "sub-model") {
		t.Fatalf("created profile missing fields: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/subagents", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list subagents status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "reviewer") {
		t.Fatalf("profile not listed: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/subagent/tasks", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list tasks status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "tasks") {
		t.Fatalf("task list response malformed: %s", w.Body.String())
	}
}

func urlQueryEscape(value string) string {
	value = strings.ReplaceAll(value, ":", "%3A")
	value = strings.ReplaceAll(value, " ", "+")
	return value
}
