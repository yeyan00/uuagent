package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		runHelperProcess()
		return
	}
	os.Exit(m.Run())
}

func TestRunnerAppliesJSONMutationFromCommand(t *testing.T) {
	runner := New(Config{TimeoutMS: 5000, FailPolicy: "fail", Events: map[string][]Command{
		EventToolBefore: {{Command: helperCommand("mutate_path")}},
	}})

	result, err := runner.Run(context.Background(), EventToolBefore, map[string]any{
		"event": EventToolBefore,
		"args":  map[string]any{"path": "README.md"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	args, ok := result.Output["args"].(map[string]any)
	if !ok {
		t.Fatalf("expected args map, got %#v", result.Output["args"])
	}
	if args["path"] != "docs/README.md" {
		t.Fatalf("expected mutated path, got %#v", args["path"])
	}
}

func TestRunnerTreatsEmptyStdoutAsNoMutation(t *testing.T) {
	runner := New(Config{TimeoutMS: 5000, FailPolicy: "fail", Events: map[string][]Command{
		EventToolBefore: {{Command: helperCommand("empty")}},
	}})

	result, err := runner.Run(context.Background(), EventToolBefore, map[string]any{"event": EventToolBefore})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Output) != 0 {
		t.Fatalf("expected no mutation, got %#v", result.Output)
	}
}

func TestRunnerFailsOnInvalidJSONStdout(t *testing.T) {
	runner := New(Config{TimeoutMS: 5000, FailPolicy: "fail", Events: map[string][]Command{
		EventToolBefore: {{Command: helperCommand("invalid_json")}},
	}})

	_, err := runner.Run(context.Background(), EventToolBefore, map[string]any{"event": EventToolBefore})
	if err == nil {
		t.Fatalf("expected invalid json error")
	}
}

func TestRunnerFailsOnTimeout(t *testing.T) {
	runner := New(Config{TimeoutMS: 50, FailPolicy: "fail", Events: map[string][]Command{
		EventToolBefore: {{Command: helperCommand("sleep"), TimeoutMS: 50}},
	}})

	_, err := runner.Run(context.Background(), EventToolBefore, map[string]any{"event": EventToolBefore})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestRunnerRunsHooksSequentially(t *testing.T) {
	runner := New(Config{TimeoutMS: 5000, FailPolicy: "fail", Events: map[string][]Command{
		EventToolBefore: {{Command: helperCommand("append_a")}, {Command: helperCommand("append_b")}},
	}})

	result, err := runner.Run(context.Background(), EventToolBefore, map[string]any{"event": EventToolBefore, "value": ""})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Output["value"] != "ab" {
		t.Fatalf("expected sequential mutation ab, got %#v", result.Output["value"])
	}
}

func TestFailurePolicyWarnReturnsWarningWithoutFailing(t *testing.T) {
	runner := New(Config{TimeoutMS: 5000, FailPolicy: "fail", Events: map[string][]Command{
		EventToolAfter: {{Command: helperCommand("exit_2"), FailPolicy: "warn"}},
	}})

	result, err := runner.Run(context.Background(), EventToolAfter, map[string]any{"event": EventToolAfter})
	if err != nil {
		t.Fatalf("expected warning without error, got %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected one warning, got %#v", result.Warnings)
	}
}

func helperCommand(mode string) string {
	return fmt.Sprintf("%s --hook-helper %s", os.Args[0], mode)
}

func runHelperProcess() {
	if len(os.Args) < 3 || os.Args[1] != "--hook-helper" {
		os.Exit(2)
	}
	mode := os.Args[2]
	var payload map[string]any
	_ = json.NewDecoder(os.Stdin).Decode(&payload)
	switch mode {
	case "mutate_path":
		fmt.Print(`{"args":{"path":"docs/README.md"}}`)
	case "empty":
	case "invalid_json":
		fmt.Print(`{`)
	case "sleep":
		time.Sleep(2 * time.Second)
	case "append_a":
		value, _ := payload["value"].(string)
		fmt.Print(`{"value":"` + value + `a"}`)
	case "append_b":
		value, _ := payload["value"].(string)
		fmt.Print(`{"value":"` + value + `b"}`)
	case "exit_2":
		fmt.Fprint(os.Stderr, "hook failed")
		os.Exit(2)
	default:
		os.Exit(2)
	}
}

func TestHelperCommandIsExecutable(t *testing.T) {
	cmd := exec.Command(os.Args[0], "--hook-helper", "empty")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("helper command: %v", err)
	}
}
