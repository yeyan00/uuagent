package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
)

func TestRunStreamsReasoningDeltasAndSendsReasoningOptions(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	var requestBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think-a\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_text\":\"think-b\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	cfg.Agent.ReasoningEnabled = true
	cfg.Agent.ReasoningEffort = "high"
	a := agent.New(cfg)
	events, err := a.Run(context.Background(), "s-reasoning", "hi")
	if err != nil {
		t.Fatal(err)
	}
	var reasoning, content string
	for evt := range events {
		if evt.Type == "reasoning" {
			reasoning += evt.Text
		}
		if evt.Type == "content" {
			content += evt.Text
		}
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if reasoning != "think-athink-b" {
		t.Fatalf("expected reasoning deltas, got %q", reasoning)
	}
	if content != "answer" {
		t.Fatalf("expected answer content, got %q", content)
	}
	if requestBody["reasoning_effort"] != "high" || requestBody["include_reasoning"] != true {
		t.Fatalf("expected reasoning options in request, got %#v", requestBody)
	}
}

func TestRunStreamsContentDeltas(t *testing.T) {
	t.Setenv("UUAGENT_HOME", t.TempDir())
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	cfg := config.Default()
	cfg.Agent.ProxyURL = ts.URL + "/v1"
	a := agent.New(cfg)
	events, err := a.Run(context.Background(), "s-stream", "hi")
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for evt := range events {
		if evt.Type == "content" {
			got += evt.Text
		}
		if evt.Type == "error" {
			t.Fatal(evt.Text)
		}
	}
	if got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}
