// 压测环境初始化：创建测试商品 + 配置秒杀活动
// 用法: go run stress_test/cmd/setup.go
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	BaseURL  = "http://localhost:8080"
	Secret   = "miaosha-sign-secret-2026"
	TestUser = "admin"
	TestPass = "admin123"
)

func genSign(ts int64, method, path, body string) string {
	payload := strconv.FormatInt(ts, 10) + method + path + body
	h := hmac.New(sha256.New, []byte(Secret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

func doRequest(method, path string, body []byte, token string) (*http.Response, []byte, error) {
	ts := time.Now().Unix()
	bodyStr := string(body)
	sign := genSign(ts, method, path, bodyStr)

	req, _ := http.NewRequest(method, BaseURL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Sign", sign)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, respBody, nil
}

func main() {
	fmt.Println("=== 压测环境初始化 ===")

	// 1. 登录
	fmt.Println("[1/4] 登录...")
	loginBody, _ := json.Marshal(map[string]string{"username": TestUser, "password": TestPass})
	resp, respBody, _ := doRequest("POST", "/api/v1/user/login", loginBody, "")
	if resp.StatusCode != 200 {
		fmt.Printf("❌ 登录失败: %d %s\n", resp.StatusCode, string(respBody))
		return
	}
	var loginResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	json.Unmarshal(respBody, &loginResp)
	token := loginResp.Data.Token
	fmt.Println("✅ 登录成功")

	// 2. 创建测试商品
	fmt.Println("[2/4] 创建测试商品（库存 10000）...")
	productBody, _ := json.Marshal(map[string]interface{}{
		"name":           "压测秒杀商品",
		"price":          99.99,
		"seckill_price":  9.99,
		"total_stock":    10000,
		"description":    "压力测试专用商品",
		"start_time":     time.Now().Format("2006-01-02 15:04:05"),
		"end_time":       time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05"),
		"limit_per_user": 3,
		"image_url":      "",
	})
	resp, respBody, _ = doRequest("POST", "/api/v1/product", productBody, token)
	if resp.StatusCode != 200 {
		fmt.Printf("❌ 创建商品失败: %d %s\n", resp.StatusCode, string(respBody))
		return
	}
	var productResp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Stock int    `json:"stock"`
		} `json:"data"`
	}
	json.Unmarshal(respBody, &productResp)
	productID := productResp.Data.ID
	fmt.Printf("   原始响应: %s\n", string(respBody))
	fmt.Printf("✅ 商品创建成功: ID=%d, 名称=%s, 库存=%d\n", productID, productResp.Data.Name, productResp.Data.Stock)

	// 3. 上架商品（开启秒杀活动）
	fmt.Println("[3/4] 开启秒杀活动...")
	activityBody, _ := json.Marshal(map[string]interface{}{
		"product_id": productID,
		"status":     1,
	})
	resp, respBody, _ = doRequest("PUT", "/api/v1/activity", activityBody, token)
	if resp.StatusCode != 200 {
		fmt.Printf("❌ 上架失败: %d %s\n", resp.StatusCode, string(respBody))
		return
	}
	fmt.Printf("✅ 商品已上架为秒杀活动: %s\n", string(respBody))

	// 4. 配置活动参数（限购数量等）
	fmt.Println("[4/4] 配置活动参数...")
	configBody, _ := json.Marshal(map[string]interface{}{
		"product_id": productID,
		"limit_num":  3,
		"start_time": time.Now().Format("2006-01-02 15:04:05"),
		"end_time":   time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05"),
	})
	resp, respBody, _ = doRequest("POST", "/api/v1/activity/config", configBody, token)
	if resp.StatusCode != 200 {
		fmt.Printf("⚠️  活动配置: %d %s\n", resp.StatusCode, string(respBody))
	} else {
		fmt.Printf("✅ 活动配置成功: %s\n", string(respBody))
	}

	// 5. 预热缓存
	fmt.Println("[额外] 预热 Redis 库存缓存...")
	warmupBody, _ := json.Marshal(map[string]interface{}{
		"product_id": productID,
	})
	resp, respBody, _ = doRequest("POST", "/api/v1/activity/cache-warmup", warmupBody, token)
	fmt.Printf("   预热结果: %d %s\n", resp.StatusCode, string(respBody))

	fmt.Println("===========================================")
	fmt.Printf("📦 测试商品 ID: %d\n", productID)
	fmt.Printf("📦 库存: 10000\n")
	fmt.Printf("📦 限购: 3 件/人\n")
	fmt.Println("===========================================")
	fmt.Println("\n✅ 压测环境初始化完成！可以运行压测了。")
	fmt.Println("   go run ./stress_test/cmd/main.go")
}
