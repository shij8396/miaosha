package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/config"
	"github.com/miaosha/log"
	"github.com/miaosha/monitor"
	"github.com/miaosha/service"
	"github.com/miaosha/utils"
)

func TraceIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" { traceID = utils.GenerateTraceID() }
		c.Set(utils.TraceIDKey, traceID)
		c.Header("X-Trace-ID", traceID)
		c.Next()
	}
}

func RequestLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		c.Next()
		duration := time.Since(start)
		log.L().Infow("HTTP请求",
			"trace_id", utils.GetTraceID(c), "method", c.Request.Method, "path", path,
			"status", c.Writer.Status(), "latency_ms", duration.Milliseconds(),
			"client_ip", c.ClientIP(),
		)
		// [修复] 慢接口监控：超过 500ms 的接口记录 WARN 日志并存储到慢接口排行
		if duration > 500*time.Millisecond {
			log.L().Warnw("慢接口告警", "path", path, "method", c.Request.Method, "duration_ms", duration.Milliseconds(), "trace_id", utils.GetTraceID(c))
			// [修复] 记录慢接口到 TOP 排行存储
			service.RecordSlowAPI(path, c.Request.Method, duration.Milliseconds())
		}
	}
}

func AuthMiddleware(jwtManager *utils.JWTManager) gin.HandlerFunc {
	whiteList := map[string]bool{"/api/v1/user/login": true, "/api/v1/user/register": true, "/metrics": true, "/health": true}
	return func(c *gin.Context) {
		if whiteList[c.Request.URL.Path] { c.Next(); return }
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" { utils.Unauthorized(c, "未提供认证Token"); c.Abort(); return }
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" { utils.Unauthorized(c, "Token格式错误"); c.Abort(); return }
		claims, err := jwtManager.ParseToken(parts[1])
		if err != nil { utils.Unauthorized(c, "Token验证失败"); c.Abort(); return }
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		// [修复] 兼容旧版Token（无role字段）：清空浏览器缓存后重新登录即可获取新版Token
		// 若role为空，默认视为"user"，避免管理员因旧Token触发限流
		role := claims.Role
		if role == "" {
			role = "user"
		}
		c.Set("role", role)
		c.Next()
	}
}

func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				traceID := utils.GetTraceID(c)
				path := c.Request.URL.Path
				log.L().Errorw("服务Panic", "trace_id", traceID, "error", err, "path", path)
				// [修复] Panic 发生时发送钉钉告警推送
				go utils.SendPanicAlert(traceID, path, err)
				utils.InternalError(c, "服务内部异常，请稍后重试")
				c.Abort()
			}
		}()
		c.Next()
	}
}

// [修复] CORS 中间件：从配置读取允许的域名白名单，避免 Access-Control-Allow-Origin: * 的安全风险
// 仅当请求 Origin 在白名单中时，才设置 Access-Control-Allow-Credentials: true
func CORSMiddleware() gin.HandlerFunc {
	cfg := config.GetConfig()
	allowedOrigins := cfg.CORS.AllowedOrigins
	allowedMethods := strings.Join(cfg.CORS.AllowedMethods, ", ")
	allowedHeaders := strings.Join(cfg.CORS.AllowedHeaders, ", ")

	// [修复] 构建白名单查找 map，提升匹配效率
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		originSet[origin] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		_, isAllowed := originSet[origin]

		// [修复] 仅当 Origin 在白名单中时才设置具体域名，否则不设置 CORS 头
		if isAllowed {
			c.Header("Access-Control-Allow-Origin", origin)
			// [修复] 仅当 Origin 在白名单中时才允许携带凭据（Cookie）
			c.Header("Access-Control-Allow-Credentials", "true")
		} else {
			// [修复] 不在白名单中的请求不设置 Access-Control-Allow-Origin
			c.Header("Access-Control-Allow-Origin", "")
		}
		c.Header("Access-Control-Allow-Methods", allowedMethods)
		c.Header("Access-Control-Allow-Headers", allowedHeaders)

		// [修复] 添加安全响应头，防止点击劫持、MIME类型嗅探和XSS攻击
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-XSS-Protection", "1; mode=block")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		// [增强] 实时统计引擎埋点：QPS 计数（每请求 +1）
		monitor.RecordRequest()
		monitor.IncHTTPRequestTotal(method, path)
		c.Next()
		// [增强] 实时统计引擎埋点：接口耗时（用于平均响应时间）
		monitor.RecordRequestLatency(float64(time.Since(start).Milliseconds()))
		monitor.ObserveHTTPRequestDuration(method, path, time.Since(start).Seconds())
		if c.Writer.Status() >= 400 { monitor.IncHTTPErrorTotal(method, path, fmt.Sprintf("%d", c.Writer.Status())) }
		// [增强] PV/UV 埋点：登录用户取 user_id（AuthMiddleware 在 Next 内已写入），未登录取客户端IP
		// 仅统计业务 API，排除 /metrics /health 等基础设施端点
		if strings.HasPrefix(path, "/api/") {
			visitorKey := c.ClientIP()
			if uid, ok := c.Get("user_id"); ok {
				if id, ok := uid.(int64); ok && id > 0 {
					visitorKey = fmt.Sprintf("u_%d", id)
				}
			}
			monitor.RecordVisit(visitorKey)
		}
	}
}

// [修复] BodyLimitMiddleware 请求体大小限制中间件
// 防止攻击者发送超大请求体耗尽服务器内存
// maxBytes 为允许的最大请求体字节数，默认 1MB
func BodyLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = 1 << 20 // 默认 1MB
	}
	return func(c *gin.Context) {
		// 仅对 POST/PUT/PATCH 请求限制 Body 大小
		method := c.Request.Method
		if method == "POST" || method == "PUT" || method == "PATCH" {
			if c.Request.ContentLength > maxBytes {
				utils.Error(c, 413, fmt.Sprintf("请求体过大，最大允许 %dMB", maxBytes/(1<<20)))
				c.Abort()
				return
			}
		}
		c.Next()
	}
}