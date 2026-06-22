package extensions_test

import (
	"os"
	"strings"
	"testing"

	"github.com/yeyan00/uuagent/internal/extensions"
)

func Test_CLIProxyAPI_Start_exposes_management_secret_and_proxy_api_token(t *testing.T) {
	// Given
	root := t.TempDir()
	buildFakeCLIProxyAPI(t, root+"/plugins")
	manager := extensions.NewCLIProxyAPIManager(extensions.CLIProxyAPIOptions{
		PluginRoot: root + "/plugins",
		DataRoot:   root + "/data",
		Port:       freeTestPort(t),
	})

	// When
	status, err := manager.Start(t.Context())

	// Then
	if err != nil {
		t.Fatalf("start fake CLIProxyAPI: %v", err)
	}
	if status.ManagementSecret == "" {
		t.Fatalf("expected management secret in status")
	}
	if status.ProxyAPIToken == "" {
		t.Fatalf("expected proxy API token in status")
	}
	if status.ManagementSecret == status.ProxyAPIToken {
		t.Fatalf("management secret and proxy API token must be distinct")
	}
	configData, err := os.ReadFile(status.ConfigPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	configText := string(configData)
	for _, want := range []string{
		"remote-management:",
		"  secret-key: " + status.ManagementSecret,
		"api-keys:",
		"  - \"" + status.ProxyAPIToken + "\"",
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("generated config missing %q in:\n%s", want, configText)
		}
	}
	if _, err := manager.Stop(t.Context()); err != nil {
		t.Fatalf("stop fake CLIProxyAPI: %v", err)
	}
}
