package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/miaosha/config"
	"github.com/miaosha/log"
)

// [修复] DingTalkMessage 钉钉机器人 Markdown 消息结构
type DingTalkMessage struct {
	MsgType  string                  `json:"msgtype"`
	Markdown DingTalkMarkdownContent `json:"markdown"`
}

// [修复] DingTalkMarkdownContent 钉钉 Markdown 内容
type DingTalkMarkdownContent struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// [修复] SendDingTalkAlert 发送钉钉机器人告警推送
// 从配置中读取 webhook_url 和 secret，构建 Markdown 格式消息并发送 HTTP POST 请求
func SendDingTalkAlert(title, content string) error {
	cfg := config.GetConfig()
	if !cfg.DingTalk.Enabled {
		// [修复] 钉钉告警未启用，直接返回
		return nil
	}

	webhookURL := cfg.DingTalk.WebhookURL
	secret := cfg.DingTalk.Secret

	// [修复] 构建钉钉签名：timestamp + "\n" + secret，使用 HMAC-SHA256 计算签名
	timestamp := time.Now().UnixMilli()
	sign, err := DingTalkSign(secret, timestamp)
	if err != nil {
		return fmt.Errorf("钉钉签名生成失败: %w", err)
	}

	// [修复] 拼接签名参数到 webhook URL
	fullURL := fmt.Sprintf("%s&timestamp=%d&sign=%s", webhookURL, timestamp, sign)

	// [修复] 构建 Markdown 格式消息
	msg := DingTalkMessage{
		MsgType: "markdown",
		Markdown: DingTalkMarkdownContent{
			Title: title,
			Text:  content,
		},
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("钉钉消息序列化失败: %w", err)
	}

	// [修复] 发送 HTTP POST 请求，支持 3 次重试
	client := &http.Client{Timeout: 5 * time.Second}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := client.Post(fullURL, "application/json", bytes.NewReader(msgBytes))
		if err != nil {
			lastErr = err
			if attempt < 3 { time.Sleep(time.Duration(attempt) * 500 * time.Millisecond) }
			continue
		}
		// [修复] 检查响应状态码
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("钉钉返回非200: %d %s", resp.StatusCode, string(body))
			if attempt < 3 { time.Sleep(time.Duration(attempt) * 500 * time.Millisecond) }
			continue
		}
		resp.Body.Close()
		log.L().Infow("钉钉告警已发送", "title", title)
		return nil
	}
	return fmt.Errorf("钉钉告警发送失败(已重试3次): %w", lastErr)
}

// [修复] SendDingTalkAlertWithTrace 发送钉钉告警并附带 TraceID
// 用于运维快速定位问题，在告警消息中附带 TraceID、详细错误日志
func SendDingTalkAlertWithTrace(title, content, traceID, errorLog string) {
	cfg := config.GetConfig()
	if !cfg.DingTalk.Enabled {
		return
	}

	// [修复] 构建带 TraceID 的 Markdown 消息
	fullContent := fmt.Sprintf("## %s\n\n"+
		"- **时间**: %s\n"+
		"- **TraceID**: %s\n"+
		"- **详情**: %s\n\n"+
		"%s\n\n"+
		"> 请及时排查处理",
		title,
		time.Now().Format("2006-01-02 15:04:05"),
		traceID,
		content,
		errorLog,
	)
	if err := SendDingTalkAlert(title, fullContent); err != nil {
		log.L().Errorw("发送钉钉告警失败", "trace_id", traceID, "error", err)
	}
}

// [修复] SendPanicAlert 发送 Panic 告警到钉钉
func SendPanicAlert(traceID, path string, panicErr interface{}) {
	title := "【秒杀系统】服务 Panic 告警"
	content := fmt.Sprintf("## 秒杀系统 Panic 告警\n\n"+
		"- **时间**: %s\n"+
		"- **TraceID**: %s\n"+
		"- **路径**: %s\n"+
		"- **错误**: %v\n\n"+
		"> 请立即检查服务状态！",
		time.Now().Format("2006-01-02 15:04:05"),
		traceID,
		path,
		panicErr,
	)
	if err := SendDingTalkAlert(title, content); err != nil {
		log.L().Errorw("发送Panic钉钉告警失败", "error", err)
	}
}