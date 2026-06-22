package extensions_test

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yeyan00/uuagent/internal/extensions"
)

func buildFakeCLIProxyAPI(t *testing.T, pluginRoot string) string {
	t.Helper()
	binaryDir := filepath.Join(pluginRoot, "cliproxyapi")
	if err := os.MkdirAll(binaryDir, 0755); err != nil {
		t.Fatalf("create fake binary dir: %v", err)
	}
	sourcePath := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(sourcePath, []byte(fakeCLIProxyAPISource), 0600); err != nil {
		t.Fatalf("write fake source: %v", err)
	}
	binaryName := "cli-proxy-api"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(binaryDir, binaryName)
	cmd := exec.Command("go", "build", "-o", binaryPath, sourcePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake CLIProxyAPI: %v\n%s", err, string(output))
	}
	return binaryPath
}

func freeTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address %q", listener.Addr().String())
	}
	return addr.Port
}

func waitForLogLine(t *testing.T, manager *extensions.CLIProxyAPIManager, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(strings.Join(manager.Logs(), "\n"), want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for log line %q; logs=%+v", want, manager.Logs())
}

func waitForStatus(t *testing.T, manager *extensions.CLIProxyAPIManager, want string) extensions.Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var status extensions.Status
	for time.Now().Before(deadline) {
		status = manager.Status(t.Context())
		if status.Status == want {
			return status
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for status %q; last=%+v", want, status)
	return status
}

const fakeCLIProxyAPISource = `package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

func main() {
	configPath := ""
	for i, arg := range os.Args {
		if arg == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
		}
	}
	port := readPort(configPath)
	fmt.Println("fake log line 1")
	fmt.Println("fake log line 2")
	fmt.Println("fake log line 3")
	fmt.Println("MANAGEMENT_STATIC_PATH=" + os.Getenv("MANAGEMENT_STATIC_PATH"))
	done := make(chan struct{})
	server := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", port)}
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	http.HandleFunc("/exit", func(w http.ResponseWriter, _ *http.Request) { close(done); _, _ = w.Write([]byte("bye")) })
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() { <-signals; close(done) }()
	go func() { _ = server.ListenAndServe() }()
	<-done
}

func readPort(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 8317
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "port:") {
			port, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "port:")))
			if err == nil {
				return port
			}
		}
	}
	return 8317
}
`

func configWants(dataRoot string, port int) []string {
	return []string{"host: 127.0.0.1", fmt.Sprintf("port: %d", port), filepath.Join(dataRoot, "cliproxyapi"), filepath.Join(dataRoot, "cliproxyapi", "auth"), filepath.Join(dataRoot, "cliproxyapi", "logs")}
}
