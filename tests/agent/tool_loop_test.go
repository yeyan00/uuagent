package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uuagent/uuagent/internal/agent"
	"github.com/uuagent/uuagent/internal/config"
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
	for _, msg := range secondMessages {
		if msg["role"] == "tool" && msg["tool_call_id"] == "tc-list" {
			foundTool = true
		}
	}
	if !foundTool {
		t.Fatalf("second model call did not include tool result: %#v", secondMessages)
	}
}
