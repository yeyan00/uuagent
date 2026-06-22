package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
)

func Test_Agent_DelegateTaskTool_returns_subagent_result(t *testing.T) {
	// Given
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	calls := 0
	childCalls := 0
	var delegateToolResult string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			args, err := json.Marshal(map[string]string{"profile_id": "planner", "task": "Draft the plan"})
			if err != nil {
				t.Fatalf("marshal delegate args: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-delegate", "type": "function", "function": map[string]any{"name": "delegate_task", "arguments": string(args)}}}}}}})
			return
		}
		if calls == 2 {
			childCalls++
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"planner-child-ok"}}]}`))
			return
		}
		delegateToolResult = lastToolContent(t, r)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"parent saw delegate result"}}]}`))
	}))
	defer llm.Close()
	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	cfg.Agent.Subagent.Profiles = []config.SubagentProfile{{ID: "planner", Name: "Planner", SystemPrompt: "Plan in isolation.", MaxTurns: 3}}
	a := agent.New(cfg)

	// When
	events, err := a.RunWithAgent(context.Background(), "delegate-task-tool", "", "Ask planner to draft the plan")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}

	// Then
	if calls != 3 {
		t.Fatalf("expected parent tool call, real child subagent call, and final parent call; got %d calls", calls)
	}
	if childCalls != 1 {
		t.Fatalf("expected delegate_task to call one child subagent, got %d", childCalls)
	}
	if !strings.Contains(delegateToolResult, "planner") || !strings.Contains(delegateToolResult, "Draft the plan") || !strings.Contains(delegateToolResult, "planner-child-ok") {
		t.Fatalf("delegate_task should return real subagent output to parent, got %q", delegateToolResult)
	}
}
