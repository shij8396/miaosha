package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/config"
	"github.com/miaosha/controller"
	"github.com/miaosha/cron"
	"github.com/miaosha/dao"
	"github.com/miaosha/detector"
	"github.com/miaosha/etcd"
	"github.com/miaosha/kafka"
	"github.com/miaosha/log"
	"github.com/miaosha/middleware"
	"github.com/miaosha/monitor"
	"github.com/miaosha/mq"
	redisClient "github.com/miaosha/redis"
	"github.com/miaosha/sentinel"
	"github.com/miaosha/service"
	"github.com/miaosha/singleflight"
	"github.com/miaosha/utils"
	ws "github.com/miaosha/websocket"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/miaosha/docs" // Swagger 文档
)

// @title           企业级分布式秒杀系统 API
// @version         1.0
// @description     基于 Go + Gin 的企业级高并发秒杀系统，支持 Redis Cluster 分布式锁、RabbitMQ 延迟队列、Kafka 行为追踪、Sentinel 流量防护、Jaeger 全链路追踪
// @termsOfService  http://swagger.io/terms/

// @contact.name   秒杀系统团队
// @contact.url    http://www.miaosha.com/support
// @contact.email  support@miaosha.com

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:8080
// @BasePath  /api/v1

// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 JWT Token，格式: Bearer {token}

