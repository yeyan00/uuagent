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

func TestAgentToolLoopSendsToolResultBackToModel(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	calls := 0
	var secondMessages []map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Need listing.","tool_calls":[{"id":"tc-list","type":"function","function":{"name":"list_dir","arguments":"{\"path\":\"internal\"}"}}]}}]}`))
			return
		}
		var req struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		secondMessages = req.Messages
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Final answer based on tool result."}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "tool-loop", "", "summarize code")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if calls != 2 {
		t.Fatalf("expected 2 model calls, got %d", calls)
	}
	foundTool := false
	foundOpenAIToolCall := false
	for _, msg := range secondMessages {
		if msg["role"] == "tool" && msg["tool_call_id"] == "tc-list" {
			foundTool = true
		}
		if msg["role"] == "assistant" {
			if calls, ok := msg["tool_calls"].([]any); ok && len(calls) == 1 {
				if call, ok := calls[0].(map[string]any); ok {
					fn, hasFunction := call["function"].(map[string]any)
					_, hasLegacyArgs := call["args"]
					_, hasLegacyName := call["name"]
					foundOpenAIToolCall = hasFunction && fn["name"] == "list_dir" && fn["arguments"] != "" && !hasLegacyArgs && !hasLegacyName
				}
			}
		}
	}
	if !foundTool {
		t.Fatalf("second model call did not include tool result: %#v", secondMessages)
	}
	if !foundOpenAIToolCall {
		t.Fatalf("second model call did not include OpenAI-compatible assistant tool call: %#v", secondMessages)
	}
}

func TestAgentProfileAskPermissionReturnsApprovalPayloadForOutsideRead(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	outside := filepath.Join(t.TempDir(), "outside.txt")
	calls := 0
	var toolResult string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			args, _ := json.Marshal(map[string]string{"path": outside})
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-read", "type": "function", "function": map[string]any{"name": "read", "arguments": string(args)}}}}}}})
			return
		}
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, msg := range req.Messages {
			if msg.Role == "tool" {
				toolResult = msg.Content
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{ID: "asker", Name: "Asker", PermissionMode: "ask"}}
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "approval-agent", "asker", "read outside")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if !strings.Contains(toolResult, `"approval_required":true`) || !strings.Contains(toolResult, `"tool":"read"`) {
		t.Fatalf("expected approval payload tool result, got %s", toolResult)
	}
}

func TestAgentDefaultAskPermissionReturnsApprovalPayload(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	outside := filepath.Join(t.TempDir(), "outside.txt")
	calls := 0
	var toolResult string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			args, _ := json.Marshal(map[string]string{"path": outside})
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-read", "type": "function", "function": map[string]any{"name": "read", "arguments": string(args)}}}}}}})
			return
		}
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, msg := range req.Messages {
			if msg.Role == "tool" {
				toolResult = msg.Content
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agent.DefaultPermission = "ask"
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "approval-default", "", "read outside")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if !strings.Contains(toolResult, `"approval_required":true`) || !strings.Contains(toolResult, `"tool":"read"`) {
		t.Fatalf("expected approval payload tool result, got %s", toolResult)
	}
}
