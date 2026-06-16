package main

import (
	"context"
	"flag"
	"fmt"
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

func main() {
	setupOnly := flag.Bool("setup", false, "initialize ~/.uuagent and exit")
	projectPath := flag.String("project", "", "workspace path whose .uuagent/project.yaml should be loaded")
	port := flag.Int("port", 0, "override HTTP port")
	noBrowser := flag.Bool("no-browser", false, "do not automatically open browser")
	flag.Parse()

	// 1. 加载配置。顺序：defaults < ~/.uuagent/config.yaml < ./config.yaml < <project>/.uuagent/project.yaml < UUAGENT_CONFIG。
	root, err := config.EnsureUserLayout()
	if err != nil {
		fmt.Printf("Setup failed: %v\n", err)
		os.Exit(1)
	}
	if *setupOnly {
		fmt.Printf("UUAgent config initialized at: %s\n", root)
		fmt.Printf("Config file: %s\n", config.UserConfigPath())
		return
	}
	cfg, configSources, err := config.LoadAuto(config.CandidatePaths(*projectPath)...)
	if err != nil {
		fmt.Printf("Config invalid (%v), using defaults.\n", err)
		cfg = config.Default()
	}
	if *port > 0 {
		cfg.Port = *port
	}
	if len(configSources) == 0 {
		fmt.Println("Config not found, using defaults. Runtime overrides can be supplied via env.")
	} else {
		fmt.Printf("Loaded config: %s\n", strings.Join(configSources, ", "))
	}
	if strings.TrimSpace(*projectPath) != "" {
		fmt.Printf("Workspace: %s\n", *projectPath)
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

	// 前端静态文件。开发/测试时直接服务仓库 web/dist；发布打包时可在构建脚本中确保先运行 npm run build。
	r.StaticFS("/ui", http.Dir("web/dist"))
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/ui/")
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	url := fmt.Sprintf("http://localhost%s", addr)

	// 4. 自动打开浏览器
	if !*noBrowser {
		go openBrowser(url)
	}

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
	for len-lenString(s) > 0 {
		s += " "
	}
	return s
}

func lenString(s string) int { return len(s) }

// Keep context for future use
var _ = context.Background
