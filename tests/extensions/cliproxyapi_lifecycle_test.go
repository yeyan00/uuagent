package extensions_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/yeyan00/uuagent/internal/extensions"
)

func Test_CLIProxyAPI_Close_stops_running_sidecar(t *testing.T) {
	// Given
	root := t.TempDir()
	buildFakeCLIProxyAPI(t, root+"/plugins")
	manager := extensions.NewCLIProxyAPIManager(extensions.CLIProxyAPIOptions{
		PluginRoot: root + "/plugins",
		DataRoot:   root + "/data",
		Port:       freeTestPort(t),
	})
	status, err := manager.Start(t.Context())
	if err != nil {
		t.Fatalf("start fake CLIProxyAPI: %v", err)
	}
	if status.Status != extensions.StatusRunning {
		t.Fatalf("expected running before close, got %+v", status)
	}

	// When
	err = manager.Close(t.Context())

	// Then
	if err != nil {
		t.Fatalf("close manager: %v", err)
	}
	waitForStatus(t, manager, extensions.StatusStopped)
	assertPortClosed(t, status.Port)
}

func assertPortClosed(t *testing.T, port int) {
	t.Helper()
	client := http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", port)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			return
		}
		_ = resp.Body.Close()
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected CLIProxyAPI port %d to be closed", port)
}
