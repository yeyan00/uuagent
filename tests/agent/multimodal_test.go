package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/types"
)

func TestRunWithAgentPartsSendsImageURL(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	var gotContent any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotContent = req.Messages[len(req.Messages)-1].Content
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	a := agent.New(cfg)
	events, err := a.RunWithAgentParts(context.Background(), "mm", "", []types.ContentPart{
		{Type: "text", Text: "describe"},
		{Type: "image_url", ImageURL: &types.ImageURL{URL: "data:image/png;base64,AAECAw=="}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for evt := range events {
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	parts, ok := gotContent.([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("expected multimodal array, got %#v", gotContent)
	}
	img, _ := parts[1].(map[string]any)
	if img["type"] != "image_url" {
		t.Fatalf("expected image_url part, got %#v", img)
	}
}
