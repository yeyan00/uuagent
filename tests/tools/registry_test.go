package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yeyan00/uuagent/internal/tools"
)

func TestToolsRejectPathsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}

	registry := tools.NewRegistry(workspace)
	read, ok := registry.Get("read")
	if !ok {
		t.Fatal("read tool missing")
	}

	out, err := read.Execute(context.Background(), map[string]any{"path": outside})
	if err == nil {
		t.Fatalf("expected outside absolute path to be rejected, got output %q", out)
	}
	if !strings.Contains(err.Error(), "outside workspace") {
		t.Fatalf("expected outside workspace error, got %v", err)
	}

	write, ok := registry.Get("write")
	if !ok {
		t.Fatal("write tool missing")
	}
	out, err = write.Execute(context.Background(), map[string]any{"path": "../escape.txt", "content": "bad"})
	if err == nil {
		t.Fatalf("expected relative traversal to be rejected, got output %q", out)
	}
	if !strings.Contains(err.Error(), "outside workspace") {
		t.Fatalf("expected outside workspace error, got %v", err)
	}
}

func TestShellToolRunsOnCurrentPlatform(t *testing.T) {
	registry := tools.NewRegistry(t.TempDir())
	shell, ok := registry.Get("shell")
	if !ok {
		t.Fatal("shell tool missing")
	}

	out, err := shell.Execute(context.Background(), map[string]any{"command": "echo uuagent-shell-ok"})
	if err != nil {
		t.Fatalf("shell execute: %v", err)
	}
	if !strings.Contains(out, "uuagent-shell-ok") {
		t.Fatalf("expected shell output, got %q on %s", out, runtime.GOOS)
	}
}

func TestShellToolTimeoutReturnsStableMessage(t *testing.T) {
	registry := tools.NewRegistry(t.TempDir())
	shell, ok := registry.Get("shell")
	if !ok {
		t.Fatal("shell tool missing")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	started := time.Now()
	out, err := shell.Execute(ctx, map[string]any{"command": longRunningCommand()})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("shell execute should return timeout text instead of error: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("expected timeout to return quickly, took %s with output %q", elapsed, out)
	}
	if !strings.Contains(strings.ToLower(out), "timeout") && !strings.Contains(strings.ToLower(out), "deadline") && !strings.Contains(strings.ToLower(out), "killed") {
		t.Fatalf("expected timeout-like message, got %q", out)
	}
}

func TestShellToolSupportsSafeCWDAndStructuredResult(t *testing.T) {
	workspace := t.TempDir()
	subdir := filepath.Join(workspace, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry(workspace)
	shell, ok := registry.Get("shell")
	if !ok {
		t.Fatal("shell tool missing")
	}

	out, err := shell.Execute(context.Background(), map[string]any{"command": pwdCommand(), "cwd": "subdir"})
	if err != nil {
		t.Fatalf("shell execute: %v", err)
	}
	var result struct {
		Stdout     string `json:"stdout"`
		Stderr     string `json:"stderr"`
		ExitCode   int    `json:"exit_code"`
		TimedOut   bool   `json:"timed_out"`
		DurationMS int64  `json:"duration_ms"`
		CWD        string `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected structured JSON shell result, got %q: %v", out, err)
	}
	if result.ExitCode != 0 || result.TimedOut || result.DurationMS < 0 {
		t.Fatalf("unexpected result metadata: %+v", result)
	}
	if filepath.Clean(result.CWD) != filepath.Clean(subdir) {
		t.Fatalf("expected cwd %s, got %+v", subdir, result)
	}
}

func TestShellToolRejectsCWDOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	registry := tools.NewRegistry(workspace)
	shell, ok := registry.Get("shell")
	if !ok {
		t.Fatal("shell tool missing")
	}

	out, err := shell.Execute(context.Background(), map[string]any{"command": "echo should-not-run", "cwd": outside})
	if err == nil {
		t.Fatalf("expected outside cwd to be rejected, got output %q", out)
	}
	if !strings.Contains(err.Error(), "outside workspace") {
		t.Fatalf("expected outside workspace error, got %v", err)
	}
}

func TestApprovalModeRequiresApprovalForOutsideReadAndAllowsWhenApproved(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(outside, []byte("external-ok"), 0600); err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistryWithOptions(workspace, tools.Options{PermissionMode: tools.PermissionAsk})
	read, ok := registry.Get("read")
	if !ok {
		t.Fatal("read tool missing")
	}

	out, err := read.Execute(context.Background(), map[string]any{"path": outside})
	if err != nil {
		t.Fatalf("ask mode should return approval payload instead of error: %v", err)
	}
	var approval struct {
		ApprovalRequired bool   `json:"approval_required"`
		Tool             string `json:"tool"`
		Path             string `json:"path"`
		Reason           string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(out), &approval); err != nil {
		t.Fatalf("expected approval JSON, got %q: %v", out, err)
	}
	if !approval.ApprovalRequired || approval.Tool != "read" || filepath.Clean(approval.Path) != filepath.Clean(outside) || approval.Reason == "" {
		t.Fatalf("unexpected approval payload: %+v", approval)
	}

	out, err = read.Execute(context.Background(), map[string]any{"path": outside, "approved": true})
	if err != nil {
		t.Fatalf("approved read should be allowed: %v", err)
	}
	if out != "external-ok" {
		t.Fatalf("unexpected approved read output: %q", out)
	}
}

func TestApprovalModeRequiresApprovalForOutsideShellCWDAndAllowsWhenApproved(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	registry := tools.NewRegistryWithOptions(workspace, tools.Options{PermissionMode: tools.PermissionAsk})
	shell, ok := registry.Get("shell")
	if !ok {
		t.Fatal("shell tool missing")
	}

	out, err := shell.Execute(context.Background(), map[string]any{"command": pwdCommand(), "cwd": outside})
	if err != nil {
		t.Fatalf("ask mode should return approval payload instead of error: %v", err)
	}
	var approval struct {
		ApprovalRequired bool   `json:"approval_required"`
		Tool             string `json:"tool"`
		Path             string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &approval); err != nil {
		t.Fatalf("expected approval JSON, got %q: %v", out, err)
	}
	if !approval.ApprovalRequired || approval.Tool != "shell" || filepath.Clean(approval.Path) != filepath.Clean(outside) {
		t.Fatalf("unexpected shell approval payload: %+v", approval)
	}

	out, err = shell.Execute(context.Background(), map[string]any{"command": pwdCommand(), "cwd": outside, "approved": true})
	if err != nil {
		t.Fatalf("approved shell cwd should be allowed: %v", err)
	}
	var result struct {
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exit_code"`
		CWD      string `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected shell result JSON, got %q: %v", out, err)
	}
	if result.ExitCode != 0 || filepath.Clean(result.CWD) != filepath.Clean(outside) {
		t.Fatalf("unexpected approved shell result: %+v", result)
	}
}

func longRunningCommand() string {
	if runtime.GOOS == "windows" {
		return "ping -n 6 127.0.0.1 > $null"
	}
	return "sleep 2"
}

func pwdCommand() string {
	if runtime.GOOS == "windows" {
		return "(Get-Location).Path"
	}
	return "pwd"
}
