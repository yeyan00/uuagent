package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnsureUserLayoutWritesFullStarterConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("UUAGENT_HOME", filepath.Join(root, "home"))
	if _, err := EnsureUserLayout(); err != nil {
		t.Fatalf("EnsureUserLayout: %v", err)
	}
	data, err := os.ReadFile(UserConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"agents:", "system_prompt:", "skills:", "mcp_servers:", "UUAGENT_API_KEY"} {
		if !strings.Contains(text, want) {
			t.Fatalf("starter config missing %q:\n%s", want, text)
		}
	}
	cfg, err := Load(UserConfigPath())
	if err != nil {
		t.Fatalf("starter config should parse: %v", err)
	}
	if len(cfg.Agents) == 0 || cfg.Agents[0].ID != "default" {
		t.Fatalf("starter config missing default agent: %+v", cfg.Agents)
	}
}

func TestEnsureUserLayoutInstallsPackagedCLIProxyAPIAssets(t *testing.T) {
	root := t.TempDir()
	t.Setenv("UUAGENT_HOME", filepath.Join(root, "home"))
	packaged := filepath.Join(root, "package", "plugins", "cliproxyapi")
	if err := os.MkdirAll(packaged, 0755); err != nil {
		t.Fatal(err)
	}
	binaryName := "cli-proxy-api"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	if err := os.WriteFile(filepath.Join(packaged, binaryName), []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packaged, "management.html"), []byte("<html>panel</html>"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UUAGENT_PACKAGED_PLUGIN_ROOT", filepath.Join(root, "package", "plugins"))

	if _, err := EnsureUserLayout(); err != nil {
		t.Fatalf("EnsureUserLayout: %v", err)
	}

	installed := filepath.Join(UserDir(), "plugins", "cliproxyapi")
	if data, err := os.ReadFile(filepath.Join(installed, binaryName)); err != nil || string(data) != "binary" {
		t.Fatalf("expected packaged binary installed, data=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(installed, "management.html")); err != nil || string(data) != "<html>panel</html>" {
		t.Fatalf("expected packaged panel installed, data=%q err=%v", data, err)
	}
}

func TestEnsureUserLayoutInstallsEmbeddedCLIProxyAPIAssets(t *testing.T) {
	root := t.TempDir()
	t.Setenv("UUAGENT_HOME", filepath.Join(root, "home"))
	t.Setenv("UUAGENT_PACKAGED_PLUGIN_ROOT", filepath.Join(root, "missing-package", "plugins"))
	binaryName := "cli-proxy-api"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	if _, err := embeddedPluginFS.ReadFile("embedded_plugins/cliproxyapi/" + binaryName); err != nil {
		t.Skip("embedded CLIProxyAPI binary is supplied by scripts/package-windows.ps1")
	}
	if _, err := embeddedPluginFS.ReadFile("embedded_plugins/cliproxyapi/management.html"); err != nil {
		t.Skip("embedded CLIProxyAPI panel is supplied by scripts/package-windows.ps1")
	}

	if _, err := EnsureUserLayout(); err != nil {
		t.Fatalf("EnsureUserLayout: %v", err)
	}

	installed := filepath.Join(UserDir(), "plugins", "cliproxyapi")
	if info, err := os.Stat(filepath.Join(installed, binaryName)); err != nil || info.IsDir() || info.Size() == 0 {
		t.Fatalf("expected embedded binary installed, info=%+v err=%v", info, err)
	}
	if info, err := os.Stat(filepath.Join(installed, "management.html")); err != nil || info.IsDir() || info.Size() == 0 {
		t.Fatalf("expected embedded management panel installed, info=%+v err=%v", info, err)
	}
}

func TestLoadAutoLayersUserCwdExplicitAndEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv("UUAGENT_HOME", filepath.Join(root, "home"))
	t.Setenv("UUAGENT_PROXY_URL", "")
	if _, err := EnsureUserLayout(); err != nil {
		t.Fatalf("EnsureUserLayout: %v", err)
	}
	if err := os.WriteFile(UserConfigPath(), []byte("port: 1111\nagent:\n  proxy-url: http://user/v1\nskills:\n  - name: user-skill\n    prompt: user\n    enabled: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	explicit := filepath.Join(root, "explicit.yaml")
	if err := os.WriteFile(explicit, []byte("port: 2222\nagent:\n  proxy-url: http://explicit/v1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UUAGENT_CONFIG", explicit)
	t.Setenv("UUAGENT_MODEL", "env-model")

	cfg, sources, err := LoadAuto(CandidatePaths("")...)
	if err != nil {
		t.Fatalf("LoadAuto: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected user+explicit sources, got %v", sources)
	}
	if cfg.Port != 2222 {
		t.Fatalf("expected explicit port override, got %d", cfg.Port)
	}
	if cfg.Agent.ProxyURL != "http://explicit/v1" {
		t.Fatalf("unexpected proxy url: %s", cfg.Agent.ProxyURL)
	}
	if got := cfg.Agent.Routing.Tiers[cfg.Agent.Routing.Fallback][0]; got != "env-model" {
		t.Fatalf("expected env model override, got %s", got)
	}
}

func TestDefaultConfigIncludesHookAndAutoCompactDefaults(t *testing.T) {
	cfg := Default()
	if cfg.Hooks.TimeoutMS != 5000 {
		t.Fatalf("expected default hook timeout 5000, got %d", cfg.Hooks.TimeoutMS)
	}
	if cfg.Hooks.FailPolicy != "warn" {
		t.Fatalf("expected default hook fail policy warn, got %q", cfg.Hooks.FailPolicy)
	}
	if cfg.Agent.Context.CompactReservedTokens != 10000 {
		t.Fatalf("expected compact reserved tokens 10000, got %d", cfg.Agent.Context.CompactReservedTokens)
	}
	if !cfg.Agent.Context.CompactAutoContinue {
		t.Fatalf("expected compact auto continue enabled")
	}
}

func Test_DefaultConfig_includes_goal_mode_subagent_profiles(t *testing.T) {
	// Given
	cfg := Default()
	wantProfiles := []string{"planner", "explorer", "builder", "tester", "reviewer"}

	// When
	profiles := map[string]SubagentProfile{}
	for _, profile := range cfg.Agent.Subagent.Profiles {
		profiles[profile.ID] = profile
	}

	// Then
	for _, id := range wantProfiles {
		profile, ok := profiles[id]
		if !ok {
			t.Fatalf("default config missing goal mode subagent profile %q; got %+v", id, cfg.Agent.Subagent.Profiles)
		}
		if profile.Name == "" || profile.SystemPrompt == "" {
			t.Fatalf("goal mode profile %q should include name and prompt: %+v", id, profile)
		}
		if profile.MaxTurns != 0 {
			t.Fatalf("goal mode profile %q should inherit subagent max turns with zero override, got %+v", id, profile)
		}
	}
}

func Test_DefaultConfig_uses_hermes_like_turn_budgets(t *testing.T) {
	// Given
	cfg := Default()

	// Then
	if cfg.Agent.MaxTurns != 90 {
		t.Fatalf("agent max_turns should default to 90, got %d", cfg.Agent.MaxTurns)
	}
	if cfg.Agent.Subagent.MaxTurns != 45 {
		t.Fatalf("subagent max_turns should default to 45, got %d", cfg.Agent.Subagent.MaxTurns)
	}
	if cfg.Goal.MaxTurns != 20 {
		t.Fatalf("goal max_turns should default to 20, got %d", cfg.Goal.MaxTurns)
	}
}
