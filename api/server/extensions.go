package server

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/yeyan00/uuagent/internal/extensions"
	"github.com/yeyan00/uuagent/internal/paths"
)

var (
	extensionsMu       sync.Mutex
	cliProxyAPIManager *extensions.CLIProxyAPIManager
)

func extensionManager() *extensions.CLIProxyAPIManager {
	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	if cliProxyAPIManager == nil {
		cliProxyAPIManager = extensions.NewCLIProxyAPIManager(extensions.CLIProxyAPIOptions{PluginRoot: filepath.Join(paths.UserDir(), "plugins"), DataRoot: filepath.Join(paths.UserDir(), "extensions")})
	}
	return cliProxyAPIManager
}

func ShutdownExtensions(ctx context.Context) error {
	extensionsMu.Lock()
	manager := cliProxyAPIManager
	extensionsMu.Unlock()
	if manager == nil {
		return nil
	}
	return manager.Close(ctx)
}

func handleListExtensions() gin.HandlerFunc {
	return func(c *gin.Context) {
		manager := extensionManager()
		c.JSON(http.StatusOK, gin.H{"extensions": []extensions.Status{manager.Status(c.Request.Context())}})
	}
}

func handleGetCLIProxyAPIExtension() gin.HandlerFunc {
	return func(c *gin.Context) {
		manager := extensionManager()
		c.JSON(http.StatusOK, manager.Status(c.Request.Context()))
	}
}

func handleStartCLIProxyAPIExtension() gin.HandlerFunc {
	return func(c *gin.Context) {
		status, err := extensionManager().Start(c.Request.Context())
		writeExtensionLifecycleResponse(c, status, err)
	}
}

func handleStopCLIProxyAPIExtension() gin.HandlerFunc {
	return func(c *gin.Context) {
		status, err := extensionManager().Stop(c.Request.Context())
		writeExtensionLifecycleResponse(c, status, err)
	}
}

func handleRestartCLIProxyAPIExtension() gin.HandlerFunc {
	return func(c *gin.Context) {
		status, err := extensionManager().Restart(c.Request.Context())
		writeExtensionLifecycleResponse(c, status, err)
	}
}

func handleCLIProxyAPILogs() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"lines": extensionManager().Logs()})
	}
}

func writeExtensionLifecycleResponse(c *gin.Context, status extensions.Status, err error) {
	if err == nil {
		c.JSON(http.StatusOK, status)
		return
	}
	code := http.StatusInternalServerError
	if errors.Is(err, extensions.ErrBinaryMissing) {
		code = http.StatusServiceUnavailable
	}
	c.JSON(code, gin.H{"error": err.Error(), "extension": status, "status": status.Status})
}
