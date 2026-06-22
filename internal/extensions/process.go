package extensions

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"time"
)

func (m *CLIProxyAPIManager) captureLogs(prefix string, src io.Reader) {
	scanner := bufio.NewScanner(src)
	for scanner.Scan() {
		m.logs.Append(prefix + ": " + scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		m.logs.Append(prefix + " log read error: " + err.Error())
	}
}

func (m *CLIProxyAPIManager) waitProcess(cmd *exec.Cmd) {
	err := cmd.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd == cmd {
		m.cmd = nil
		close(m.done)
		m.done = nil
	}
	if err != nil {
		m.lastError = fmt.Sprintf("process exited: %v", err)
		m.logs.Append(m.lastError)
	}
}

func (m *CLIProxyAPIManager) waitHealthy(ctx context.Context) error {
	for i := 0; i < 100; i++ {
		if err := m.Health(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return context.DeadlineExceeded
}

func stopProcess(ctx context.Context, cmd *exec.Cmd, done <-chan struct{}) error {
	if done == nil {
		return cmd.Process.Kill()
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return cmd.Process.Kill()
	}
	select {
	case <-ctx.Done():
		return cmd.Process.Kill()
	case <-time.After(2 * time.Second):
		return cmd.Process.Kill()
	case <-done:
		return nil
	}
}

func choosePort(preferred int) (int, error) {
	if preferred == 0 {
		preferred = 8317
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferred))
	if err == nil {
		return preferred, listener.Close()
	}
	listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("listen on fallback port: %w", err)
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address %q", listener.Addr().String())
	}
	return addr.Port, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
