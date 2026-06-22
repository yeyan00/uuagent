package extensions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const cliProxyAPIID = "cliproxyapi"

var ErrBinaryMissing = errors.New("CLIProxyAPI binary missing")

type CLIProxyAPIManager struct {
	mu        sync.Mutex
	opts      CLIProxyAPIOptions
	logs      *LogBuffer
	cmd       *exec.Cmd
	done      chan struct{}
	port      int
	lastError string
}

func NewCLIProxyAPIManager(opts CLIProxyAPIOptions) *CLIProxyAPIManager {
	port := opts.Port
	if port == 0 {
		port = 8317
	}
	return &CLIProxyAPIManager{opts: opts, logs: NewLogBuffer(opts.LogLines), port: port}
}

func (m *CLIProxyAPIManager) Status(context.Context) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusLocked(m.stateLocked())
}

func (m *CLIProxyAPIManager) Start(ctx context.Context) (Status, error) {
	m.mu.Lock()
	missing := !fileExists(m.binaryPathLocked())
	if missing {
		status := m.setErrorLocked(StatusMissing, fmt.Errorf("%w: %s", ErrBinaryMissing, m.binaryPathLocked()))
		m.mu.Unlock()
		return status, fmt.Errorf("%w: %s", ErrBinaryMissing, status.BinaryPath)
	}
	if m.runningLocked() {
		status := m.statusLocked(StatusRunning)
		m.mu.Unlock()
		return status, nil
	}
	status, err := m.startLocked(ctx)
	m.mu.Unlock()
	if err != nil {
		return status, err
	}
	if err := m.waitHealthy(ctx); err != nil {
		m.mu.Lock()
		status = m.setErrorLocked(StatusError, fmt.Errorf("health check: %w", err))
		m.mu.Unlock()
		_, _ = m.Stop(ctx)
		return status, fmt.Errorf("wait for CLIProxyAPI health: %w", err)
	}
	return m.Status(ctx), nil
}

func (m *CLIProxyAPIManager) Stop(ctx context.Context) (Status, error) {
	m.mu.Lock()
	cmd := m.cmd
	if cmd == nil || cmd.Process == nil || cmd.ProcessState != nil {
		status := m.statusLocked(m.stateLocked())
		m.mu.Unlock()
		return status, nil
	}
	done := m.done
	m.mu.Unlock()
	if err := stopProcess(ctx, cmd, done); err != nil {
		m.mu.Lock()
		status := m.setErrorLocked(StatusError, fmt.Errorf("stop: %w", err))
		m.mu.Unlock()
		return status, fmt.Errorf("stop CLIProxyAPI: %w", err)
	}
	m.mu.Lock()
	if m.cmd == cmd {
		m.cmd = nil
		m.done = nil
	}
	status := m.statusLocked(m.stateLocked())
	m.mu.Unlock()
	return status, nil
}

func (m *CLIProxyAPIManager) Restart(ctx context.Context) (Status, error) {
	if _, err := m.Stop(ctx); err != nil {
		return m.Status(ctx), err
	}
	return m.Start(ctx)
}

