package agent_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
)

func TestToolCallsPersistInSession(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"I will inspect files.","tool_calls":[{"id":"tc-read","type":"function","function":{"name":"list_dir","arguments":"{\"path\":\".\"}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Final summary after tool."}}]}`))
	}))
	defer ts.Close()
	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "tool-persist", "", "share current code")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	sess, ok := a.Sessions().Get("tool-persist")
	if !ok {
		t.Fatal("session not found")
	}
	snap := sess.Snapshot()
	if len(snap.Messages) < 3 {
		t.Fatalf("expected user/assistant/tool messages, got %d", len(snap.Messages))
	}
	assistant := snap.Messages[1]
	if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Name != "list_dir" {
		t.Fatalf("assistant tool call not persisted: %+v", assistant)
	}
	tool := snap.Messages[2]
	if tool.Role != "tool" || tool.ToolName != "list_dir" || tool.ToolCallID != "tc-read" {
		t.Fatalf("tool result not persisted: %+v", tool)
	}
	final := snap.Messages[len(snap.Messages)-1]
	if final.Role != "assistant" || final.Content != "Final summary after tool." {
		t.Fatalf("final assistant not persisted: %+v", final)
	}
}
