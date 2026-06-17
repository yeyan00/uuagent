package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/types"
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

func TestProjectRunExecutesToolsInProjectWorkspace(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "project-marker.txt"), []byte("marker"), 0600); err != nil {
		t.Fatal(err)
	}

	calls := 0
	var toolResult string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Need listing.","tool_calls":[{"id":"tc-list","type":"function","function":{"name":"list_dir","arguments":"{}"}}]}}]}`))
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
	a := agent.New(cfg)
	events, err := a.RunWithAgentProjectParts(context.Background(), "project-tool-workspace", "", workspace, []types.ContentPart{{Type: "text", Text: "list current directory"}})
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if !strings.Contains(toolResult, "project-marker.txt") {
		t.Fatalf("tool should list selected project workspace, got %q", toolResult)
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
		t.Fatalf("approval-required tool result should pause the run before a second LLM call")
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
		if evt.Type == "tool_result" {
			toolResult = evt.Text
		}
	}
	if !strings.Contains(toolResult, `"approval_required":true`) || !strings.Contains(toolResult, `"tool":"read"`) {
		t.Fatalf("expected approval payload tool result, got %s", toolResult)
	}
	if calls != 1 {
		t.Fatalf("expected run to pause after approval request, got %d LLM calls", calls)
	}
}

func TestApproveRunContinuesFullToolLoopAfterApprovedTool(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	outside := filepath.Join(t.TempDir(), "outside.txt")
	inside := "inside-fixture.txt"
	if err := os.WriteFile(outside, []byte("approved-ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("module test-project"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(inside) })
	calls := 0
	var approvedToolContent string
	var followupToolContent string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			args, _ := json.Marshal(map[string]string{"path": outside})
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-read-outside", "type": "function", "function": map[string]any{"name": "read", "arguments": string(args)}}}}}}})
		case 2:
			approvedToolContent = lastToolContent(t, r)
			args, _ := json.Marshal(map[string]string{"path": inside})
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-read-inside", "type": "function", "function": map[string]any{"name": "read", "arguments": string(args)}}}}}}})
		default:
			followupToolContent = lastToolContent(t, r)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"final analysis after reading files"}}]}`))
		}
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{ID: "asker", Name: "Asker", PermissionMode: "ask"}}
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "approval-resume", "asker", "read outside")
	if err != nil {
		t.Fatal(err)
	}
	var runID string
	for evt := range events {
		if evt.Type == "run" {
			runID = evt.RunID
		}
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if calls != 1 || runID == "" {
		t.Fatalf("expected pending approval after first call, calls=%d runID=%q", calls, runID)
	}

	content, err := a.ApproveRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if content != "final analysis after reading files" {
		t.Fatalf("approve should continue full loop to final answer, got %q", content)
	}
	if calls != 3 {
		t.Fatalf("expected continuation and follow-up tool loop, got %d model calls", calls)
	}
	if approvedToolContent != "approved-ok" {
		t.Fatalf("expected approved tool output sent to model, got %q", approvedToolContent)
	}
	if !strings.Contains(followupToolContent, "module test-project") {
		t.Fatalf("expected follow-up tool output sent to model, got %q", followupToolContent)
	}
}

func lastToolContent(t *testing.T, r *http.Request) string {
	t.Helper()
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "tool" {
			return req.Messages[i].Content
		}
	}
	return ""
}

func TestApproveRunEventsEmitsContinuationToolAndContentEvents(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	outside := filepath.Join(t.TempDir(), "outside.txt")
	inside := "inside-events-fixture.txt"
	if err := os.WriteFile(outside, []byte("approved-events-ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("events-fixture-content"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(inside) })
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			args, _ := json.Marshal(map[string]string{"path": outside})
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-outside", "type": "function", "function": map[string]any{"name": "read", "arguments": string(args)}}}}}}})
		case 2:
			args, _ := json.Marshal(map[string]string{"path": inside})
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-inside", "type": "function", "function": map[string]any{"name": "read", "arguments": string(args)}}}}}}})
		default:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"streamed final answer"}}]}`))
		}
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{ID: "asker", Name: "Asker", PermissionMode: "ask"}}
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "approval-events", "asker", "read outside")
	if err != nil {
		t.Fatal(err)
	}
	var runID string
	for evt := range events {
		if evt.Type == "run" {
			runID = evt.RunID
		}
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if runID == "" {
		t.Fatal("missing run id")
	}

	approvalEvents, err := a.ApproveRunEvents(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for evt := range approvalEvents {
		seen = append(seen, evt.Type+":"+evt.ToolName+":"+evt.Text)
	}
	joined := strings.Join(seen, "\n")
	for _, want := range []string{"tool_start:read:", "tool_result:read:approved-events-ok", "tool_result:read:events-fixture-content", "content::streamed final answer", "done::"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected event %q in\n%s", want, joined)
		}
	}
}

func TestAskPermissionToolDescriptionsTellModelToRequestApproval(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	var descriptions []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tools []struct {
				Function struct {
					Name        string `json:"name"`
					Description string `json:"description"`
				} `json:"function"`
			} `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, tool := range req.Tools {
			if tool.Function.Name == "read" || tool.Function.Name == "list_dir" {
				descriptions = append(descriptions, tool.Function.Description)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{ID: "default", Name: "Default", EnabledTools: []string{"read", "list_dir"}, PermissionMode: "ask"}}
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "approval-tool-descriptions", "default", "analyze C:\\outside")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}

	joined := strings.ToLower(strings.Join(descriptions, "\n"))
	if !strings.Contains(joined, "approval_required") || !strings.Contains(joined, "absolute") {
		t.Fatalf("ask-mode tool descriptions should tell model external absolute paths request approval, got %q", joined)
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
		if evt.Type == "tool_result" {
			toolResult = evt.Text
		}
	}
	if !strings.Contains(toolResult, `"approval_required":true`) || !strings.Contains(toolResult, `"tool":"read"`) {
		t.Fatalf("expected approval payload tool result, got %s", toolResult)
	}
}

func TestWorkspaceWritePermissionReturnsApprovalPayload(t *testing.T) {
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
	cfg.Agents = []config.AgentProfile{{ID: "default", Name: "Default", PermissionMode: "workspace-write"}}
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "approval-workspace-write", "default", "read outside")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
		if evt.Type == "tool_result" {
			toolResult = evt.Text
		}
	}
	if !strings.Contains(toolResult, `"approval_required":true`) || !strings.Contains(toolResult, `"tool":"read"`) {
		t.Fatalf("expected approval payload for workspace-write, got %s", toolResult)
	}
}