func (m *CLIProxyAPIManager) Health(ctx context.Context) error {
	port := m.currentPort()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/healthz", port), nil)
	if err != nil {
		return fmt.Errorf("build health request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("call health endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("health endpoint status=%d", resp.StatusCode)
	}
	return nil
}

func (m *CLIProxyAPIManager) Logs() []string { return m.logs.Lines() }

func (m *CLIProxyAPIManager) startLocked(ctx context.Context) (Status, error) {
	if m.cmd != nil && m.cmd.ProcessState != nil {
		m.cmd = nil
	}
	port, err := choosePort(m.port)
	if err != nil {
		return m.setErrorLocked(StatusError, fmt.Errorf("choose port: %w", err)), fmt.Errorf("choose CLIProxyAPI port: %w", err)
	}
	m.port = port
	if err := m.writeConfigLocked(); err != nil {
		return m.setErrorLocked(StatusError, err), fmt.Errorf("write CLIProxyAPI config: %w", err)
	}
	cmd := exec.CommandContext(ctx, m.binaryPathLocked(), "--config", m.configPathLocked())
	cmd.Dir = filepath.Dir(m.binaryPathLocked())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return m.setErrorLocked(StatusError, err), fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return m.setErrorLocked(StatusError, err), fmt.Errorf("create stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return m.setErrorLocked(StatusError, err), fmt.Errorf("start CLIProxyAPI: %w", err)
	}
	m.cmd = cmd
	m.done = make(chan struct{})
	m.lastError = ""
	go m.captureLogs("stdout", stdout)
	go m.captureLogs("stderr", stderr)
	go m.waitProcess(cmd)
	return m.statusLocked(StatusStarting), nil
}

func (m *CLIProxyAPIManager) stateLocked() string {
	if !fileExists(m.binaryPathLocked()) {
		return StatusMissing
	}
	if m.runningLocked() {
		return StatusRunning
	}
	if m.cmd != nil && m.cmd.ProcessState != nil {
		m.cmd = nil
	}
	return StatusStopped
}

func (m *CLIProxyAPIManager) statusLocked(state string) Status {
	managementURL := ""
	if state == StatusRunning && m.managementPanelAvailableLocked() {
		managementURL = fmt.Sprintf("http://127.0.0.1:%d/management.html", m.port)
	}
	return Status{ID: cliProxyAPIID, Name: "CLIProxyAPI", Description: "OpenAI-compatible model proxy and management panel", BuiltIn: true, Installed: fileExists(m.binaryPathLocked()), Enabled: true, Status: state, BinaryPath: m.binaryPathLocked(), ConfigPath: m.configPathLocked(), Port: m.port, ProxyURL: fmt.Sprintf("http://127.0.0.1:%d/v1", m.port), ManagementURL: managementURL, LastError: m.lastError}
}

func (m *CLIProxyAPIManager) setErrorLocked(state string, err error) Status {
	m.lastError = err.Error()
	m.logs.Append(m.lastError)
	return m.statusLocked(state)
}

func (m *CLIProxyAPIManager) managementPanelAvailableLocked() bool {
	client := http.Client{Timeout: 500 * time.Millisecond}
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d/management.html", m.port), nil)
		if err != nil {
			return false
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return true
		}
	}
	return false
}

func (m *CLIProxyAPIManager) binaryPathLocked() string {
	name := "cli-proxy-api"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(m.opts.PluginRoot, cliProxyAPIID, name)
}

func (m *CLIProxyAPIManager) configPathLocked() string {
	return filepath.Join(m.opts.DataRoot, cliProxyAPIID, "config.yaml")
}

func (m *CLIProxyAPIManager) writeConfigLocked() error {
	dataDir := filepath.Join(m.opts.DataRoot, cliProxyAPIID)
	if err := os.MkdirAll(filepath.Join(dataDir, "logs"), 0755); err != nil {
		return fmt.Errorf("create extension log dir: %w", err)
	}
	authDir := filepath.Join(dataDir, "auth")
	if err := os.MkdirAll(authDir, 0755); err != nil {
		return fmt.Errorf("create extension auth dir: %w", err)
	}
	secretPath := filepath.Join(authDir, "management.secret")
	if _, err := os.Stat(secretPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(secretPath, []byte(fmt.Sprintf("local-%d", time.Now().UnixNano())), 0600); err != nil {
			return fmt.Errorf("write management secret: %w", err)
		}
	}
	secretData, err := os.ReadFile(secretPath)
	if err != nil {
		return fmt.Errorf("read management secret: %w", err)
	}
	secret := strings.TrimSpace(string(secretData))
	body := strings.Join([]string{"host: 127.0.0.1", fmt.Sprintf("port: %d", m.port), "data_dir: " + dataDir, "auth_dir: " + authDir, "log_dir: " + filepath.Join(dataDir, "logs"), "remote-management:", "  secret-key: " + secret, ""}, "\n")
	if err := os.WriteFile(m.configPathLocked(), []byte(body), 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

func (m *CLIProxyAPIManager) runningLocked() bool {
	return m.cmd != nil && m.cmd.Process != nil && m.cmd.ProcessState == nil
}

func (m *CLIProxyAPIManager) currentPort() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.port
}
