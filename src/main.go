package main

import (
	"fmt"
	"image-api/app"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// version 由 ldflags 在构建时注入
var version = "dev"

func main() {

	// 支持 --version / -v 查看版本号
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("image-api %s\n", version)
		return
	}

	// 读取配置（默认 config.yaml，可通过命令行参数覆盖）
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, createdCfg, err := app.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("[启动失败] %v", err)
	}
	if createdCfg {
		log.Printf("[配置] 默认配置已生成: %s", cfgPath)
	}
	log.Printf("[配置] 加载成功: %s", cfgPath)

	// 加载 URL 池
	pool := app.NewURLPool()
	createdURLFiles, err := pool.Load(cfg.URLs.Portrait, cfg.URLs.Landscape)
	if err != nil {
		log.Fatalf("[启动失败] %v", err)
	}
	if createdURLFiles {
		log.Printf("[URL池] 默认 URL 列表已生成: %s, %s", cfg.URLs.Portrait, cfg.URLs.Landscape)
	}
	portrait, landscape := pool.Count()
	log.Printf("[URL池] portrait=%d landscape=%d", portrait, landscape)

	// 初始化限流器（全局限流，无需清理任务）
	limiter := app.NewRateLimiter(cfg.RateLimit)

	// 初始化 Handler
	handler := app.NewHandler(cfg, pool, limiter)

	// 路由（支持 server.base_path 前缀，默认空=无前缀）
	mux := http.NewServeMux()
	bp := strings.TrimRight(cfg.Server.BasePath, "/")
	if bp != "" {
		sub := http.NewServeMux()
		sub.HandleFunc("/", handler.ServeImage)
		sub.HandleFunc("/health", handler.Health)
		sub.HandleFunc("/admin/reload", handler.Reload)
		mux.Handle(bp+"/", http.StripPrefix(bp, sub))
	} else {
		mux.HandleFunc("/", handler.ServeImage)
		mux.HandleFunc("/health", handler.Health)
		mux.HandleFunc("/admin/reload", handler.Reload)
	}

	// 启动 HTTP Server（显式设置超时，防 Slowloris 慢连接攻击）
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("[启动] image-api %s | HTTP Server 监听 %s", version, addr)
	log.Printf("[鉴权] enabled=%v", cfg.Auth.Enabled)
	log.Printf("[限流] enabled=%v", cfg.RateLimit.Enabled)
	log.Printf("[防盗链] enabled=%v", cfg.Referer.Enabled)
	if cfg.Server.BasePath != "" {
		log.Printf("[路径前缀] base_path=%s", cfg.Server.BasePath)
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("[服务异常] %v", err)
	}
}
