package agent_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeyan00/uuagent/internal/config"
)

type routeDecisionResponse struct {
	SelectedModel string `json:"selected_model"`
	SelectedTier  string `json:"selected_tier"`
	Source        string `json:"source"`
	RuleName      string `json:"rule_name"`
	Reason        string `json:"reason"`
}

func TestRouteEndpointReturnsDecisionSourceAndRule_whenPromptMatchesFastSimple(t *testing.T) {
	// Given
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	cfg := config.Default()
	cfg.Agent.Routing.Tiers = map[string][]string{
		"fast":   {"fast-model"},
		"strong": {"strong-model"},
	}
	r := newModelsSettingsRouter(cfg)

	// When
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/route?prompt=format%20this", nil))

	// Then
	if w.Code != http.StatusOK {
		t.Fatalf("route status=%d body=%s", w.Code, w.Body.String())
	}
	var got routeDecisionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SelectedModel != "fast-model" || got.SelectedTier != "fast" || got.Source != "rule" || got.RuleName != "fast-simple" {
		t.Fatalf("unexpected route decision: %+v body=%s", got, w.Body.String())
	}
	if !strings.Contains(got.Reason, "format") {
		t.Fatalf("expected reason to explain matched pattern, got %q", got.Reason)
	}
}

func TestChatRequestHonorsModelOverride_whenConcreteModelProvided(t *testing.T) {
	// Given
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	seenModel := ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		seenModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer upstream.Close()
	cfg := config.Default()
	cfg.Agent.ProxyURL = upstream.URL + "/v1"
	cfg.Agent.Routing.Tiers = map[string][]string{"fast": {"fast-model"}, "strong": {"strong-model"}}
	r := newModelsSettingsRouter(cfg)

	// When
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"format this","session_id":"manual-session","model_override":"manual-model"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Then
	if w.Code != http.StatusOK {
		t.Fatalf("chat status=%d body=%s", w.Code, w.Body.String())
	}
	if seenModel != "manual-model" {
		t.Fatalf("expected manual-model upstream, got %q", seenModel)
	}
	if !strings.Contains(w.Body.String(), `"source":"manual"`) {
		t.Fatalf("expected route event to report manual source, body=%s", w.Body.String())
	}
}

func TestChatRequestUsesAutomaticRouting_whenModelOverrideAutoOrEmpty(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "auto override", body: `{"prompt":"format this","session_id":"auto-session","model_override":"auto"}`},
		{name: "empty override", body: `{"prompt":"format this","session_id":"empty-session"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
			seenModel := ""
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode upstream request: %v", err)
				}
				seenModel = body.Model
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
			}))
			defer upstream.Close()
			cfg := config.Default()
			cfg.Agent.ProxyURL = upstream.URL + "/v1"
			cfg.Agent.Routing.Tiers = map[string][]string{"fast": {"fast-model"}, "strong": {"strong-model"}}
			r := newModelsSettingsRouter(cfg)

			// When
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			// Then
			if w.Code != http.StatusOK {
				t.Fatalf("chat status=%d body=%s", w.Code, w.Body.String())
			}
			if seenModel != "fast-model" {
				t.Fatalf("expected automatic fast-model upstream, got %q", seenModel)
			}
			if strings.Contains(w.Body.String(), `"source":"manual"`) {
				t.Fatalf("auto/empty override must not force manual route, body=%s", w.Body.String())
			}
		})
	}
}
