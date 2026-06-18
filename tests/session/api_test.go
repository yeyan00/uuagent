package session_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/api/server"
	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
)

func TestSessionAPIGetPatchDelete(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	gin.SetMode(gin.TestMode)
	a := agent.New(config.Default())
	s := a.Sessions().GetOrCreate("api-session")
	s.Append("user", "hello")

	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), a)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/api-session", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] != "api-session" {
		t.Fatalf("unexpected session: %+v", got)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/sessions/api-session", bytes.NewBufferString(`{"title":"Readable Title"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("Readable Title")) {
		t.Fatalf("patch status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/sessions/api-session", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", w.Code, w.Body.String())
	}
	if _, ok := a.Sessions().Get("api-session"); ok {
		t.Fatal("session still exists after delete")
	}
}

func TestProjectScopedSessionAPIStoresSessionsUnderProject(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := filepath.Join(t.TempDir(), "workspace")
	t.Setenv("UUAGENT_HOME", home)
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	a := agent.New(config.Default())
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), a)

	projectID := createProjectForSessionTest(t, r, workspace)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID+"/sessions", strings.NewReader(`{"id":"s-one"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("create session status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(home, "projects", projectID, "sessions", "s-one.json")); err != nil {
		t.Fatalf("expected project session file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "sessions", "s-one.json")); !os.IsNotExist(err) {
		t.Fatalf("session should not be stored in global sessions dir, err=%v", err)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID+"/sessions", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"s-one"`) {
		t.Fatalf("list project sessions status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestChatBindsSessionToProjectAndAutoTitlesFromFirstPrompt(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := filepath.Join(t.TempDir(), "workspace")
	t.Setenv("UUAGENT_HOME", home)
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer llm.Close()
	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	gin.SetMode(gin.TestMode)
	a := agent.New(cfg)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), a)
	projectID := createProjectForSessionTest(t, r, workspace)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/chat?prompt="+url.QueryEscape("当前目录是什么\n请简短回答")+"&session_id=s-chat&project_id="+projectID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", w.Code, w.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(home, "projects", projectID, "sessions", "s-chat.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sess map[string]any
	if err := json.Unmarshal(data, &sess); err != nil {
		t.Fatal(err)
	}
	if sess["project_id"] != projectID || sess["project_path"] != workspace {
		t.Fatalf("session not bound to project: %+v", sess)
	}
	if sess["title"] != "当前目录是什么 请简短回答" {
		t.Fatalf("title should come from first prompt, got %+v", sess["title"])
	}
}

func TestProjectSessionContextAPIReportsCurrentContextAndUsage(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := filepath.Join(t.TempDir(), "workspace")
	t.Setenv("UUAGENT_HOME", home)
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"usage response"}}],"usage":{"prompt_tokens":123,"completion_tokens":45,"total_tokens":168}}`))
	}))
	defer llm.Close()
	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	gin.SetMode(gin.TestMode)
	a := agent.New(cfg)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), a)
	projectID := createProjectForSessionTest(t, r, workspace)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/chat?prompt=hello&session_id=s-context&project_id="+projectID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID+"/sessions/s-context/context", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("context status=%d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Context struct {
			EstimatedTokens int     `json:"estimated_tokens"`
			MaxTokens       int     `json:"max_tokens"`
			Percent         float64 `json:"percent"`
		} `json:"context"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Context.EstimatedTokens == 0 || got.Context.MaxTokens != 32000 || got.Context.Percent <= 0 {
		t.Fatalf("unexpected context stats: %+v", got.Context)
	}
	if got.Usage.InputTokens != 123 || got.Usage.OutputTokens != 45 || got.Usage.TotalTokens != 168 {
		t.Fatalf("unexpected usage stats: %+v", got.Usage)
	}
}

