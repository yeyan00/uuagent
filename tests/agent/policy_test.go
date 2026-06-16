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

func TestAgentProfileToolPolicyFiltersDefinitions(t *testing.T) {
	var toolNames []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, tool := range req.Tools {
			toolNames = append(toolNames, tool.Function.Name)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agents = []config.AgentProfile{{ID: "limited", Name: "Limited", EnabledTools: []string{"read"}, EnabledMCPServers: []string{"none"}}}
	a := agent.New(cfg)
	events, err := a.RunWithAgent(context.Background(), "s-policy", "limited", "hi")
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if len(toolNames) != 1 || toolNames[0] != "read" {
		t.Fatalf("expected only read tool, got %v", toolNames)
	}
}
