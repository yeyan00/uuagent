package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	EventToolBefore             = "tool.execute.before"
	EventToolAfter              = "tool.execute.after"
	EventChatHeaders            = "chat.headers"
	EventChatParams             = "chat.params"
	EventLLMBefore              = "llm.before"
	EventLLMAfter               = "llm.after"
	EventSessionCompacting      = "experimental.session.compacting"
	EventCompactionAutoContinue = "experimental.compaction.autocontinue"
)

type FailPolicy string

const (
	FailPolicyFail   FailPolicy = "fail"
	FailPolicyWarn   FailPolicy = "warn"
	FailPolicyIgnore FailPolicy = "ignore"
)

type Command struct {
	Command    string
	Cwd        string
	Env        map[string]string
	TimeoutMS  int
	FailPolicy string
}

type Config struct {
	TimeoutMS  int
	FailPolicy string
	Events     map[string][]Command
}

type Runner struct {
	cfg Config
}

type Result struct {
	Output   map[string]any
	Warnings []string
}

func New(cfg Config) *Runner {
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = 5000
	}
	if strings.TrimSpace(cfg.FailPolicy) == "" {
		cfg.FailPolicy = string(FailPolicyWarn)
	}
	if cfg.Events == nil {
		cfg.Events = map[string][]Command{}
	}
	return &Runner{cfg: cfg}
}

func (r *Runner) Run(ctx context.Context, event string, payload map[string]any) (Result, error) {
	if r == nil {
		return Result{Output: map[string]any{}}, nil
	}
	commands := r.cfg.Events[event]
	if len(commands) == 0 {
		return Result{Output: map[string]any{}}, nil
	}
	current := cloneMap(payload)
	mutations := map[string]any{}
	var warnings []string
	for _, command := range commands {
		out, err := r.runCommand(ctx, command, current)
		if err != nil {
			policy := r.policy(command, event)
			switch policy {
			case FailPolicyIgnore:
				continue
			case FailPolicyWarn:
				warnings = append(warnings, err.Error())
				continue
			default:
				return Result{Output: mutations, Warnings: warnings}, err
			}
		}
		for key, value := range out {
			current[key] = value
			mutations[key] = value
		}
	}
	return Result{Output: mutations, Warnings: warnings}, nil
}

func (r *Runner) runCommand(ctx context.Context, command Command, payload map[string]any) (map[string]any, error) {
	if strings.TrimSpace(command.Command) == "" {
		return nil, fmt.Errorf("hook command is empty")
	}
	timeout := time.Duration(r.timeoutMS(command)) * time.Millisecond
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := shellCommand(cmdCtx, command.Command)
	if strings.TrimSpace(command.Cwd) != "" {
		cmd.Dir = command.Cwd
	}
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GO_WANT_HELPER_PROCESS=1")
	for key, value := range command.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	input, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if cmdCtx.Err() != nil {
		return nil, fmt.Errorf("hook command timed out after %dms", r.timeoutMS(command))
	}
	if err != nil {
		return nil, fmt.Errorf("hook command failed: %w stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.Len() == 0 {
		return map[string]any{}, nil
	}
	if stdout.Len() > 1024*1024 {
		return nil, fmt.Errorf("hook stdout exceeded 1048576 bytes")
	}
	limited := io.LimitReader(bytes.NewReader(stdout.Bytes()), 1024*1024)
	var output map[string]any
	if err := json.NewDecoder(limited).Decode(&output); err != nil {
		return nil, fmt.Errorf("hook stdout invalid json: %w", err)
	}
	if output == nil {
		output = map[string]any{}
	}
	return output, nil
}

func (r *Runner) timeoutMS(command Command) int {
	if command.TimeoutMS > 0 {
		return command.TimeoutMS
	}
	if r.cfg.TimeoutMS > 0 {
		return r.cfg.TimeoutMS
	}
	return 5000
}

func (r *Runner) policy(command Command, event string) FailPolicy {
	if command.FailPolicy != "" {
		return FailPolicy(command.FailPolicy)
	}
	if r.cfg.FailPolicy != "" {
		return FailPolicy(r.cfg.FailPolicy)
	}
	switch event {
	case EventToolBefore, EventLLMBefore:
		return FailPolicyFail
	default:
		return FailPolicyWarn
	}
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}

func cloneMap(input map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		out[key] = value
	}
	return out
}
