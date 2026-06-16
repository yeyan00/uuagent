package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/uuagent/uuagent/api/server"
	"github.com/uuagent/uuagent/internal/agent"
	"github.com/uuagent/uuagent/internal/config"
)

//go:embed web/dist/*
var webFS embed.FS

func main() {
	// 1. 加载配置
	cfg, err := config.Load("config.yaml")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		fmt.Println("Run 'uuagent --setup' to create a config file")
		os.Exit(1)
	}

	// 2. 创建 Agent
	agt := agent.New(cfg)

	// 3. 启动 HTTP Server
	r := gin.Default()

	// UUAgent API
	api := r.Group("/api")
	server.RegisterRoutes(api, agt)

	// CLIProxyAPI 管理面板 (模型管理)
	// TODO: embed CLIProxyAPI SDK, 注册 /v1/* 和 /v0/management/* 路由

	// 前端静态文件
	distFS, _ := fs.Sub(webFS, "web/dist")
	r.StaticFS("/ui", http.FS(distFS))
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/ui/")
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	url := fmt.Sprintf("http://localhost%s", addr)

	// 4. 自动打开浏览器
	go openBrowser(url)

	fmt.Printf("╔══════════════════════════════════════╗\n")
	fmt.Printf("║  UUAgent - 轻量智能 Coding Agent      ║\n")
	fmt.Printf("║  %s              ║\n", padRight(url, 29))
	fmt.Printf("║  管理: %s/management.html   ║\n", padRight(url, 22))
	fmt.Printf("╚══════════════════════════════════════╝\n")

	if err := r.Run(addr); err != nil {
		fmt.Printf("Server error: %v\n", err)
		os.Exit(1)
	}

	// TODO: Wails 桌面模式
	// if cfg.UIMode == "desktop" { ... }
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func padRight(s string, len int) string {
	if len <= 0 {
		return s
	}
	for len-strings.Len(s) > 0 {
		s += " "
	}
	return s
}

// Keep context for future use
var _ = context.Background
