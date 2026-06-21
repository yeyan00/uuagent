package agent_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/api/server"
	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
)

type modelsSettingsResponse struct {
	ProxyURL     string              `json:"proxy_url"`
	RoutingTiers map[string][]string `json:"routing_tiers"`
	FallbackTier string              `json:"fallback_tier"`
	ModelIDs     []string            `json:"model_ids"`
}

type modelsTestResponse struct {
	Success  bool     `json:"success"`
	ModelIDs []string `json:"model_ids"`
	Error    string   `json:"error"`
}

func TestModelsSettingsAPI_gets_current_settings_and_flattened_model_ids(t *testing.T) {
	// Given
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	cfg := config.Default()
	cfg.Agent.ProxyURL = "http://proxy.local/v1"
	cfg.Agent.Routing.Fallback = "strong"
	cfg.Agent.Routing.Tiers = map[string][]string{
		"fast":   {"fast-a", "shared-model"},
		"strong": {"strong-a", "shared-model"},
	}
	r := newModelsSettingsRouter(cfg)

	// When
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/models/settings", nil))

	// Then
	if w.Code != http.StatusOK {
		t.Fatalf("settings status=%d body=%s", w.Code, w.Body.String())
	}
	var got modelsSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ProxyURL != "http://proxy.local/v1" || got.FallbackTier != "strong" {
		t.Fatalf("unexpected settings: %+v", got)
	}
	if strings.Contains(w.Body.String(), "api_key") || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("settings response must not expose secrets: %s", w.Body.String())
	}
	wantModels := map[string]bool{"fast-a": true, "strong-a": true, "shared-model": true}
	if len(got.ModelIDs) != len(wantModels) {
		t.Fatalf("model_ids should be flattened and deduplicated, got %+v", got.ModelIDs)
	}
	for _, id := range got.ModelIDs {
		if !wantModels[id] {
			t.Fatalf("unexpected model id %q in %+v", id, got.ModelIDs)
		}
	}
}

func TestModelsSettingsAPI_put_persists_settings_and_reloads_router(t *testing.T) {
	// Given
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("UUAGENT_HOME", home)
	r := newModelsSettingsRouter(config.Default())
	body := `{"proxy_url":"https://updated.example.com/v1","routing_tiers":{"fast":["fast-new"],"strong":["strong-new"]},"fallback_tier":"fast","api_key":"must-not-save"}`

	// When
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Then
	if w.Code != http.StatusOK {
		t.Fatalf("put settings status=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/models/settings", nil))
	var got modelsSettingsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ProxyURL != "https://updated.example.com/v1" || got.FallbackTier != "fast" || got.RoutingTiers["fast"][0] != "fast-new" {
		t.Fatalf("runtime settings were not reloaded: %+v", got)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/route?prompt=hello", nil))
	if !strings.Contains(w.Body.String(), "fast-new") || !strings.Contains(w.Body.String(), "fast") {
		t.Fatalf("router did not use updated fallback tier: %s", w.Body.String())
	}
	saved, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "https://updated.example.com/v1") || !strings.Contains(string(saved), "fast-new") {
		t.Fatalf("config.yaml missing saved settings: %s", string(saved))
	}
	if strings.Contains(string(saved), "must-not-save") {
		t.Fatalf("config.yaml must not persist API keys/secrets: %s", string(saved))
	}
}

func TestModelsTestAPI_returns_model_ids_from_openai_compatible_models_endpoint(t *testing.T) {
	// Given
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-a"},{"id":"model-b"}]}`))
	}))
	defer upstream.Close()
	r := newModelsSettingsRouter(config.Default())

	// When
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/test", strings.NewReader(`{"proxy_url":"`+upstream.URL+`/v1"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Then
	if w.Code != http.StatusOK {
		t.Fatalf("test models status=%d body=%s", w.Code, w.Body.String())
	}
	var got modelsTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Success || strings.Join(got.ModelIDs, ",") != "model-a,model-b" {
		t.Fatalf("unexpected models test response: %+v", got)
	}
}

func TestModelsTestAPI_returns_clear_error_for_non_2xx_response(t *testing.T) {
	// Given
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"bad gateway"}`))
	}))
	defer upstream.Close()
	r := newModelsSettingsRouter(config.Default())

	// When
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/test", strings.NewReader(`{"proxy_url":"`+upstream.URL+`/v1"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Then
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected upstream failure status, got status=%d body=%s", w.Code, w.Body.String())
	}
	var got modelsTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Success || !strings.Contains(got.Error, "status=502") {
		t.Fatalf("expected clear non-2xx error, got %+v", got)
	}
}

func newModelsSettingsRouter(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agent.New(cfg))
	return r
}

