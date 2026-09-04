package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/utils"
)

// [修复] TokenBlacklist Token 黑名单，用于主动失效 Token（如退出登录、密码修改后）
var (
	tokenBlacklist     = make(map[string]time.Time)
	tokenBlacklistLock sync.RWMutex
)

// AddToBlacklist 将 Token 加入黑名单
func AddToBlacklist(token string, expiry time.Duration) {
	tokenBlacklistLock.Lock()
	defer tokenBlacklistLock.Unlock()
	// 只存储黑名单条目，到期后自动清理
	tokenBlacklist[token] = time.Now().Add(expiry)

	// 启动后台清理过期条目
	go func() {
		time.Sleep(expiry)
		tokenBlacklistLock.Lock()
		delete(tokenBlacklist, token)
		tokenBlacklistLock.Unlock()
	}()
}

// IsBlacklisted 检查 Token 是否在黑名单中
func IsBlacklisted(token string) bool {
	tokenBlacklistLock.RLock()
	defer tokenBlacklistLock.RUnlock()
	_, exists := tokenBlacklist[token]
	return exists
}

// TokenBlacklistMiddleware Token 黑名单校验中间件
// 在 AuthMiddleware 之后注册，检查 Token 是否已被主动失效
func TokenBlacklistMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}
		// 提取 Token 值
		token := ""
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}
		if token != "" && IsBlacklisted(token) {
			utils.Unauthorized(c, "Token已失效，请重新登录")
			c.Abort()
			return
		}
		c.Next()
	}
}

// CleanExpiredBlacklist 清理过期的黑名单条目（定时任务）
func CleanExpiredBlacklist() {
	tokenBlacklistLock.Lock()
	defer tokenBlacklistLock.Unlock()
	now := time.Now()
	for token, expiry := range tokenBlacklist {
		if now.After(expiry) {
			delete(tokenBlacklist, token)
		}
	}
}
