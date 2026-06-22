package subagent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/router"
	"github.com/yeyan00/uuagent/internal/session"
	"github.com/yeyan00/uuagent/internal/subagent"
	"github.com/yeyan00/uuagent/internal/types"
)

func TestDelegateRunsSubagentsWithMockLLM(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"subagent-ok"}}]}`))
	}))
	defer ts.Close()
	t.Setenv("UUAGENT_API_KEY", "test-key")

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	parent := agent.New(cfg)

	m := subagent.NewManager(config.SubagentConfig{MaxConcurrent: 2, BlockedTools: []string{"shell"}}, router.New(config.Default().Agent.Routing))
	results := m.Delegate(parent, []string{"a", "b"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if res.ID == "" || res.Goal == "" || res.Model == "" {
			t.Fatalf("incomplete result: %+v", res)
		}
		if res.Error != "" {
			t.Fatalf("unexpected subagent error: %s", res.Error)
		}
		if !strings.Contains(res.Output, "subagent-ok") {
			t.Fatalf("unexpected subagent output: %s", res.Output)
		}
	}
}

func TestDelegateProfilePersistsTaskTreeAndUsesIndependentSessions(t *testing.T) {
	var systemPrompt string
	var requestedModel string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode llm request: %v", err)
		}
		requestedModel = req.Model
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
			systemPrompt = req.Messages[0].Content
		}
		for _, tool := range req.Tools {
			if tool.Function.Name == "shell" {
				t.Fatalf("blocked shell tool should not be sent to child model")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"profile-child-ok"}}]}`))
	}))
	defer ts.Close()
	t.Setenv("UUAGENT_API_KEY", "test-key")
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	parent := agent.New(cfg)
	profile := config.SubagentProfile{
		ID:            "reviewer",
		Name:          "Reviewer",
		SystemPrompt:  "Review with isolated context.",
		Model:         "subagent-model",
		EnabledTools:  []string{"read"},
		BlockedTools:  []string{"shell"},
		EnabledSkills: []string{"mock-planner"},
		MaxTurns:      3,
	}
	taskPath := filepath.Join(t.TempDir(), "subagent-tasks.json")
	sessionRoot := filepath.Join(t.TempDir(), "subagent-sessions")
	m := subagent.NewManagerAt(config.SubagentConfig{MaxConcurrent: 1, Profiles: []config.SubagentProfile{profile}}, router.New(config.Default().Agent.Routing), taskPath, sessionRoot)

	results := m.DelegateProfile(context.Background(), parent, "parent-run", "reviewer", []string{"inspect api server"})
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Error != "" || !strings.Contains(results[0].Output, "profile-child-ok") {
		t.Fatalf("unexpected result: %+v", results[0])
	}
	if results[0].SessionID == "" || !strings.HasPrefix(results[0].SessionID, "subagent-") {
		t.Fatalf("expected independent subagent session id, got %+v", results[0])
	}
	if requestedModel != "subagent-model" {
		t.Fatalf("profile model was not used, got %q", requestedModel)
	}
	if !strings.Contains(systemPrompt, "Review with isolated context.") || !strings.Contains(systemPrompt, "mock-planner") {
		t.Fatalf("profile prompt and skills were not injected: %q", systemPrompt)
	}

	sessionFile := filepath.Join(sessionRoot, results[0].SessionID+".json")
	if _, err := os.Stat(sessionFile); err != nil {
		t.Fatalf("expected child session file %s: %v", sessionFile, err)
	}
	data, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatalf("read task tree: %v", err)
	}
	var tasks []subagent.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		t.Fatalf("decode task tree: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one persisted task, got %d: %s", len(tasks), string(data))
	}
	if tasks[0].ParentID != "parent-run" || tasks[0].ProfileID != "reviewer" || tasks[0].SessionID != results[0].SessionID || tasks[0].Status != "done" {
		t.Fatalf("unexpected persisted task: %+v", tasks[0])
	}
}

func TestDelegateProfileReportsSubagentRouteOverride_whenProfileModelConfigured(t *testing.T) {
	// Given
	parent := &captureParentAgent{}
	profile := config.SubagentProfile{ID: "reviewer", Name: "Reviewer", Model: "subagent-model"}
	routing := config.Default().Agent.Routing
	routing.Fallback = "fast"
	routing.Tiers = map[string][]string{
		"fast":   {"fast-model"},
		"strong": {"strong-model"},
	}
	m := subagent.NewManagerAt(config.SubagentConfig{MaxConcurrent: 1, Profiles: []config.SubagentProfile{profile}}, router.New(routing), "", "")

	// When
	results := m.DelegateProfile(context.Background(), parent, "parent-run", "reviewer", []string{"format this"})

	// Then
	if len(results) != 1 || results[0].Error != "" {
		t.Fatalf("unexpected delegation result: %+v", results)
	}
	if results[0].Model != "subagent-model" || parent.model != "subagent-model" {
		t.Fatalf("subagent profile model should win over routing, result=%+v parent_model=%q", results[0], parent.model)
	}
	if results[0].Route.Source != "subagent" || results[0].Route.Model != "subagent-model" || results[0].Route.Tier != router.TierStrong {
		t.Fatalf("expected subagent route metadata, got %+v", results[0].Route)
	}
	if results[0].Route.Reason != "subagent profile model override" {
		t.Fatalf("expected subagent profile reason, got %q", results[0].Route.Reason)
	}
}

