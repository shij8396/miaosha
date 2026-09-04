// 临时诊断：复现压测工具验证码握手失败，打印具体错误
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
	"strings"
	"sync"
	"time"
)

const (
	BaseURL = "http://localhost:8080"
	Secret  = "miaosha-sign-secret-2026"
)

var httpClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		DisableKeepAlives:   false,
	},
	Timeout: 10 * time.Second,
}

func genSign(ts int64, method, path, body string) string {
	payload := strconv.FormatInt(ts, 10) + method + path + body
	h := hmac.New(sha256.New, []byte(Secret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

func doRequest(method, path string, body []byte, token string) (*http.Response, []byte, error) {
	ts := time.Now().Unix()
	signPath := path
	if i := strings.Index(signPath, "?"); i >= 0 {
		signPath = signPath[:i]
	}
	sign := genSign(ts, method, signPath, string(body))
	req, _ := http.NewRequest(method, BaseURL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Sign", sign)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, respBody, nil
}

func solveExpr(expr string) int {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimSuffix(expr, "=?")
	for _, op := range []string{"+", "-"} {
		if i := strings.Index(expr, op); i >= 0 {
			a, _ := strconv.Atoi(strings.TrimSpace(expr[:i]))
			b, _ := strconv.Atoi(strings.TrimSpace(expr[i+1:]))
			if op == "+" {
				return a + b
			}
			return a - b
		}
	}
	return 0
}

func fetchPathCaptcha(token string, productID int64) (string, string, int, error) {
	resp, ptBody, err := doRequest("GET", fmt.Sprintf("/api/v1/seckill/path?product_id=%d", productID), nil, token)
	if err != nil {
		return "", "", 0, fmt.Errorf("获取path_token失败: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", "", 0, fmt.Errorf("path HTTP %d: %s", resp.StatusCode, string(ptBody))
	}
	var ptResp struct {
		Data struct {
			PathToken string `json:"path_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(ptBody, &ptResp); err != nil || ptResp.Data.PathToken == "" {
		return "", "", 0, fmt.Errorf("path_token解析失败: %s", string(ptBody))
	}
	resp, cpBody, err := doRequest("GET", fmt.Sprintf("/api/v1/seckill/captcha?product_id=%d", productID), nil, token)
	if err != nil {
		return "", "", 0, fmt.Errorf("获取验证码失败: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", "", 0, fmt.Errorf("captcha HTTP %d: %s", resp.StatusCode, string(cpBody))
	}
	var cpResp struct {
		Data struct {
			CaptchaID  string `json:"captcha_id"`
			Expression string `json:"expression"`
		} `json:"data"`
	}
	if err := json.Unmarshal(cpBody, &cpResp); err != nil || cpResp.Data.CaptchaID == "" {
		return "", "", 0, fmt.Errorf("验证码解析失败: %s", string(cpBody))
	}
	return ptResp.Data.PathToken, cpResp.Data.CaptchaID, solveExpr(cpResp.Data.Expression), nil
}

func login(username string) string {
	body, _ := json.Marshal(map[string]string{"username": username, "password": "test123"})
	_, rb, _ := doRequest("POST", "/api/v1/user/login", body, "")
	var lr struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	json.Unmarshal(rb, &lr)
	return lr.Data.Token
}

func main() {
	// 登录 20 个 stress 用户
	tokens := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		t := login(fmt.Sprintf("stress_user_%d", i))
		if t == "" {
			fmt.Printf("用户%d登录失败\n", i)
			continue
		}
		tokens = append(tokens, t)
	}
	fmt.Printf("共获取 %d 个 token\n", len(tokens))

	var wg sync.WaitGroup
	var mu sync.Mutex
	start := time.Now()
	for i := 0; i < len(tokens); i++ {
		wg.Add(1)
		go func(tok string, idx int) {
			defer wg.Done()
			pt, cid, ans, err := fetchPathCaptcha(tok, 26)
			if err != nil {
				mu.Lock()
				fmt.Printf("[%d] 握手失败: %v\n", idx, err)
				mu.Unlock()
				return
			}
			body, _ := json.Marshal(map[string]interface{}{
				"product_id": 26, "quantity": 1, "path_token": pt,
				"captcha_id": cid, "captcha_code": ans,
				"idempotent_key": fmt.Sprintf("probe_%d_%d", idx, time.Now().UnixNano()),
			})
			_, rb, err := doRequest("POST", "/api/v1/seckill", body, tok)
			var sr struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			json.Unmarshal(rb, &sr)
			mu.Lock()
			if err != nil {
				fmt.Printf("[%d] seckill网络错误: %v\n", idx, err)
			} else {
				fmt.Printf("[%d] seckill code=%d msg=%s\n", idx, sr.Code, sr.Message)
			}
			mu.Unlock()
		}(tokens[i], i)
	}
	wg.Wait()
	fmt.Printf("总耗时: %v\n", time.Since(start))
}
