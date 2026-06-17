package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
)

func TestRunWithAgentProfileAppliesSystemPromptAndModel(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	var model string
	var system string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		model = req.Model
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
			system = req.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{
		ID:           "coder",
		Name:         "Coder",
		SystemPrompt: "You are a coding agent.",
		Model:        "profile-model",
	}}
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "s-profile", "coder", "hi")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if model != "profile-model" {
		t.Fatalf("expected profile model, got %s", model)
	}
	if !strings.Contains(system, "coding agent") {
		t.Fatalf("expected system prompt, got %q", system)
	}
}
