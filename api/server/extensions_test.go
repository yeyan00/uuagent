package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/extensions"
)

func TestExtensionsAPIUsesUserPluginDirectory(t *testing.T) {
	// Given
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("UUAGENT_HOME", home)
	resetExtensionManagerForTest()
	binaryName := "cli-proxy-api"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(home, "plugins", "cliproxyapi", binaryName)
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		t.Fatalf("create user plugin dir: %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("fake"), 0755); err != nil {
		t.Fatalf("write fake user plugin binary: %v", err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r.Group("/api"), agent.New(config.Default()))

	// When
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/extensions/cliproxyapi", nil))

	// Then
	if rec.Code != http.StatusOK {
		t.Fatalf("get extension status=%d body=%s", rec.Code, rec.Body.String())
	}
	var status extensions.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode extension status: %v", err)
	}
	if !status.Installed || status.Status != extensions.StatusStopped {
		t.Fatalf("expected installed stopped user plugin status, got %+v", status)
	}
	if status.BinaryPath != binaryPath {
		t.Fatalf("expected binary path %q, got %q", binaryPath, status.BinaryPath)
	}
}

func resetExtensionManagerForTest() {
	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	if cliProxyAPIManager != nil {
		_, _ = cliProxyAPIManager.Stop(nil)
	}
	cliProxyAPIManager = nil
}
