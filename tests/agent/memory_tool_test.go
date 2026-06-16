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
	"github.com/yeyan00/uuagent/internal/memory"
	"github.com/yeyan00/uuagent/internal/types"
)

func TestMemoryToolPersistsWithoutMutatingFrozenPrompt(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	calls := 0
	var prompts []string
	var memoryToolResult string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
			prompts = append(prompts, req.Messages[0].Content)
		} else {
			prompts = append(prompts, "")
		}
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"first"}}]}`))
		case 2:
			args := mustJSON(map[string]string{"action": "add", "content": "tool saved memory", "project": "project-a", "scope": "project", "status": "confirmed"})
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "tc-memory", "type": "function", "function": map[string]any{"name": "memory", "arguments": args}}}}}}})
		case 3:
			for _, msg := range req.Messages {
				if msg.Role == "tool" {
					memoryToolResult = msg.Content
				}
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"saved"}}]}`))
		default:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"later"}}]}`))
		}
	}))
	defer llm.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	cfg.Agent.Memory.AutoDraft = false
	agt := agent.New(cfg)
	agt.Memories().Add("initial memory", "project-a", "project", "user", memory.StatusConfirmed)

	runAgentTurn(t, agt, "mem-tool", "project-a", "first")
	runAgentTurn(t, agt, "mem-tool", "project-a", "please remember")
	if !strings.Contains(memoryToolResult, `"success":true`) || !strings.Contains(memoryToolResult, "tool saved memory") {
		t.Fatalf("unexpected memory tool result: %s", memoryToolResult)
	}
	confirmed := agt.Memories().ListFiltered(memory.StatusConfirmed, "project-a", "project")
	found := false
	for _, entry := range confirmed {
		if entry.Content == "tool saved memory" {
			found = true
		}
	}
	if !found {
		t.Fatalf("memory tool did not persist confirmed memory: %+v", confirmed)
	}

	runAgentTurn(t, agt, "mem-tool", "project-a", "next turn")
	if len(prompts) < 4 {
		t.Fatalf("expected at least four model prompts, got %#v", prompts)
	}
	if strings.Contains(prompts[len(prompts)-1], "tool saved memory") {
		t.Fatalf("memory tool write should not mutate current session frozen snapshot: %q", prompts[len(prompts)-1])
	}
}

func TestAutoDraftMemoryExtractionRunsAfterFinalAnswerWithoutExtraLLMCall(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	calls := 0
	var systemPrompt string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
			systemPrompt = req.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"noted"}}]}`))
	}))
	defer llm.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	cfg.Agent.Memory.AutoDraft = true
	agt := agent.New(cfg)
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatal(err)
	}
	runAgentTurn(t, agt, "auto-draft", project, "remember that my preferred test runner is go test ./...")
	if calls != 1 {
		t.Fatalf("auto draft should not make an extra LLM call, got %d calls", calls)
	}
	drafts := agt.Memories().ListFiltered(memory.StatusDraft, project, "project")
	found := false
	for _, entry := range drafts {
		if strings.Contains(entry.Content, "preferred test runner is go test ./...") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected heuristic draft memory, got %+v", drafts)
	}
	if strings.Contains(systemPrompt, "preferred test runner") {
		t.Fatalf("draft memory should not be injected into current prompt: %q", systemPrompt)
	}
	draftFile, err := os.ReadFile(filepath.Join(project, ".uuagent", "memory.draft.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(draftFile), "preferred test runner is go test ./...") {
		t.Fatalf("expected auto draft in markdown file, got %s", string(draftFile))
	}
}

func runAgentTurn(t *testing.T, agt *agent.Agent, sessionID, projectID, prompt string) {
	t.Helper()
	events, err := agt.RunWithAgentProjectParts(context.Background(), sessionID, "", projectID, []types.ContentPart{{Type: "text", Text: prompt}})
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
}
