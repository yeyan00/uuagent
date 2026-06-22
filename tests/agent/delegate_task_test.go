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

func Test_Agent_DelegateTaskTool_rejects_subagent_disabled_for_active_agent(t *testing.T) {
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
		delegateToolResult = lastToolContent(t, r)
		if strings.Contains(delegateToolResult, "subagent profile is not enabled for this agent") {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"parent saw disallowed delegate result"}}]}`))
			return
		}
		childCalls++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"planner-child-should-not-run"}}]}`))
	}))
	defer llm.Close()
	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	cfg.Agent.Subagent.Profiles = []config.SubagentProfile{{ID: "planner", Name: "Planner", SystemPrompt: "Plan in isolation.", MaxTurns: 3}}
	cfg.Agents = []config.AgentProfile{{ID: "limited", Name: "Limited", EnabledSubagents: []string{"reviewer"}}}
	a := agent.New(cfg)

	// When
	events, err := a.RunWithAgent(context.Background(), "delegate-task-disallowed", "limited", "Ask planner to draft the plan")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}

	// Then
	if calls != 2 {
		t.Fatalf("expected parent tool call and final parent call only; got %d calls", calls)
	}
	if childCalls != 0 {
		t.Fatalf("expected disallowed delegate_task not to call child subagent, got %d", childCalls)
	}
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal([]byte(delegateToolResult), &result); err != nil {
		t.Fatalf("delegate_task result should be JSON: %v; result=%q", err, delegateToolResult)
	}
	if result.Success {
		t.Fatalf("delegate_task should fail for disallowed subagent, got %q", delegateToolResult)
	}
	if result.Error != "subagent profile is not enabled for this agent" {
		t.Fatalf("delegate_task should return allow-list error, got %q", result.Error)
	}
}

func Test_Agent_DelegateTaskTool_allows_all_subagents_when_active_agent_allow_list_empty(t *testing.T) {
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
	cfg.Agents = []config.AgentProfile{{ID: "empty", Name: "Empty", EnabledSubagents: nil}}
	a := agent.New(cfg)

	// When
	events, err := a.RunWithAgent(context.Background(), "delegate-task-empty-allow-list", "empty", "Ask planner to draft the plan")
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
		t.Fatalf("expected empty allow-list to permit one child subagent call, got %d", childCalls)
	}
	if !strings.Contains(delegateToolResult, "planner-child-ok") {
		t.Fatalf("delegate_task should preserve existing behavior for empty allow-list, got %q", delegateToolResult)
	}
}
