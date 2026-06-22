package extensions_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/api/server"
	"github.com/yeyan00/uuagent/internal/agent"
	"github.com/yeyan00/uuagent/internal/config"
	"github.com/yeyan00/uuagent/internal/extensions"
)

func Test_CLIProxyAPI_Status_reports_missing_binary(t *testing.T) {
	// Given
	root := t.TempDir()
	manager := extensions.NewCLIProxyAPIManager(extensions.CLIProxyAPIOptions{
		PluginRoot: filepath.Join(root, "plugins"),
		DataRoot:   filepath.Join(root, "data"),
	})

	// When
	status := manager.Status(t.Context())

	// Then
	if status.ID != "cliproxyapi" {
		t.Fatalf("expected cliproxyapi id, got %q", status.ID)
	}
	if status.Installed {
		t.Fatalf("expected missing binary to be not installed")
	}
	if status.Status != extensions.StatusMissing {
		t.Fatalf("expected missing status, got %q", status.Status)
	}
	if status.BinaryPath == "" {
		t.Fatalf("expected binary path in status")
	}
	wantName := "cli-proxy-api"
	if runtime.GOOS == "windows" {
		wantName += ".exe"
	}
	wantSuffix := filepath.Join("plugins", "cliproxyapi", wantName)
	if !strings.HasSuffix(status.BinaryPath, wantSuffix) {
		t.Fatalf("binary path %q should end with %q", status.BinaryPath, wantSuffix)
	}
}

func Test_CLIProxyAPI_Start_returns_missing_status_when_binary_absent(t *testing.T) {
	// Given
	root := t.TempDir()
	manager := extensions.NewCLIProxyAPIManager(extensions.CLIProxyAPIOptions{
		PluginRoot: filepath.Join(root, "plugins"),
		DataRoot:   filepath.Join(root, "data"),
	})

	// When
	status, err := manager.Start(t.Context())

	// Then
	if err == nil {
		t.Fatalf("expected missing binary error")
	}
	if status.Status != extensions.StatusMissing || status.Installed {
		t.Fatalf("expected missing status from start, got %+v", status)
	}
	if !strings.Contains(err.Error(), "CLIProxyAPI binary missing") {
		t.Fatalf("expected clear missing binary error, got %v", err)
	}
}

func Test_CLIProxyAPI_Status_becomes_stopped_when_binary_exists(t *testing.T) {
	// Given
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "plugins")
	manager := extensions.NewCLIProxyAPIManager(extensions.CLIProxyAPIOptions{
		PluginRoot: pluginRoot,
		DataRoot:   filepath.Join(root, "data"),
		LogLines:   20,
	})
	missing := manager.Status(t.Context())
	if missing.Status != extensions.StatusMissing {
		t.Fatalf("expected missing before binary exists, got %q", missing.Status)
	}
	if missing.Installed {
		t.Fatalf("missing binary should report installed=false")
	}
	if err := os.MkdirAll(filepath.Dir(missing.BinaryPath), 0755); err != nil {
		t.Fatalf("create plugin dir: %v", err)
	}
	if err := os.WriteFile(missing.BinaryPath, []byte("fake"), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	// When
	installed := manager.Status(t.Context())

	// Then
	if installed.Status != extensions.StatusStopped {
		t.Fatalf("expected stopped after binary exists, got %q", installed.Status)
	}
	if !installed.Installed {
		t.Fatalf("existing binary should report installed=true")
	}
}

