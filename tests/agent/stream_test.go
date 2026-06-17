package agent_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
)

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