func main() {
	cfg, err := config.LoadConfig("")
	if err != nil {
		panic(fmt.Sprintf("配置加载失败: %v", err))
	}

	if err := log.Init(log.Config{
		Level: cfg.Log.Level, Format: cfg.Log.Format, Output: cfg.Log.Output,
		FilePath: cfg.Log.FilePath, MaxSize: cfg.Log.MaxSize, MaxBackups: cfg.Log.MaxBackups,
		MaxAge: cfg.Log.MaxAge, Compress: cfg.Log.Compress,
	}); err != nil {
		panic(fmt.Sprintf("日志初始化失败: %v", err))
	}
	defer log.Sync()
	log.L().Info("====== 企业级秒杀系统启动中 ======")

	if err := dao.InitDB(&cfg.MySQL); err != nil {
		log.L().Fatalw("MySQL初始化失败", "error", err)
	}
	log.L().Info("MySQL主从+读写分离+分表初始化完成")

	if err := redisClient.InitCluster(&cfg.Redis); err != nil {
		log.L().Fatalw("Redis Cluster初始化失败", "error", err)
	}
	defer redisClient.Close()
	log.L().Info("Redis Cluster集群初始化完成")

	if err := mq.Init(&cfg.RabbitMQ); err != nil {
		log.L().Warnw("RabbitMQ初始化失败，MQ功能不可用", "error", err)
	} else {
		defer mq.Close()
		log.L().Info("RabbitMQ镜像队列+延迟队列+死信队列初始化完成")
	}

	if err := kafka.InitProducer(&cfg.Kafka, log.L()); err != nil {
		log.L().Warnw("Kafka生产者初始化失败", "error", err)
	} else {
		defer kafka.Close()
		log.L().Info("Kafka生产者初始化完成")
	}

	if err := etcd.Init(&cfg.Etcd); err != nil {
		log.L().Warnw("Etcd初始化失败", "error", err)
	} else {
		defer etcd.Close()
		etcd.RegisterService(&cfg.Server, &cfg.Etcd)
		log.L().Info("Etcd服务注册发现初始化完成")
	}

	if err := sentinel.Init(&cfg.Sentinel); err != nil {
		log.L().Warnw("Sentinel初始化失败", "error", err)
	}
	log.L().Info("Sentinel-Go流量防护初始化完成")

	monitor.Init()
	monitor.StartMetricsServer(&cfg.Prometheus)
	log.L().Info("Prometheus监控初始化完成")

	jwtManager := utils.NewJWTManager(cfg.JWT.Secret, cfg.JWT.ExpireHours, cfg.JWT.Issuer)
	sf, err := utils.NewSnowflake(int64(cfg.Server.Port) % 1024)
	if err != nil {
		log.L().Fatalw("雪花算法初始化失败", "error", err)
	}
	// [速度优化] 缓冲通道预生成 ID，高并发下 NextID() 无锁竞争
	idGen := utils.NewBufferedSnowflake(sf, 1024)
	log.L().Info("Snowflake ID预生成器初始化完成（缓冲区1024）")

	// [创新] AI 实时异常检测引擎 — 纯 Go 滑动窗口 + Z-Score 统计模型
	anomalyDetector := detector.NewAnomalyDetector(detector.DefaultConfig)
	log.L().Info("AI实时异常检测引擎初始化完成")

	// [创新] 请求合并引擎 — 高并发场景下合并相同商品请求，大幅降低 Redis 压力
	mergeGroup := singleflight.NewShardedGroup(256) // [速度优化] 256 分片消除锁竞争
	log.L().Info("请求合并引擎初始化完成")

	// [创新] WebSocket 实时推送中心 — 秒杀结果/订单状态/系统告警实时推送
	wsHub := ws.NewHub()
	go wsHub.Run()
	log.L().Info("WebSocket实时推送中心初始化完成")

	userService := service.NewUserService(jwtManager)
	productService := service.NewProductService()
	seckillService := service.NewSeckillService(idGen)
	orderService := service.NewOrderService()
	blacklistService := service.NewBlacklistService()
	sentinelService := service.GetSentinelService()
	monitorService := service.NewMonitorService()
	activityService := service.NewActivityService() // [修复] 秒杀活动配置服务
	auditService := service.NewAuditService()       // [修复] 审计日志服务

	userController := controller.NewUserController(userService)
	productController := controller.NewProductController(productService)
	seckillController := controller.NewSeckillController(seckillService, anomalyDetector, mergeGroup, wsHub)
	orderController := controller.NewOrderController(orderService)
	blacklistController := controller.NewBlacklistController(blacklistService)
	sentinelController := controller.NewSentinelController(sentinelService)
	monitorController := controller.NewMonitorController(monitorService, anomalyDetector, wsHub)
	activityController := controller.NewActivityController(activityService) // [修复] 秒杀活动配置控制器
	auditController := controller.NewAuditController(auditService)          // [修复] 审计日志控制器

	mq.StartOrderConsumer(orderService)
	mq.StartDeadLetterConsumer(orderService)
	log.L().Info("MQ消费者全部启动完成")
	// [修复] 启动 Kafka 行为追踪消费者
	go kafka.StartBehaviorConsumer()

	// [修复] 启动配置热更新监听，修改 config.yaml 后自动重载
	config.WatchConfig(func(newCfg *config.Config) {
		log.L().Infow("配置热更新已生效", "global_qps", newCfg.Sentinel.GlobalQPS, "seckill_qps", newCfg.Sentinel.SeckillQPS)
	})

	cron.StartReconciler()
	log.L().Info("定时对账任务启动完成")

	gin.SetMode(cfg.Server.Mode)
	router := gin.New()

	// [优化] 压测模式：跳过 TraceID/RequestLog/CORS 中间件，减少每个请求的日志 IO 开销
	if cfg.Server.PerformanceMode {
		log.L().Info("性能模式已启用 — 跳过 TraceID/RequestLog/CORS 中间件")
		router.Use(middleware.RecoveryMiddleware(), middleware.BodyLimitMiddleware(cfg.Server.MaxBodyBytes), middleware.MetricsMiddleware(), middleware.SignMiddleware(), middleware.TokenBlacklistMiddleware())
	} else {
		router.Use(middleware.TraceIDMiddleware(), middleware.RequestLogMiddleware(), middleware.RecoveryMiddleware(), middleware.CORSMiddleware(), middleware.BodyLimitMiddleware(cfg.Server.MaxBodyBytes), middleware.MetricsMiddleware(), middleware.SignMiddleware(), middleware.TokenBlacklistMiddleware())
	}

	// [修复] 初始化 Jaeger 全链路追踪（如果启用）
	if cfg.Tracing.Enabled {
		tp, err := middleware.InitTracer(cfg.Server.Name, cfg.Tracing.Endpoint)
		if err != nil {
			log.L().Warnw("Jaeger链路追踪初始化失败", "error", err)
		} else {
			defer tp.Shutdown(context.Background())
			router.Use(middleware.TracingMiddleware())
			log.L().Info("Jaeger全链路追踪已启用")
		}
	}

	router.GET("/health", controller.Health)
	// [创新] WebSocket 实时推送端点
	router.GET("/ws", middleware.AuthMiddleware(jwtManager), func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		wsHub.HandleWebSocket(c.Writer, c.Request, userID.(int64))
	})
	// Swagger API 文档路由
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	// [修复] 静态文件服务：提供商品图片上传目录访问
	router.Static("/uploads", "./uploads")
	// [修复] 登录/注册接口添加限流中间件，防止暴力破解（同一IP每分钟最多10次）
	router.POST("/api/v1/user/register", middleware.LoginRateLimitMiddleware(10, 60), userController.Register)
	router.POST("/api/v1/user/login", middleware.LoginRateLimitMiddleware(10, 60), userController.Login)
	// [修复] 忘记密码接口：无需登录但需限流防止暴力破解
	router.POST("/api/v1/user/forgot-password", middleware.LoginRateLimitMiddleware(5, 60), userController.ForgotPassword)

	api := router.Group("/api/v1")
	api.Use(middleware.AuthMiddleware(jwtManager))
	{
		api.POST("/product", productController.CreateProduct)
		api.PUT("/product/:id", productController.UpdateProduct)
		api.GET("/product/list", productController.GetProductList)
		api.GET("/product/active", productController.GetActiveProducts)
		api.GET("/product/:id", productController.GetProductDetail)
		api.POST("/product/batch", productController.BatchImportProducts) // [修复] 商品批量导入
		api.POST("/product/upload", productController.UploadImage)        // [修复] 商品图片上传
		api.POST("/seckill", seckillController.Seckill)
		api.GET("/seckill/path", seckillController.GetSeckillPath)          // [创新] 秒杀地址隐藏
		api.GET("/seckill/captcha", seckillController.GetCaptcha)           // [创新] 数学验证码
		api.GET("/seckill/purchased", seckillController.GetPurchasedCounts) // [修复] 用户已购数量（恢复限购按钮状态）
		api.GET("/order/list", orderController.GetUserOrders)
		api.GET("/order/:order_no", orderController.GetOrderDetail)
		api.POST("/order/cancel", orderController.CancelOrder)
		// [修复] 支付回调 + 退款流程
		api.POST("/order/pay-callback", orderController.PayCallback)
		api.POST("/order/refund", orderController.Refund)
		// 用户管理
		api.GET("/user/list", userController.GetUserList)
		api.PUT("/user/role", userController.UpdateUserRole)
		api.GET("/user/info", userController.GetUserInfo)
		api.PUT("/user/password", userController.ChangePassword) // [修复] 密码修改
		// 订单管理（管理员）
		api.GET("/order/all", orderController.GetAllOrders)
		api.GET("/order/export", orderController.ExportOrders)
		api.POST("/order/import", orderController.ImportOrders) // [修复] 订单批量导入
		api.GET("/order/recon-diff", orderController.GetReconDiff)
		api.POST("/order/recon-fix", orderController.FixReconDiff)
		// 黑名单管理
		api.GET("/sentinel/blacklist", blacklistController.GetBlacklist)
		api.POST("/sentinel/blacklist", blacklistController.AddBlacklist)
		api.DELETE("/sentinel/blacklist/:id", blacklistController.RemoveBlacklist)
		// Sentinel 规则管理
		api.GET("/sentinel/rules", sentinelController.GetRules)
		api.POST("/sentinel/rule", sentinelController.AddRule)
		api.PUT("/sentinel/rule/:id", sentinelController.UpdateRule) // [修复] 更新规则
		api.DELETE("/sentinel/rule/:id", sentinelController.DeleteRule)
		// 监控
		api.GET("/monitor/metrics", monitorController.GetMetrics)
		api.GET("/monitor/qps", monitorController.GetQPS)
		api.GET("/monitor/middleware", monitorController.GetMiddlewareStatus)
		api.GET("/monitor/alarms", monitorController.GetAlarms)
		api.GET("/monitor/slow-api", monitorController.GetSlowAPIs) // [修复] 慢接口TOP排行
		// 秒杀统计
		api.GET("/seckill/stats", monitorController.GetSeckillStats)
		// [修复] 秒杀活动配置 API
		api.GET("/activity", activityController.GetActivity)
		api.PUT("/activity", activityController.UpdateActivity)
		api.POST("/activity/cache-warmup", activityController.CacheWarmUp)
		api.POST("/activity/config", activityController.SaveActivityConfig) // [修复] 活动配置保存（限购数量等）
		// [修复] 审计日志
		api.GET("/audit/list", auditController.GetAuditLogs)
		// [创新] AI 异常检测 + WebSocket 监控端点
		api.GET("/monitor/anomaly", monitorController.GetAnomalyStats)
		api.GET("/monitor/ws-stats", monitorController.GetWSStats)
		// [增强] 数据大屏补全：实时流量/热销排行/库存状态
		api.GET("/monitor/pvuv", monitorController.GetPVUV)
		api.GET("/monitor/hot-products", monitorController.GetHotProducts)
		api.GET("/monitor/inventory", monitorController.GetInventory)
	}

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.L().Infow("HTTP服务启动", "addr", addr, "instance_id", cfg.Server.InstanceID)

	// [修复] 使用自定义 http.Server 配置完整超时参数，替代 gin 默认的 router.Run()
	// ReadTimeout：防止慢客户端耗尽连接；WriteTimeout：防止响应写入阻塞；IdleTimeout：释放空闲连接
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
		// [修复] 限制请求体大小，防止大请求耗尽内存
		MaxHeaderBytes: 1 << 20, // 1MB Header 限制
	}

	// [修复] 使用 channel 通知主 goroutine 退出，让 defer 中的 Close() 正常执行
	shutdownCh := make(chan struct{})
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.L().Info("收到退出信号，正在优雅关闭...")

		// [修复] 优雅关闭 HTTP 服务，等待现有请求处理完毕（最多 10 秒）
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.L().Errorw("HTTP服务关闭异常", "error", err)
		}
		log.L().Info("HTTP服务已停止接收新请求")
		log.Sync()
		close(shutdownCh)
	}()

	// [修复] 在独立 goroutine 中启动 HTTP 服务，主 goroutine 等待退出信号
	go func() {
		log.L().Infow("HTTP服务监听中", "addr", addr, "read_timeout", cfg.Server.ReadTimeout, "write_timeout", cfg.Server.WriteTimeout, "idle_timeout", cfg.Server.IdleTimeout)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.L().Fatalw("HTTP服务启动失败", "error", err)
		}
	}()

	// [修复] 等待退出信号，收到信号后主函数正常返回，执行所有 defer 中的 Close()
	<-shutdownCh
	log.L().Info("服务已优雅关闭")
}
