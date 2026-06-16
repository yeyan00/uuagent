package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
