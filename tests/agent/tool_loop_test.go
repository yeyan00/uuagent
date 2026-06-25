package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		runAgentHookHelper()
		return
	}
	os.Exit(m.Run())
}

func TestToolExecuteBeforeHookMutatesArgs(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "hooked.txt"), []byte("hooked content"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	var toolResult string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Need file.","tool_calls":[{"id":"tc-read","type":"function","function":{"name":"read","arguments":"{\"path\":\"original.txt\"}"}}]}}]}`))
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
	cfg.Hooks.Events["tool.execute.before"] = []config.HookCommand{{Command: agentHookHelperCommand("tool_before_path"), FailPolicy: "fail"}}
	a := agent.New(cfg)
	events, err := a.RunWithAgentProjectParts(context.Background(), "tool-before-hook", "", workspace, []types.ContentPart{{Type: "text", Text: "read file"}})
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if !strings.Contains(toolResult, "hooked content") {
		t.Fatalf("expected hook-mutated tool result, got %q", toolResult)
	}
}

func TestToolExecuteAfterHookMutatesOutput(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("raw content"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	var toolResult string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Need file.","tool_calls":[{"id":"tc-read","type":"function","function":{"name":"read","arguments":"{\"path\":\"file.txt\"}"}}]}}]}`))
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
	cfg.Hooks.Events["tool.execute.after"] = []config.HookCommand{{Command: agentHookHelperCommand("tool_after_output"), FailPolicy: "fail"}}
	a := agent.New(cfg)
	events, err := a.RunWithAgentProjectParts(context.Background(), "tool-after-hook", "", workspace, []types.ContentPart{{Type: "text", Text: "read file"}})
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if toolResult != "redacted by hook" {
		t.Fatalf("expected hook-mutated output, got %q", toolResult)
	}
}

func TestToolExecuteBeforeHookFailPolicyBlocksTool(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("raw content"), 0600); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Need file.","tool_calls":[{"id":"tc-read","type":"function","function":{"name":"read","arguments":"{\"path\":\"file.txt\"}"}}]}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agent.MaxTurns = 2
	cfg.Hooks.Events["tool.execute.before"] = []config.HookCommand{{Command: agentHookHelperCommand("exit_2"), FailPolicy: "fail"}}
	a := agent.New(cfg)
	events, err := a.RunWithAgentProjectParts(context.Background(), "tool-before-fail", "", workspace, []types.ContentPart{{Type: "text", Text: "read file"}})
	if err != nil {
		t.Fatal(err)
	}
	var gotError string
	for evt := range events {
		if evt.Type == "error" {
			gotError = evt.Text
		}
	}
	if !strings.Contains(gotError, "hook command failed") {
		t.Fatalf("expected hook failure error, got %q", gotError)
	}
}

func agentHookHelperCommand(mode string) string {
	return fmt.Sprintf("%s --agent-hook-helper %s", os.Args[0], mode)
}

func runAgentHookHelper() {
	if len(os.Args) < 3 || os.Args[1] != "--agent-hook-helper" {
		os.Exit(2)
	}
	var payload map[string]any
	_ = json.NewDecoder(os.Stdin).Decode(&payload)
	switch os.Args[2] {
	case "tool_before_path":
		fmt.Print(`{"args":{"path":"hooked.txt"}}`)
	case "tool_after_output":
		fmt.Print(`{"output":"redacted by hook"}`)
	case "chat_headers":
		fmt.Print(`{"headers":{"X-Hook-Trace":"trace-123"}}`)
	case "chat_params":
		fmt.Print(`{"params":{"temperature":0.25,"max_tokens":77}}`)
	case "llm_after_response":
		fmt.Print(`{"response":"mutated by hook"}`)
	case "llm_after_drop_tools":
		fmt.Print(`{"response":"mutated text with original tool calls","tool_calls":[]}`)
	case "autocontinue_false":
		fmt.Print(`{"continue":false}`)
	case "exit_2":
		fmt.Fprint(os.Stderr, "blocked by hook")
		os.Exit(2)
	default:
		os.Exit(2)
	}
}

func TestAgentToolLoopSendsToolResultBackToModel(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	calls := 0
	var secondMessages []map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Need listing.","tool_calls":[{"id":"tc-list","type":"function","function":{"name":"ls","arguments":"{\"path\":\"internal\"}"}}]}}]}`))
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
					foundOpenAIToolCall = hasFunction && fn["name"] == "ls" && fn["arguments"] != "" && !hasLegacyArgs && !hasLegacyName
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

func TestAgentMaxTurnsDisablesToolsOnFinalProviderTurn(t *testing.T) {
	// Given
	t.Setenv("UUAGENT_HOME", t.TempDir())
	calls := 0
	secondToolsLen := -1
	secondToolChoice := ""
	executedForbiddenTool := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Need listing.","tool_calls":[{"id":"tc-list","type":"function","function":{"name":"ls","arguments":"{\"path\":\"internal\"}"}}]}}]}`))
			return
		}
		var req struct {
			Tools      []map[string]any `json:"tools"`
			ToolChoice any              `json:"tool_choice"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode second request: %v", err)
		}
		secondToolsLen = len(req.Tools)
		if text, ok := req.ToolChoice.(string); ok {
			secondToolChoice = text
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"I should not call tools.","tool_calls":[{"id":"tc-forbidden","type":"function","function":{"name":"ls","arguments":"{\"path\":\".\"}"}}]}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agent.MaxTurns = 2
	a := agent.New(cfg)

	// When
	events, err := a.RunWithAgent(context.Background(), "max-turn-no-tools", "", "list once then summarize")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "tool_start" && evt.ToolID == "tc-forbidden" {
			executedForbiddenTool = true
		}
	}

	// Then
	if calls != 2 {
		t.Fatalf("expected 2 provider turns, got %d", calls)
	}
	if secondToolsLen != 0 {
		t.Fatalf("final provider turn should not advertise tools, got %d tools", secondToolsLen)
	}
	if secondToolChoice != "none" {
		t.Fatalf("final provider turn should force tool_choice none, got %q", secondToolChoice)
	}
	if executedForbiddenTool {
		t.Fatalf("tool emitted on final provider turn must not execute")
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
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Need listing.","tool_calls":[{"id":"tc-list","type":"function","function":{"name":"ls","arguments":"{}"}}]}}]}`))
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

