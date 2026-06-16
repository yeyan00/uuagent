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

func TestConfigFromModelFileDoesNotExposeAPIKey(t *testing.T) {
	cfg, err := agent.ConfigFromModelFile("../models.txt")
	if err != nil {
		t.Fatalf("ConfigFromModelFile: %v", err)
	}
	if cfg.Agent.ProxyURL == "" {
		t.Fatalf("expected proxy url from model file")
	}
	if got := cfg.Agent.Routing.Tiers["strong"][0]; got == "" {
		t.Fatalf("expected model from model file")
	}
	data, _ := json.Marshal(cfg)
	if strings.Contains(string(data), "apiKey") || strings.Contains(string(data), "api-key") {
		t.Fatalf("api key field leaked into config: %s", string(data))
	}
}

func TestRunOpenAICompatible(t *testing.T) {
	var auth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}]}`))
	}))
	defer ts.Close()

	t.Setenv("UUAGENT_API_KEY", "test-key")
	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	a := agent.New(cfg)

	events, err := a.Run(context.Background(), "s1", "hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var content string
	for evt := range events {
		if evt.Type == "content" {
			content += evt.Text
		}
		if evt.Type == "error" {
			t.Fatalf("unexpected error event: %s", evt.Text)
		}
	}
	if content != "hello" {
		t.Fatalf("unexpected content: %q", content)
	}
	if auth != "Bearer test-key" {
		t.Fatalf("unexpected auth header: %q", auth)
	}
}

func TestRunMockMCPToolCall(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"tc1","type":"function","function":{"name":"mcp_echo","arguments":"{\"x\":1}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	a := agent.New(cfg)
	events, err := a.Run(context.Background(), "s2", "call mcp")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var toolResult string
	for evt := range events {
		if evt.Type == "tool_result" {
			toolResult = evt.Text
		}
	}
	if !strings.Contains(toolResult, "mock-mcp") || !strings.Contains(toolResult, "\"x\":1") {
		t.Fatalf("unexpected mock mcp output: %s", toolResult)
	}
}

func TestBlockedToolViaRun(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"tc1","type":"function","function":{"name":"read","arguments":"{\"path\":\"go.mod\"}"}}]}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer ts.Close()

	t.Setenv("UUAGENT_PROXY_URL", ts.URL+"/v1")
	a := agent.NewWithModel("mock", map[string]bool{"read": true})
	events, err := a.Run(context.Background(), "s3", "read")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var toolResult string
	for evt := range events {
		if evt.Type == "tool_result" {
			toolResult = evt.Text
		}
	}
	if !strings.Contains(toolResult, "blocked") {
		t.Fatalf("expected blocked tool, got: %s", toolResult)
	}
}
