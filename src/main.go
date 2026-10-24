package main

import (
	"context"
	"errors"
	"fmt"
	"image-api/app"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("image-api %s\n", version)
		return
	}

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

	pool := app.NewURLPool()
	stats, err := pool.Load(cfg)
	if err != nil {
		log.Fatalf("[启动失败] %v", err)
	}
	if stats.CreatedFiles {
		log.Printf("[URL池] 默认 URL 列表已生成: %s, %s", cfg.URLs.Portrait, cfg.URLs.Landscape)
	}
	if cfg.PrefetchEnabled() {
		log.Printf("[URL池] portrait=%d landscape=%d pending=%d dead=%d", stats.Portrait, stats.Landscape, stats.Pending, stats.Dead)
	} else {
		log.Printf("[URL池] portrait=%d landscape=%d", stats.Portrait, stats.Landscape)
	}
	log.Printf("[图片来源] source=%s", cfg.Source)
	if cfg.UsesLocal() {
		log.Printf("[本地图片] portrait=%d landscape=%d", stats.LocalPortrait, stats.LocalLandscape)
	}
	if stats.Portrait == 0 {
		log.Printf("[警告] portrait 池为空，请检查 %s", poolHint(cfg, app.OrientationPortrait))
	}
	if stats.Landscape == 0 {
		log.Printf("[警告] landscape 池为空，请检查 %s", poolHint(cfg, app.OrientationLandscape))
	}

	limiter := app.NewRateLimiter(cfg.RateLimit)
	handler := app.NewHandler(cfg, pool, limiter)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           app.NewRouter(cfg, handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("[启动] image-api %s | HTTP Server 监听 %s", version, addr)
	log.Printf("[鉴权] enabled=%v", cfg.Auth.Enabled)
	if cfg.Auth.AdminToken != "" {
		log.Printf("[管理端点] /admin/* 使用专用 admin_token 认证")
	} else if !cfg.Auth.Enabled {
		log.Printf("[警告] 未配置 auth.admin_token 且 auth 未启用，/admin/* 将拒绝访问（401）；请设置 auth.admin_token")
	}
	log.Printf("[限流] enabled=%v", cfg.RateLimit.Enabled)
	log.Printf("[防盗链] enabled=%v", cfg.Referer.Enabled)
	if cfg.Server.BasePath != "" {
		log.Printf("[路径前缀] base_path=%s", cfg.Server.BasePath)
	}

	if cfg.PrefetchEnabled() {
		go pool.RetryPending()
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.PrefetchEnabled() {
		go pool.MetadataWorker(ctx.Done())
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[服务异常] %v", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()
	log.Printf("[退出] 正在关闭...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[退出] 关闭出错: %v", err)
	} else {
		log.Printf("[退出] 已关闭")
	}
}

// poolHint 给空池告警指明该检查哪一处配置。
func poolHint(cfg *app.Config, orientation string) string {
	if cfg.UsesLocal() {
		if orientation == app.OrientationPortrait {
			return fmt.Sprintf("local.portrait_dirs %v", cfg.Local.PortraitDirs)
		}
		return fmt.Sprintf("local.landscape_dirs %v", cfg.Local.LandscapeDirs)
	}
	if orientation == app.OrientationPortrait {
		return cfg.URLs.Portrait
	}
	return cfg.URLs.Landscape
}
