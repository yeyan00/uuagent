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

func TestAutoCompactAddsSyntheticContinueAndKeepsRunning(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	calls := 0
	seenSyntheticContinue := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			var body struct {
				Messages []struct {
					Role    string         `json:"role"`
					Content string         `json:"content"`
					Meta    map[string]any `json:"metadata"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request: %v", err)
			}
			for _, msg := range body.Messages {
				if msg.Role == "user" && strings.Contains(msg.Content, "Continue if you have next steps") {
					seenSyntheticContinue = true
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + strings.Repeat("first compactable response ", 40) + `"}}],"usage":{"prompt_tokens":40,"completion_tokens":80,"total_tokens":120}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"final after compact"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer upstream.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = upstream.URL + "/v1"
	cfg.Agent.MaxTurns = 3
	cfg.Agent.Context.MaxTokens = 100
	cfg.Agent.Context.CompactReservedTokens = 90
	cfg.Agent.Context.KeepLastMessages = 2
	a := agent.New(cfg)
	sess := a.Sessions().GetOrCreate("auto-compact-continue")
	sess.Append("user", strings.Repeat("previous question ", 20))
	sess.Append("assistant", strings.Repeat("previous answer ", 20))

	events, err := a.Run(context.Background(), "auto-compact-continue", "continue")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}

	if calls < 2 {
		t.Fatalf("expected auto-compact to continue with a second LLM call, got %d", calls)
	}
	if !seenSyntheticContinue {
		t.Fatalf("expected second LLM request to include synthetic continue message")
	}
	snap := sess.Snapshot()
	if len(snap.Archives) == 0 {
		t.Fatalf("expected compact archive")
	}
}

func TestAutoCompactCanBeDisabledByEnv(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv("UUAGENT_DISABLE_AUTOCOMPACT", "1")
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + strings.Repeat("large response ", 40) + `"}}],"usage":{"prompt_tokens":40,"completion_tokens":80,"total_tokens":120}}`))
	}))
	defer upstream.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = upstream.URL + "/v1"
	cfg.Agent.MaxTurns = 3
	cfg.Agent.Context.MaxTokens = 100
	cfg.Agent.Context.CompactReservedTokens = 90
	cfg.Agent.Context.KeepLastMessages = 2
	a := agent.New(cfg)
	sess := a.Sessions().GetOrCreate("auto-compact-disabled")
	sess.Append("user", strings.Repeat("previous question ", 20))
	sess.Append("assistant", strings.Repeat("previous answer ", 20))

	events, err := a.Run(context.Background(), "auto-compact-disabled", "continue")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}

	if calls != 1 {
		t.Fatalf("expected one LLM call when auto-compact is disabled, got %d", calls)
	}
	if len(sess.Snapshot().Archives) != 0 {
		t.Fatalf("expected no compact archive when disabled")
	}
}

func TestCompactionAutoContinueHookCanDisableContinue(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + strings.Repeat("hook stops response ", 40) + `"}}],"usage":{"prompt_tokens":40,"completion_tokens":80,"total_tokens":120}}`))
	}))
	defer upstream.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = upstream.URL + "/v1"
	cfg.Agent.MaxTurns = 3
	cfg.Agent.Context.MaxTokens = 100
	cfg.Agent.Context.CompactReservedTokens = 90
	cfg.Agent.Context.KeepLastMessages = 2
	cfg.Hooks.Events["experimental.compaction.autocontinue"] = []config.HookCommand{{Command: agentHookHelperCommand("autocontinue_false"), FailPolicy: "fail"}}
	a := agent.New(cfg)
	sess := a.Sessions().GetOrCreate("auto-compact-hook-stop")
	sess.Append("user", strings.Repeat("previous question ", 20))
	sess.Append("assistant", strings.Repeat("previous answer ", 20))

	events, err := a.Run(context.Background(), "auto-compact-hook-stop", "continue")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}

	if calls != 1 {
		t.Fatalf("expected hook to stop continuation after compaction, got %d calls", calls)
	}
	if len(sess.Snapshot().Archives) == 0 {
		t.Fatalf("expected compaction archive before hook stopped continuation")
	}
}
