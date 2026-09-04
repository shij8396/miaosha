package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	redisClient "github.com/miaosha/redis"
	"github.com/miaosha/utils"
)

// LoginRateLimitMiddleware 登录/注册接口限流中间件
// 基于 Redis 滑动窗口实现，防止暴力破解
// 默认：同一 IP 每分钟最多 10 次登录尝试
// 响应头包含 RFC 标准限流信息：
//   - X-RateLimit-Limit: 窗口内最大请求数
//   - X-RateLimit-Remaining: 窗口内剩余请求数
//   - X-RateLimit-Reset: 窗口重置时间（Unix 秒）
//   - Retry-After: 达到上限后建议重试秒数（仅 429 时返回）
func LoginRateLimitMiddleware(maxAttempts int, windowSeconds int) gin.HandlerFunc {
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	if windowSeconds <= 0 {
		windowSeconds = 60
	}

	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		key := fmt.Sprintf("rate_limit:login:%s", clientIP)

		ctx := context.Background()
		rdb := redisClient.GetClient()

		// 使用 Redis INCR + EXPIRE 实现滑动窗口计数
		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			// Redis 不可用时降级放行（避免阻塞正常登录）
			c.Next()
			return
		}

		// 首次访问时设置过期时间，并记录窗口起始时间
		if count == 1 {
			rdb.Expire(ctx, key, time.Duration(windowSeconds)*time.Second)
			// 记录窗口起始时间，用于计算 X-RateLimit-Reset
			resetKey := fmt.Sprintf("rate_limit:login:%s:reset", clientIP)
			rdb.Set(ctx, resetKey, time.Now().Unix()+int64(windowSeconds), time.Duration(windowSeconds)*time.Second)
		}

		// 获取窗口重置时间
		resetKey := fmt.Sprintf("rate_limit:login:%s:reset", clientIP)
		resetAt, _ := rdb.Get(ctx, resetKey).Int64()

		remaining := int64(maxAttempts) - count
		if remaining < 0 {
			remaining = 0
		}

		// 设置 RFC 标准限流响应头
		c.Header("X-RateLimit-Limit", strconv.Itoa(maxAttempts))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetAt, 10))

		if count > int64(maxAttempts) {
			ttl, _ := rdb.TTL(ctx, key).Result()
			retryAfter := int(ttl.Seconds())
			if retryAfter < 0 {
				retryAfter = windowSeconds
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			utils.Error(c, 429, fmt.Sprintf("登录尝试过于频繁，请 %d 秒后再试", retryAfter))
			c.Abort()
			return
		}

		c.Next()
	}
}