func Test_CLIProxyAPI_Start_generates_config_reaches_running_and_captures_bounded_logs(t *testing.T) {
	// Given
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	binaryPath := buildFakeCLIProxyAPI(t, filepath.Join(root, "plugins"))
	manager := extensions.NewCLIProxyAPIManager(extensions.CLIProxyAPIOptions{
		PluginRoot: filepath.Join(root, "plugins"),
		DataRoot:   dataRoot,
		Port:       freeTestPort(t),
		LogLines:   2,
	})

	// When
	status, err := manager.Start(t.Context())

	// Then
	if err != nil {
		t.Fatalf("start fake CLIProxyAPI: %v", err)
	}
	if status.Status != extensions.StatusRunning || !status.Installed || status.BinaryPath != binaryPath {
		t.Fatalf("expected running installed status, got %+v", status)
	}
	configData, err := os.ReadFile(status.ConfigPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	configText := string(configData)
	for _, want := range configWants(dataRoot, status.Port) {
		if !strings.Contains(configText, want) {
			t.Fatalf("generated config missing %q in:\n%s", want, configText)
		}
	}
	waitForLogLine(t, manager, "fake log line 3")
	logs := manager.Logs()
	if len(logs) != 2 || !strings.Contains(strings.Join(logs, "\n"), "fake log line 3") {
		t.Fatalf("expected bounded logs to retain latest fake output, got %+v", logs)
	}

	// When
	stopped, err := manager.Stop(t.Context())

	// Then
	if err != nil {
		t.Fatalf("stop fake CLIProxyAPI: %v", err)
	}
	if stopped.Status != extensions.StatusStopped {
		t.Fatalf("expected stopped status after stop, got %+v", stopped)
	}
}

func Test_CLIProxyAPI_Status_reports_stopped_after_child_exits(t *testing.T) {
	// Given
	root := t.TempDir()
	buildFakeCLIProxyAPI(t, filepath.Join(root, "plugins"))
	manager := extensions.NewCLIProxyAPIManager(extensions.CLIProxyAPIOptions{
		PluginRoot: filepath.Join(root, "plugins"),
		DataRoot:   filepath.Join(root, "data"),
		Port:       freeTestPort(t),
	})
	if _, err := manager.Start(t.Context()); err != nil {
		t.Fatalf("start fake CLIProxyAPI: %v", err)
	}
	if _, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/exit", manager.Status(t.Context()).Port)); err != nil {
		t.Fatalf("request fake exit: %v", err)
	}

	// When
	status := waitForStatus(t, manager, extensions.StatusStopped)

	// Then
	if status.Status != extensions.StatusStopped {
		t.Fatalf("expected exited child to be stopped, got %+v", status)
	}
}

func Test_ExtensionsAPI_returns_cliproxyapi_missing_status_and_logs(t *testing.T) {
	// Given
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("UUAGENT_HOME", home)
	r := newExtensionsRouter()

	// When
	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/extensions", nil))
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/api/extensions/cliproxyapi", nil))
	logsRec := httptest.NewRecorder()
	r.ServeHTTP(logsRec, httptest.NewRequest(http.MethodGet, "/api/extensions/cliproxyapi/logs", nil))

	// Then
	if listRec.Code != http.StatusOK {
		t.Fatalf("list extensions status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var list struct {
		Extensions []extensions.Status `json:"extensions"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(list.Extensions) != 1 || list.Extensions[0].Status != extensions.StatusMissing {
		t.Fatalf("expected one missing CLIProxyAPI extension, got %+v", list.Extensions)
	}
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), `"status":"missing"`) {
		t.Fatalf("get extension status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	if logsRec.Code != http.StatusOK || !strings.Contains(logsRec.Body.String(), `"lines"`) {
		t.Fatalf("logs status=%d body=%s", logsRec.Code, logsRec.Body.String())
	}
}

func Test_ExtensionsAPI_start_stop_restart_return_clear_missing_status(t *testing.T) {
	// Given
	t.Setenv("UUAGENT_HOME", filepath.Join(t.TempDir(), "home"))
	r := newExtensionsRouter()

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "start", path: "/api/extensions/cliproxyapi/start"},
		{name: "restart", path: "/api/extensions/cliproxyapi/restart"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// When
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tc.path, nil))

			// Then
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("%s status=%d body=%s", tc.name, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"status":"missing"`) || !strings.Contains(rec.Body.String(), "CLIProxyAPI binary missing") {
				t.Fatalf("%s response should include missing status and clear error: %s", tc.name, rec.Body.String())
			}
		})
	}

	// When
	stopRec := httptest.NewRecorder()
	r.ServeHTTP(stopRec, httptest.NewRequest(http.MethodPost, "/api/extensions/cliproxyapi/stop", nil))

	// Then
	if stopRec.Code != http.StatusOK || !strings.Contains(stopRec.Body.String(), `"status":"missing"`) {
		t.Fatalf("stop missing extension status=%d body=%s", stopRec.Code, stopRec.Body.String())
	}
}

func newExtensionsRouter() http.Handler {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	server.RegisterRoutes(r.Group("/api"), agent.New(config.Default()))
	return r
}
