package agent_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/api/server"
	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/session"
)

func TestStopRunCancelsStreamingChatAndPersistsSessionJSON(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("UUAGENT_HOME", home)
	gin.SetMode(gin.TestMode)

	llmStarted := make(chan struct{})
	llmClosed := make(chan struct{})
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(llmStarted)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
		close(llmClosed)
	}))
	defer llm.Close()

	cfg := config.Default()
	cfg.Agent.ProxyURL = llm.URL + "/v1"
	agt := agent.New(cfg)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agt)
	api := httptest.NewServer(r)
	defer api.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.URL+"/api/chat?prompt=long-task&session_id=stop-session&agent_id=default", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("chat status=%d", resp.StatusCode)
	}

	runID := readRunID(t, resp.Body)
	if runID == "" {
		t.Fatal("expected run_id event")
	}

	select {
	case <-llmStarted:
	case <-ctx.Done():
		t.Fatal("mock LLM was not reached")
	}

	stopReq, err := http.NewRequestWithContext(ctx, http.MethodPost, api.URL+"/api/runs/"+runID+"/stop", nil)
	if err != nil {
		t.Fatal(err)
	}
	stopResp, err := http.DefaultClient.Do(stopReq)
	if err != nil {
		t.Fatal(err)
	}
	defer stopResp.Body.Close()
	if stopResp.StatusCode != http.StatusOK {
		t.Fatalf("stop status=%d", stopResp.StatusCode)
	}

	select {
	case <-llmClosed:
	case <-time.After(time.Second):
		t.Fatal("StopRun did not cancel the upstream LLM request quickly")
	}

	data, err := os.ReadFile(filepath.Join(home, "sessions", "stop-session.json"))
	if err != nil {
		t.Fatalf("read session json: %v", err)
	}
	var stored session.Session
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("parse session json: %v", err)
	}
	if stored.ID != "stop-session" {
		t.Fatalf("unexpected session id: %s", stored.ID)
	}
	if len(stored.Messages) != 1 || stored.Messages[0].Role != "user" || stored.Messages[0].Content != "long-task" {
		t.Fatalf("stopped run should persist the user prompt only, got %+v", stored.Messages)
	}
	if len(stored.Runs) != 1 || stored.Runs[0].ID == "" || stored.Runs[0].Prompt != "long-task" {
		t.Fatalf("run metadata not persisted: %+v", stored.Runs)
	}
}

func readRunID(t *testing.T, body any) string {
	t.Helper()
	reader, ok := body.(interface{ Read([]byte) (int, error) })
	if !ok {
		t.Fatalf("body is not readable")
	}
	scanner := bufio.NewScanner(reader)
	deadline := time.After(5 * time.Second)
	lines := make(chan string, 8)
	go func() {
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("stream ended before run event: %v", scanner.Err())
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var evt struct {
				Type  string `json:"type"`
				RunID string `json:"run_id"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &evt); err != nil {
				continue
			}
			if evt.Type == "run" {
				return evt.RunID
			}
		case <-deadline:
			t.Fatal("timed out waiting for run event")
		}
	}
}
