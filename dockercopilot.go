package main

import (
	"embed"
	"flag"
	"fmt"
	"go/types"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/l429609201/dockerCopilot/internal/config"
	"github.com/l429609201/dockerCopilot/internal/handler"
	"github.com/l429609201/dockerCopilot/internal/module/bot"
	"github.com/l429609201/dockerCopilot/internal/module/scheduler"
	"github.com/l429609201/dockerCopilot/internal/svc"
	"github.com/l429609201/dockerCopilot/internal/utiles"
	"github.com/robfig/cron/v3"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
	"github.com/zeromicro/x/errors"
	xhttp "github.com/zeromicro/x/http"
)

//go:embed front/*
var embeddedFront embed.FS

//go:embed swagger-ui/*
var embeddedSwagger embed.FS

var configFile = flag.String("f", "etc/dockerCopilot.yaml", "the config file")

type UnauthorizedResponse struct {
	Code int                    `json:"code"`
	Msg  string                 `json:"msg"`
	Data map[string]interface{} `json:"data"`
}

// @title        dockerCopilot API
// @version      1.0.0
// @description  Docker 可视化管理工具 API
// @license.name AGPLv3
// @BasePath     /api
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Bearer JWT 令牌（登录后获取）
func main() {
	logDir := "./logs"
	ErrSetupLog := SetupLog(logDir)
	if ErrSetupLog != nil {
		logx.Errorf("failed to setup log: %v", ErrSetupLog)
		os.Exit(1)
	}
	logx.SetLevel(logx.InfoLevel)

	// 自更新辅助容器模式：以新镜像被拉起后，只负责接管主容器的"停旧→建新→启动→删旧"，
	// 完成后进程退出（容器 AutoRemove）。不加载业务配置、不启动 HTTP 服务。
	if utiles.IsHelperMode() {
		utiles.RunHelper()
		return
	}

	flag.Parse()
	var c config.Config
	err := conf.Load(*configFile, &c, conf.UseEnv())
	if err != nil {
		logx.Errorf("无法加载配置文件出错: %v", err)
		logx.Errorf("请确认secretKey设置正确，要求非纯数字且大于八位")
		os.Exit(1)
	}
	server := rest.MustNewServer(c.RestConf, rest.WithCors("*"), rest.WithUnauthorizedCallback(
		func(w http.ResponseWriter, r *http.Request, err error) {
			response := UnauthorizedResponse{
				Code: http.StatusUnauthorized, // 401
				Msg:  "未授权",
				Data: map[string]interface{}{},
			}
			httpx.WriteJson(w, http.StatusUnauthorized, response)
		}))
	defer server.Stop()
	ctx := svc.NewServiceContext(c)

	// 创建 Telegram Bot：既是指令交互入口，也作为定时更新的通知渠道。
	tgBot := bot.New(ctx)
	ctx.Bot = tgBot
	tgBot.Reload() // 按持久化配置决定是否启动
	defer tgBot.Stop()

	// 创建并启动定时更新调度器，注入 Bot 作为通知渠道；配置变更时由 handler 触发重载。
	sched := scheduler.New(ctx, tgBot)
	ctx.Scheduler = sched
	sched.Start()
	defer sched.Stop()

	// Ensure data directory and config exist (Auto-init)
	dataDir := "/data/config/image"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logx.Errorf("Failed to create data directory: %v", err)
	}
	// 持久化图标目录：favicon 自动抓取下载与手动上传统一存放
	if err := os.MkdirAll("/data/images", 0755); err != nil {
		logx.Errorf("Failed to create images directory: %v", err)
	}

	imageLogosPath := "/data/config/imageLogos.js"
	if _, err := os.Stat(imageLogosPath); os.IsNotExist(err) {
		defaultConfig := []byte(`// 自定义镜像logo配置
export const customImageLogos = {
};
`)
		if err := os.WriteFile(imageLogosPath, defaultConfig, 0644); err != nil {
			logx.Errorf("Failed to create default imageLogos.js: %v", err)
		}
	}

	// 首轮镜像更新检查放到后台执行：镜像列表获取和加速器域名探测都可能较慢，
	// 绝不能阻塞 HTTP 服务启动，否则容器控制等页面在启动阶段会长时间无响应。
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logx.Errorf("首轮镜像更新检查发生 panic 已恢复: %v", r)
			}
		}()
		// 聚合所有已启用主机的镜像（按ID去重），使远程主机容器也能正确显示"可更新"
		list, err := utiles.GetAllImagesList(ctx)
		if err != nil {
			// 获取失败仅记录日志，不清空已有结果、不退出进程
			logx.Errorf("首轮获取镜像列表出错: %v", err)
			return
		}
		ctx.HubImageInfo.CheckUpdate(list)
	}()
	corndanmu := cron.New(cron.WithParser(cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
	)))
	_, err = corndanmu.AddFunc("30 * * * *", func() {
		// 定时检查同样做 panic 兜底，单次失败不影响后续调度和主服务
		defer func() {
			if r := recover(); r != nil {
				logx.Errorf("定时镜像更新检查发生 panic 已恢复: %v", r)
			}
		}()
		// 聚合所有已启用主机的镜像（按ID去重），使远程主机容器也能正确显示"可更新"
		list, err := utiles.GetAllImagesList(ctx)
		if err != nil {
			logx.Errorf("定时获取镜像列表出错: %v", err)
			return
		}
		ctx.HubImageInfo.CheckUpdate(list)
	})
	if err != nil {
		logx.Errorf("添加定时任务出错: %v", err)
		panic(err)
	}
	corndanmu.Start()
	defer corndanmu.Stop()
	httpx.SetErrorHandler(func(err error) (int, any) {
		switch e := err.(type) {
		case *errors.CodeMsg:
			return http.StatusOK, xhttp.BaseResponse[types.Nil]{
				Code: e.Code,
				Msg:  e.Msg,
			}
		default:
			return http.StatusOK, xhttp.BaseResponse[types.Nil]{
				Code: 50000,
				Msg:  err.Error(),
			}
		}
	})
	handler.RegisterHandlers(server, ctx)
	RegisterHandlers(server)
	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	logx.Info("程序版本" + config.Version)
	server.Start()
}
func RegisterHandlers(engine *rest.Server) {
	frontFS, err := fs.Sub(embeddedFront, "front")
	if err != nil {
		log.Fatal(err)
	}

	frontFileServer := http.StripPrefix("/manager", http.FileServer(http.FS(frontFS)))

	assetsHandler := http.FileServer(http.FS(frontFS))

	// Serve custom icons（历史目录，保持兼容）
	iconFileServer := http.StripPrefix("/src/config/image/", http.FileServer(http.Dir("/data/config/image")))
	// 持久化图标目录 /data/images（favicon 自动抓取下载 + 手动上传统一存放）
	imagesFileServer := http.StripPrefix("/images/", http.FileServer(http.Dir("/data/images")))
	engine.AddRoutes(
		[]rest.Route{
			{
				Method: http.MethodGet,
				Path:   "/src/config/image/:file",
				Handler: func(w http.ResponseWriter, r *http.Request) {
					iconFileServer.ServeHTTP(w, r)
				},
			},
			{
				// 持久化图标静态访问：/images/xxx.png -> /data/images/xxx.png
				Method: http.MethodGet,
				Path:   "/images/:file",
				Handler: func(w http.ResponseWriter, r *http.Request) {
					imagesFileServer.ServeHTTP(w, r)
				},
			},
			{
				// DockerCopilot 自身的 favicon（用于浏览器标签页图标）
				Method: http.MethodGet,
				Path:   "/favicon.png",
				Handler: func(w http.ResponseWriter, r *http.Request) {
					// 从嵌入的前端资源返回
					data, err := embeddedDist.ReadFile("dist/favicon.png")
					if err != nil {
						logx.Errorf("无法读取嵌入的 favicon: %v", err)
						http.Error(w, "Icon not found", http.StatusNotFound)
						return
					}
					w.Header().Set("Content-Type", "image/png")
					w.Header().Set("Cache-Control", "public, max-age=86400")
					w.Write(data)
				},
			},
			{
				// 兼容 /favicon.ico 请求
				Method: http.MethodGet,
				Path:   "/favicon.ico",
				Handler: func(w http.ResponseWriter, r *http.Request) {
					// 从嵌入的前端资源返回
					data, err := embeddedDist.ReadFile("dist/favicon.png")
					if err != nil {
						logx.Errorf("无法读取嵌入的 favicon: %v", err)
						http.Error(w, "Icon not found", http.StatusNotFound)
						return
					}
					w.Header().Set("Content-Type", "image/png")
					w.Header().Set("Cache-Control", "public, max-age=86400")
					w.Write(data)
				},
			},
		},
	)

	// Swagger-UI 静态文件服务：嵌入 swagger-ui/ 目录，通过 /api/docs/ 访问（静态资源无需 JWT）。
	// swagger-ui/swagger-initializer.js 指向 ./swagger.json（由 swag init 在镜像构建阶段生成到 swagger-ui/ 目录）。
	swaggerFS, swaggerErr := fs.Sub(embeddedSwagger, "swagger-ui")
	if swaggerErr != nil {
		logx.Errorf("嵌入 swagger-ui 失败: %v", swaggerErr)
	} else {
		swaggerHandler := http.StripPrefix("/api/docs/", http.FileServer(http.FS(swaggerFS)))
		engine.AddRoutes(
			[]rest.Route{
				{
					Method: http.MethodGet,
					Path:   "/docs/",
					Handler: func(w http.ResponseWriter, r *http.Request) {
						swaggerHandler.ServeHTTP(w, r)
					},
				},
				{
					Method: http.MethodGet,
					Path:   "/docs/:file",
					Handler: func(w http.ResponseWriter, r *http.Request) {
						swaggerHandler.ServeHTTP(w, r)
					},
				},
			},
			rest.WithPrefix("/api"),
		)
	}

	engine.AddRoutes(
		[]rest.Route{
			{
				Method: http.MethodGet,
				Path:   "/manager",
				Handler: func(w http.ResponseWriter, r *http.Request) {
					frontFileServer.ServeHTTP(w, r)
				},
			},
			{
				Method: http.MethodGet,
				Path:   "/manager/:path",
				Handler: func(w http.ResponseWriter, r *http.Request) {
					frontFileServer.ServeHTTP(w, r)
				},
			},
			{
				Method: http.MethodGet,
				Path:   "/manager/assets/:path",
				Handler: func(w http.ResponseWriter, r *http.Request) {
					frontFileServer.ServeHTTP(w, r)
				},
			},
			{
				Method: http.MethodGet,
				Path:   "/assets/:path",
				Handler: func(w http.ResponseWriter, r *http.Request) {
					assetsHandler.ServeHTTP(w, r)
				},
			},
		},
	)
}

// 检查并创建日志目录
func ensureLogDirectory(logDir string) error {
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		return os.MkdirAll(logDir, 0755) // 创建目录并设置权限
	}
	return nil
}

// SetupLog 初始化日志设置
func SetupLog(logDir string) error {
	// 检查日志目录是否存在
	if err := ensureLogDirectory(logDir); err != nil {
		return fmt.Errorf("failed to create log directory: %v", err)
	}

	logConf := logx.LogConf{
		Path:     logDir,
		Level:    "info",
		KeepDays: 7,
		Compress: true,
		Mode:     "file",
	}
	logx.MustSetup(logConf)
	logx.AddWriter(logx.NewWriter(os.Stdout))
	return nil
}