func TestModelsTestAPI_rejects_invalid_proxy_url(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	cfg := config.Default()
	r := newModelsSettingsRouter(cfg)

	body := `{"proxy_url":"not-a-valid-url"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid URL, got %d body=%s", w.Code, w.Body.String())
	}
	var resp modelsTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Error, "invalid URL") {
		t.Fatalf("expected error about invalid URL, got: %s", resp.Error)
	}
}

func TestModelsTestAPI_rejects_private_ip_proxy_url(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	cfg := config.Default()
	r := newModelsSettingsRouter(cfg)

	body := `{"proxy_url":"http://192.168.1.1/v1"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for private IP URL, got %d body=%s", w.Code, w.Body.String())
	}
	var resp modelsTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Error, "private IP") && !strings.Contains(resp.Error, "not allowed") {
		t.Fatalf("expected error about private IP not allowed, got: %s", resp.Error)
	}
}

func TestModelsTestAPI_redacts_upstream_response_body_in_error(t *testing.T) {
	// Loopback addresses should be allowed, and error responses should be redacted
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_api_key_12345","message":"Authentication failed with key sk-abc123"}`))
	}))
	defer upstream.Close()

	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	cfg := config.Default()
	r := newModelsSettingsRouter(cfg)

	body := `{"proxy_url":"` + upstream.URL + `/v1"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Loopback should be allowed, so request reaches upstream and returns 401
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 from upstream, got %d body=%s", w.Code, w.Body.String())
	}
	var resp modelsTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	// Verify sensitive data is redacted
	if strings.Contains(resp.Error, "invalid_api_key_12345") || strings.Contains(resp.Error, "sk-abc123") {
		t.Fatalf("error should not contain sensitive upstream data, got: %s", resp.Error)
	}
	if !strings.Contains(resp.Error, "[upstream error redacted]") {
		t.Fatalf("expected redacted error message, got: %s", resp.Error)
	}
}

func TestModelsSettingsPut_rejects_http_hostname_proxy_url(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	cfg := config.Default()
	r := newModelsSettingsRouter(cfg)

	body := `{"proxy_url":"http://updated.local/v1","routing_tiers":{"fast":["gpt-4o-mini"]},"fallback_tier":"fast"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for HTTP hostname URL, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp["error"], "not allowed") && !strings.Contains(resp["error"], "loopback") {
		t.Fatalf("expected error about non-loopback HTTP not allowed, got: %s", resp["error"])
	}
}

func TestModelsSettingsPut_rejects_http_private_ip_proxy_url(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	cfg := config.Default()
	r := newModelsSettingsRouter(cfg)

	body := `{"proxy_url":"http://192.168.1.10/v1","routing_tiers":{"fast":["gpt-4o-mini"]},"fallback_tier":"fast"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for HTTP private IP URL, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp["error"], "not allowed") && !strings.Contains(resp["error"], "private") {
		t.Fatalf("expected error about private IP not allowed, got: %s", resp["error"])
	}
}

func TestModelsSettingsPut_accepts_http_loopback_proxy_url(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	cfg := config.Default()
	r := newModelsSettingsRouter(cfg)

	body := `{"proxy_url":"http://localhost:8080/v1","routing_tiers":{"fast":["gpt-4o-mini"]},"fallback_tier":"fast"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for HTTP localhost URL, got %d body=%s", w.Code, w.Body.String())
	}

	body = `{"proxy_url":"http://127.0.0.1:8080/v1","routing_tiers":{"fast":["gpt-4o-mini"]},"fallback_tier":"fast"}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/models/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for HTTP 127.0.0.1 URL, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelsSettingsPut_accepts_https_any_host_proxy_url(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	cfg := config.Default()
	r := newModelsSettingsRouter(cfg)

	body := `{"proxy_url":"https://example.com/v1","routing_tiers":{"fast":["gpt-4o-mini"]},"fallback_tier":"fast"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for HTTPS URL, got %d body=%s", w.Code, w.Body.String())
	}

	body = `{"proxy_url":"https://192.168.1.10/v1","routing_tiers":{"fast":["gpt-4o-mini"]},"fallback_tier":"fast"}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/models/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for HTTPS private IP URL, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestModelsTestAPI_rejects_http_hostname_proxy_url(t *testing.T) {
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	cfg := config.Default()
	r := newModelsSettingsRouter(cfg)

	body := `{"proxy_url":"http://updated.local/v1"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for HTTP hostname URL, got %d body=%s", w.Code, w.Body.String())
	}
	var resp modelsTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Error, "not allowed") && !strings.Contains(resp.Error, "loopback") {
		t.Fatalf("expected error about non-loopback HTTP not allowed, got: %s", resp.Error)
	}
}