func toolContentsByID(t *testing.T, r *http.Request) map[string]string {
	t.Helper()
	var req struct {
		Messages []struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolCallID string `json:"tool_call_id"`
		} `json:"messages"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	contents := map[string]string{}
	for _, msg := range req.Messages {
		if msg.Role == "tool" {
			contents[msg.ToolCallID] = msg.Content
		}
	}
	return contents
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

func TestApproveRunExecutesRemainingToolCallsFromSameAssistantTurn(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	outside := filepath.Join(t.TempDir(), "outside.txt")
	inside := "same-turn-inside.txt"
	if err := os.WriteFile(outside, []byte("outside-approved"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("inside-same-turn"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(inside) })
	calls := 0
	var continuationTools map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			outsideArgs, _ := json.Marshal(map[string]string{"path": outside})
			insideArgs, _ := json.Marshal(map[string]string{"path": inside})
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{
				map[string]any{"id": "tc-outside", "type": "function", "function": map[string]any{"name": "read", "arguments": string(outsideArgs)}},
				map[string]any{"id": "tc-inside", "type": "function", "function": map[string]any{"name": "read", "arguments": string(insideArgs)}},
			}}}}})
			return
		}
		continuationTools = toolContentsByID(t, r)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done with both tools"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{ID: "asker", Name: "Asker", PermissionMode: "ask"}}
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "approval-same-turn-remaining", "asker", "read both")
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
	if _, err := a.ApproveRun(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected one continuation model call, got %d", calls)
	}
	if continuationTools["tc-outside"] != "outside-approved" || !strings.Contains(continuationTools["tc-inside"], "inside-same-turn") {
		t.Fatalf("continuation should include all same-turn tool results, got %#v", continuationTools)
	}
}

func TestApproveRunDoesNotSendApprovalPayloadAsToolHistory(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("approved-content"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	var continuationToolMessages []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			args, _ := json.Marshal(map[string]string{"path": outside})
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-read-outside", "type": "function", "function": map[string]any{"name": "read", "arguments": string(args)}}}}}}})
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
				continuationToolMessages = append(continuationToolMessages, msg.Content)
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{ID: "asker", Name: "Asker", PermissionMode: "ask"}}
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "approval-history", "asker", "read outside")
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
	if _, err := a.ApproveRun(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	if len(continuationToolMessages) != 1 {
		t.Fatalf("expected one real tool result in continuation history, got %#v", continuationToolMessages)
	}
	if strings.Contains(continuationToolMessages[0], `"approval_required":true`) || continuationToolMessages[0] != "approved-content" {
		t.Fatalf("approval payload should not be sent as tool history, got %#v", continuationToolMessages)
	}
}

func TestApprovedOutsideDirectoryAllowsChildFileDuringSameRun(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	outsideDir := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(outsideDir, "go.mod")
	if err := os.WriteFile(child, []byte("module outside-child"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			args, _ := json.Marshal(map[string]string{"path": outsideDir})
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-list-outside", "type": "function", "function": map[string]any{"name": "ls", "arguments": string(args)}}}}}}})
		case 2:
			args, _ := json.Marshal(map[string]string{"path": child})
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-read-child", "type": "function", "function": map[string]any{"name": "read", "arguments": string(args)}}}}}}})
		default:
			last := lastToolContent(t, r)
			if strings.Contains(last, `"approval_required":true`) {
				t.Fatalf("child file under approved directory should not require another approval: %s", last)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done after child read"}}]}`))
		}
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{ID: "asker", Name: "Asker", PermissionMode: "ask"}}
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "approval-dir-child", "asker", "analyze outside dir")
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
	content, err := a.ApproveRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if content != "done after child read" {
		t.Fatalf("unexpected final content %q", content)
	}
	if calls != 3 {
		t.Fatalf("expected one approval then automatic child read continuation, got %d calls", calls)
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
			if tool.Function.Name == "read" || tool.Function.Name == "ls" {
				descriptions = append(descriptions, tool.Function.Description)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{ID: "default", Name: "Default", EnabledTools: []string{"read", "ls"}, PermissionMode: "ask"}}
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
