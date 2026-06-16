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

func TestMemorySnapshotIsFrozenUntilRefresh(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	var prompts []string
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
			prompts = append(prompts, req.Messages[0].Content)
		} else {
			prompts = append(prompts, "")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer llm.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	agt := agent.New(cfg)
	agt.Memories().Add("initial stable memory", "project-a", "project", "user", memory.StatusConfirmed)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agt)

	requestChat(t, r, "/api/chat?prompt=first&session_id=mem-freeze&project_id=project-a")
	if len(prompts) != 1 || !strings.Contains(prompts[0], "initial stable memory") {
		t.Fatalf("first prompt did not include initial snapshot: %#v", prompts)
	}

	agt.Memories().Add("new memory after session start", "project-a", "project", "user", memory.StatusConfirmed)
	requestChat(t, r, "/api/chat?prompt=second&session_id=mem-freeze&project_id=project-a")
	if len(prompts) != 2 || !strings.Contains(prompts[1], "initial stable memory") {
		t.Fatalf("second prompt lost frozen snapshot: %#v", prompts)
	}
	if strings.Contains(prompts[1], "new memory after session start") {
		t.Fatalf("new memory should not enter existing session before refresh: %q", prompts[1])
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/sessions/mem-freeze/memory/refresh?project_id=project-a", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", w.Code, w.Body.String())
	}
	requestChat(t, r, "/api/chat?prompt=third&session_id=mem-freeze&project_id=project-a")
	if len(prompts) != 3 || !strings.Contains(prompts[2], "new memory after session start") {
		t.Fatalf("refreshed prompt should include new memory: %#v", prompts)
	}
}

func TestMarkdownMemorySnapshotIsFrozenUntilRefresh(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, ".uuagent"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".uuagent", "memory.md"), []byte("# Project Memory\n\n- original markdown memory\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var prompts []string
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
			prompts = append(prompts, req.Messages[0].Content)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer llm.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	agt := agent.New(cfg)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agt)

	requestChat(t, r, "/api/chat?prompt=first&session_id=md-freeze&project_id="+project)
	if len(prompts) != 1 || !strings.Contains(prompts[0], "original markdown memory") {
		t.Fatalf("first prompt did not include markdown snapshot: %#v", prompts)
	}

	if err := os.WriteFile(filepath.Join(project, ".uuagent", "memory.md"), []byte("# Project Memory\n\n- edited markdown memory\n"), 0600); err != nil {
		t.Fatal(err)
	}
	requestChat(t, r, "/api/chat?prompt=second&session_id=md-freeze&project_id="+project)
	if len(prompts) != 2 || strings.Contains(prompts[1], "edited markdown memory") {
		t.Fatalf("markdown edit should not enter frozen session before refresh: %#v", prompts)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/sessions/md-freeze/memory/refresh?project_id="+project, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", w.Code, w.Body.String())
	}
	requestChat(t, r, "/api/chat?prompt=third&session_id=md-freeze&project_id="+project)
	if len(prompts) != 3 || !strings.Contains(prompts[2], "edited markdown memory") {
		t.Fatalf("refreshed prompt should include edited markdown: %#v", prompts)
	}
}

func requestChat(t *testing.T, r http.Handler, path string) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("chat %s status=%d body=%s", path, w.Code, w.Body.String())
	}
}