func TestProjectSessionCompactAPIArchivesContext(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := filepath.Join(t.TempDir(), "workspace")
	t.Setenv("UUAGENT_HOME", home)
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	a := agent.New(config.Default())
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), a)
	projectID := createProjectForSessionTest(t, r, workspace)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID+"/sessions", strings.NewReader(`{"id":"s-compact"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("create session status=%d body=%s", w.Code, w.Body.String())
	}
	store, _, ok := a.ProjectSessions(projectID)
	if !ok {
		t.Fatal("project session store missing")
	}
	sess, ok := store.Get("s-compact")
	if !ok {
		t.Fatal("project session missing")
	}
	for i := range 8 {
		sess.Append("user", strings.Repeat("archive candidate message ", 120)+fmt.Sprintf("%d", i))
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID+"/sessions/s-compact/compact", strings.NewReader(`{"keep_last_messages":1,"threshold":0.01}`))
	req.Header.Set("Content-Type", "application/json")

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("compact status=%d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Context struct {
			EstimatedTokens int     `json:"estimated_tokens"`
			MaxTokens       int     `json:"max_tokens"`
			Percent         float64 `json:"percent"`
		} `json:"context"`
		Usage     map[string]any `json:"usage"`
		Summaries []any          `json:"summaries"`
		Archives  []any          `json:"archives"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Context.MaxTokens != 32000 || got.Context.EstimatedTokens == 0 || got.Context.Percent <= 0 {
		t.Fatalf("unexpected context stats: %+v", got.Context)
	}
	if got.Usage == nil {
		t.Fatalf("usage missing from compact response: %s", w.Body.String())
	}
	if len(got.Summaries) == 0 {
		t.Fatalf("summaries missing from compact response: %s", w.Body.String())
	}
	if len(got.Archives) == 0 {
		t.Fatalf("archives missing from compact response: %s", w.Body.String())
	}
}

func TestProjectSessionCompactMalformedJSON(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspace := filepath.Join(t.TempDir(), "workspace")
	t.Setenv("UUAGENT_HOME", home)
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Agent.Context.CompressThreshold = 0.01
	a := agent.New(cfg)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), a)
	projectID := createProjectForSessionTest(t, r, workspace)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID+"/sessions", strings.NewReader(`{"id":"s-malformed"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("create session status=%d body=%s", w.Code, w.Body.String())
	}

	store, _, ok := a.ProjectSessions(projectID)
	if !ok {
		t.Fatal("project session store missing")
	}
	sess, ok := store.Get("s-malformed")
	if !ok {
		t.Fatal("project session missing")
	}
	for i := range 20 {
		sess.Append("user", strings.Repeat("archive candidate message ", 120)+fmt.Sprintf("%d", i))
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID+"/sessions/s-malformed/compact", strings.NewReader(`{"keep_last_messages":`))
	req.Header.Set("Content-Type", "application/json")

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected malformed compact JSON to return 400, status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestChatRejectsSessionProjectSwitch(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	workspaceA := filepath.Join(t.TempDir(), "workspace-a")
	workspaceB := filepath.Join(t.TempDir(), "workspace-b")
	t.Setenv("UUAGENT_HOME", home)
	for _, workspace := range []string{workspaceA, workspaceB} {
		if err := os.MkdirAll(workspace, 0755); err != nil {
			t.Fatal(err)
		}
	}
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer llm.Close()
	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	gin.SetMode(gin.TestMode)
	a := agent.New(cfg)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), a)
	projectA := createProjectForSessionTest(t, r, workspaceA)
	projectB := createProjectForSessionTest(t, r, workspaceB)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/chat?prompt=hello&session_id=s-lock&project_id="+projectA, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("first chat status=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/chat?prompt=hello&session_id=s-lock&project_id="+projectB, nil))
	if w.Code != http.StatusConflict {
		t.Fatalf("expected project switch conflict, status=%d body=%s", w.Code, w.Body.String())
	}
}

func createProjectForSessionTest(t *testing.T, r http.Handler, workspace string) string {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(`{"name":"Test","workspace_path":"`+strings.ReplaceAll(workspace, `\`, `\\`)+`"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("create project status=%d body=%s", w.Code, w.Body.String())
	}
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p.ID
}
