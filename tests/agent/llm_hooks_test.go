package agent_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeyan00/uuagent/internal/config"
)

func TestLLMBeforeHookAddsHeaderAndParam(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	seenHeader := ""
	seenTemperature := 0.0
	seenMaxTokens := 0.0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeader = r.Header.Get("X-Hook-Trace")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		seenTemperature, _ = body["temperature"].(float64)
		seenMaxTokens, _ = body["max_tokens"].(float64)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer upstream.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = upstream.URL + "/v1"
	cfg.Hooks.Events["chat.headers"] = []config.HookCommand{{Command: agentHookHelperCommand("chat_headers"), FailPolicy: "fail"}}
	cfg.Hooks.Events["chat.params"] = []config.HookCommand{{Command: agentHookHelperCommand("chat_params"), FailPolicy: "fail"}}
	r := newModelsSettingsRouter(cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"hello","session_id":"llm-before-hook"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", w.Code, w.Body.String())
	}
	if seenHeader != "trace-123" {
		t.Fatalf("expected hook header, got %q", seenHeader)
	}
	if seenTemperature != 0.25 || seenMaxTokens != 77 {
		t.Fatalf("expected hook params temperature=0.25 max_tokens=77, got temperature=%v max_tokens=%v", seenTemperature, seenMaxTokens)
	}
}

func TestLLMAfterHookMutatesAssistantResponse(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"raw response"}}]}`))
	}))
	defer upstream.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = upstream.URL + "/v1"
	cfg.Hooks.Events["llm.after"] = []config.HookCommand{{Command: agentHookHelperCommand("llm_after_response"), FailPolicy: "fail"}}
	r := newModelsSettingsRouter(cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"hello","session_id":"llm-after-hook"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "mutated by hook") {
		t.Fatalf("expected mutated response in SSE body, got %s", w.Body.String())
	}
}

func TestLLMAfterHookDoesNotMutateToolCalls(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	calls := 0
	seenToolResult := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Need listing.","tool_calls":[{"id":"tc-list","type":"function","function":{"name":"ls","arguments":"{\"path\":\"internal\"}"}}]}}]}`))
			return
		}
		var body struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		for _, msg := range body.Messages {
			if msg.Role == "tool" {
				seenToolResult = true
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer upstream.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = upstream.URL + "/v1"
	cfg.Agent.MaxTurns = 2
	cfg.Hooks.Events["llm.after"] = []config.HookCommand{{Command: agentHookHelperCommand("llm_after_drop_tools"), FailPolicy: "fail"}}
	r := newModelsSettingsRouter(cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"list","session_id":"llm-after-tools-hook"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", w.Code, w.Body.String())
	}
	if !seenToolResult {
		t.Fatalf("expected original tool call to execute despite llm.after tool_calls mutation")
	}
}
