package subagent_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/router"
	"github.com/yeyan00/uuagent/internal/subagent"
)

func TestDelegateRunsSubagentsWithMockLLM(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"subagent-ok"}}]}`))
	}))
	defer ts.Close()
	t.Setenv("UUAGENT_API_KEY", "test-key")

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	parent := agent.New(cfg)

	m := subagent.NewManager(config.SubagentConfig{MaxConcurrent: 2, BlockedTools: []string{"shell"}}, router.New(config.Default().Agent.Routing))
	results := m.Delegate(parent, []string{"a", "b"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if res.ID == "" || res.Goal == "" || res.Model == "" {
			t.Fatalf("incomplete result: %+v", res)
		}
		if res.Error != "" {
			t.Fatalf("unexpected subagent error: %s", res.Error)
		}
		if !strings.Contains(res.Output, "subagent-ok") {
			t.Fatalf("unexpected subagent output: %s", res.Output)
		}
	}
}
