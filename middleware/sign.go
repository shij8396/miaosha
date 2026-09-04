package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/config"
	"github.com/miaosha/log"
	"github.com/miaosha/utils"
)

// SignMiddleware API 签名校验中间件
// 防止恶意非法调用后端接口，所有非白名单接口需要携带 HMAC-SHA256 签名
// 签名规则：Sign = HMAC-SHA256(Timestamp + Method + Path + Body, Secret)
// 请求头：X-Timestamp + X-Sign
// 签名有效期：60 秒（防止重放攻击）
func SignMiddleware() gin.HandlerFunc {
	cfg := config.GetConfig()
	secret := cfg.Server.SignSecret
	if secret == "" {
		// 签名密钥未配置，使用默认值（仅开发环境，生产环境必须通过配置或环境变量注入）
		secret = "default-dev-sign-secret-change-in-production"
		log.L().Warnw("签名密钥未配置，使用默认开发密钥，生产环境请务必修改", "file", "middleware/sign.go")
	}

	// 签名白名单：健康检查、Prometheus 指标、登录注册、图片上传等不需要签名
	whiteList := map[string]bool{
		"/health":                true,
		"/metrics":               true,
		"/api/v1/user/login":     true,
		"/api/v1/user/register":  true,
		"/api/v1/product/upload": true,
	}
	// [修复] 静态图片资源前缀放行：<img> 标签请求无法携带签名头
	// 未放行时 /uploads/* 请求返回 401"缺少签名参数"，导致商品图片无法显示
	staticPrefixes := []string{"/uploads/"}

	return func(c *gin.Context) {
		// 白名单放行：同时兼容原始 path 与带 query string 的 RawPath 写法
		if whiteList[c.Request.URL.Path] || whiteList[c.Request.URL.RawPath] {
			c.Next()
			return
		}

		// [修复] 静态资源前缀放行（如 /uploads/xxx.png）
		for _, prefix := range staticPrefixes {
			if strings.HasPrefix(c.Request.URL.Path, prefix) {
				c.Next()
				return
			}
		}

		// 获取签名头
		timestamp := c.GetHeader("X-Timestamp")
		sign := c.GetHeader("X-Sign")

		if timestamp == "" || sign == "" {
			utils.Error(c, 401, "缺少签名参数")
			c.Abort()
			return
		}

		// 验证时间戳有效期（60 秒内）
		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			utils.Error(c, 401, "签名时间戳格式错误")
			c.Abort()
			return
		}
		now := time.Now().Unix()
		if abs(now-ts) > 60 {
			utils.Error(c, 401, "签名已过期，请重新生成")
			c.Abort()
			return
		}

		// 读取请求 Body（需要保存副本供后续处理器使用）
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// 计算期望签名：HMAC-SHA256(Timestamp + Method + Path + Body, Secret)
		payload := timestamp + c.Request.Method + c.Request.URL.Path + string(bodyBytes)
		expectedSign := hmacSHA256(payload, secret)

		if sign != expectedSign {
			utils.Error(c, 401, "签名校验失败")
			c.Abort()
			return
		}

		c.Next()
	}
}

func hmacSHA256(data, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// GenerateSign 生成签名（供客户端/测试使用）
func GenerateSign(timestamp int64, method, path, body, secret string) string {
	payload := strconv.FormatInt(timestamp, 10) + method + path + body
	return hmacSHA256(payload, secret)
}