func TestDelegateProfileInheritsSubagentMaxTurnsWhenProfileUnset(t *testing.T) {
	// Given
	parent := &captureParentAgent{}
	profile := config.SubagentProfile{ID: "planner", Name: "Planner"}
	m := subagent.NewManagerAt(config.SubagentConfig{MaxConcurrent: 1, MaxTurns: 45, Profiles: []config.SubagentProfile{profile}}, router.New(config.Default().Agent.Routing), "", "")

	// When
	results := m.DelegateProfile(context.Background(), parent, "parent-run", "planner", []string{"draft plan"})

	// Then
	if len(results) != 1 || results[0].Error != "" {
		t.Fatalf("unexpected delegation result: %+v", results)
	}
	if parent.child.profile.MaxTurns != 45 {
		t.Fatalf("profile max turns should inherit subagent config value 45, got %d", parent.child.profile.MaxTurns)
	}
}

func TestDelegateProfileKeepsProfileMaxTurnsOverride(t *testing.T) {
	// Given
	parent := &captureParentAgent{}
	profile := config.SubagentProfile{ID: "planner", Name: "Planner", MaxTurns: 7}
	m := subagent.NewManagerAt(config.SubagentConfig{MaxConcurrent: 1, MaxTurns: 45, Profiles: []config.SubagentProfile{profile}}, router.New(config.Default().Agent.Routing), "", "")

	// When
	results := m.DelegateProfile(context.Background(), parent, "parent-run", "planner", []string{"draft plan"})

	// Then
	if len(results) != 1 || results[0].Error != "" {
		t.Fatalf("unexpected delegation result: %+v", results)
	}
	if parent.child.profile.MaxTurns != 7 {
		t.Fatalf("profile max turns override should be preserved, got %d", parent.child.profile.MaxTurns)
	}
}

func TestDelegateDefaultsInvalidConcurrency(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"subagent-ok"}}]}`))
	}))
	defer ts.Close()
	t.Setenv("UUAGENT_API_KEY", "test-key")

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	parent := agent.New(cfg)
	m := subagent.NewManager(config.SubagentConfig{MaxConcurrent: 0}, router.New(config.Default().Agent.Routing))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan []subagent.Result, 1)
	go func() {
		done <- m.Delegate(parent, []string{"a"})
	}()

	select {
	case results := <-done:
		if len(results) != 1 {
			t.Fatalf("expected one result, got %d", len(results))
		}
		if results[0].Error != "" {
			t.Fatalf("unexpected subagent error: %s", results[0].Error)
		}
	case <-ctx.Done():
		t.Fatal("Delegate deadlocked with MaxConcurrent=0")
	}
}

type captureParentAgent struct {
	child *captureChildAgent
	model string
}

func (p *captureParentAgent) NewSubagentChildWithSession(model string, _ map[string]bool, _ *session.Store) subagent.ChildAgent {
	p.model = model
	p.child = &captureChildAgent{}
	return p.child
}

type captureChildAgent struct {
	profile config.AgentProfile
}

func (c *captureChildAgent) RunOnce(_ context.Context, prompt string) (string, error) {
	return prompt, nil
}

func (c *captureChildAgent) RunWithProfileParts(_ context.Context, _ string, _ string, profile config.AgentProfile, _ []types.ContentPart) (<-chan types.Event, error) {
	c.profile = profile
	events := make(chan types.Event, 1)
	events <- types.Event{Type: "content", Text: "captured"}
	close(events)
	return events, nil
}

func TestDelegateContextCancelsChildAgents(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	llmStarted := make(chan struct{})
	llmClosed := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(llmStarted)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		close(llmClosed)
	}))
	defer ts.Close()
	t.Setenv("UUAGENT_API_KEY", "test-key")

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	parent := agent.New(cfg)
	m := subagent.NewManager(config.SubagentConfig{MaxConcurrent: 1}, router.New(config.Default().Agent.Routing))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan []subagent.Result, 1)
	go func() {
		done <- m.DelegateContext(ctx, parent, []string{"slow child"})
	}()

	select {
	case <-llmStarted:
	case <-time.After(time.Second):
		t.Fatal("mock LLM was not reached")
	}
	cancel()
	select {
	case <-llmClosed:
	case <-time.After(time.Second):
		t.Fatal("child LLM request was not cancelled")
	}
	select {
	case results := <-done:
		if len(results) != 1 || results[0].Error == "" {
			t.Fatalf("expected cancelled subagent result, got %+v", results)
		}
	case <-time.After(time.Second):
		t.Fatal("DelegateContext did not return after cancellation")
	}
}
