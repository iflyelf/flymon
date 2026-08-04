package gateway

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
)

// ============================================================================
// Gateway 初始化：对齐 center.Initialize / pushgw.Initialize 签名
// ============================================================================

// Initialize 初始化 gateway 服务，返回 cleanup 函数。
//
// 参数：
//   - configDir: 配置目录（gateway 从环境变量读取配置，此参数保留用于统一签名）
//   - cryptoKey: 加密密钥（gateway 无需加密，此参数保留用于统一签名）
//
// 内部流程：
//   1. LoadConfig() 从环境变量读取配置
//   2. 初始化 TokenStore（file / redis）
//   3. 创建 N9EClient, FeishuClient, AIEngine
//   4. 创建 gin router
//   5. 注册路由并启动 HTTP Server 监听
//   6. 返回 cleanup 函数（优雅关闭 HTTP Server）
func Initialize(configDir string, cryptoKey string) (func(), error) {
	// 1. 加载配置
	cfg := LoadConfig()
	logger.Infof("gateway 配置加载完成: listen=%s:%d, n9e_url=%s, ai_strategy=%s, data_dir=%s",
		cfg.ListenHost, cfg.ListenPort, cfg.N9EAPIURL, cfg.AIProviderStrategy, cfg.DataDir)

	// 2. 初始化 token 存储
	if err := InitTokenStores(cfg); err != nil {
		return nil, fmt.Errorf("初始化 token 存储失败: %w", err)
	}
	StartTokenGC()
	logger.Info("token 存储初始化完成，GC 已启动")

	// 3. 初始化客户端
	n9eClient := NewN9EClient(cfg)
	feishuClient := NewFeishuClient(cfg)
	aiEngine := NewAIEngine(cfg)
	logger.Info("N9E、飞书、AI 客户端初始化完成")

	// 4. 创建 Server（持有依赖与去重缓存）
	server := NewServer(cfg, n9eClient, feishuClient, aiEngine)

	// 5. 创建 gin router 并注册路由
	//    handlers 保持标准 net/http 签名，通过 gin.WrapF 适配。
	//    外部反向代理时可自行加 /api/n9e/gateway/ 前缀，此处直接根路径注册。
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/", gin.WrapF(server.handleIndex))
	router.GET("/health", gin.WrapF(server.handleHealth))
	router.POST("/mute/register", gin.WrapF(server.handleMuteRegister))
	router.POST("/ai_analysis/register", gin.WrapF(server.handleAIRegister))
	router.POST("/group_chat/register", gin.WrapF(server.handleGroupChatRegister))
	router.GET("/ai_analysis/trigger", gin.WrapF(server.handleAITrigger))
	router.POST("/feishu_callback", gin.WrapF(server.handleFeishuCallback))

	// 6. 启动 HTTP Server
	addr := fmt.Sprintf("%s:%d", cfg.ListenHost, cfg.ListenPort)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Infof("gateway HTTP 服务监听: %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("gateway HTTP 服务异常: %v", err)
		}
	}()

	// 7. 返回 cleanup 函数
	cleanup := func() {
		logger.Info("gateway 正在关闭...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			logger.Errorf("gateway HTTP 服务关闭失败: %v", err)
		}
		logger.Info("gateway 已关闭")
	}

	return cleanup, nil
}
