// Package detector 实现基于滑动窗口 + Z-Score 的实时异常检测引擎
// 纯 Go 实现，零外部依赖，O(1) 时间复杂度，适用于高并发秒杀场景
// 检测维度：请求频率、间隔方差、成功率、IP 切换频率
// 异常分数 > 阈值自动触发黑名单联动
package detector

import (
	"math"
	"sync"
	"time"
)

// BehaviorSample 用户行为采样点
type BehaviorSample struct {
	Timestamp    time.Time
	Interval     float64 // 距上次请求间隔（秒）
	Success      bool    // 是否成功
	IPChanged    bool    // IP 是否变更
	RequestCount int     // 窗口内请求数
}

// UserProfile 用户行为画像（滑动窗口）
type UserProfile struct {
	UserID       int64
	Samples      []BehaviorSample // 最近 N 次行为采样
	WindowSize   int             // 窗口大小
	Idx          int             // 环形缓冲区写入位置
	mu           sync.Mutex
	lastIP       string
	lastTime     time.Time
	blockedUntil time.Time // 封禁截止时间
	blockReason  string
}

// AnomalyConfig 异常检测配置
type AnomalyConfig struct {
	WindowSize         int     // 滑动窗口采样数（默认 20）
	FreqThreshold      float64 // 请求频率 Z-Score 阈值（默认 3.0）
	IntervalThreshold  float64 // 间隔方差 Z-Score 阈值（默认 2.5）
	SuccessThreshold   float64 // 成功率异常阈值（默认 0.3，即成功率 < 30%）
	BlockDuration      time.Duration // 自动封禁时长
	MinSamplesForDetect int    // 最少采样数才开始检测（默认 5）
}

// DefaultConfig 默认配置：适合秒杀场景的保守参数
var DefaultConfig = AnomalyConfig{
	WindowSize:          20,
	FreqThreshold:       3.0,
	IntervalThreshold:   2.5,
	SuccessThreshold:    0.3,
	BlockDuration:       5 * time.Minute,
	MinSamplesForDetect: 5,
}

// AnomalyResult 检测结果
type AnomalyResult struct {
	Score       float64 // 综合异常分数 0-100
	IsAnomaly   bool
	Reasons     []string // 异常原因列表
	FreqZScore  float64
	IntervalStd float64
	SuccessRate float64
}

// AnomalyDetector 异常检测引擎
type AnomalyDetector struct {
	profiles map[int64]*UserProfile
	config   AnomalyConfig
	mu       sync.RWMutex

	// 全局统计（用于 Z-Score 基准）
	globalFreqMean    float64
	globalFreqStd     float64
	globalIntervalMean float64
	globalIntervalStd float64
	globalStatsMu     sync.RWMutex
	globalStatsCount  int
}

// NewAnomalyDetector 创建检测引擎
func NewAnomalyDetector(config AnomalyConfig) *AnomalyDetector {
	if config.WindowSize <= 0 {
		config.WindowSize = DefaultConfig.WindowSize
	}
	if config.MinSamplesForDetect <= 0 {
		config.MinSamplesForDetect = DefaultConfig.MinSamplesForDetect
	}
	if config.BlockDuration <= 0 {
		config.BlockDuration = DefaultConfig.BlockDuration
	}

	ad := &AnomalyDetector{
		profiles: make(map[int64]*UserProfile),
		config:   config,
	}

	// 定期清理过期 profile，防止内存泄漏
	go ad.cleanupLoop()
	return ad
}

// Record 记录一次用户行为，返回检测结果
func (ad *AnomalyDetector) Record(userID int64, success bool, clientIP string, requestTime time.Time) *AnomalyResult {
	ad.mu.Lock()
	profile, exists := ad.profiles[userID]
	if !exists {
		profile = &UserProfile{
			UserID:     userID,
			Samples:    make([]BehaviorSample, ad.config.WindowSize),
			WindowSize: ad.config.WindowSize,
			lastIP:     clientIP,
			lastTime:   requestTime,
		}
		ad.profiles[userID] = profile
	}
	ad.mu.Unlock()

	profile.mu.Lock()
	defer profile.mu.Unlock()

	// 检查是否在封禁期
	if time.Now().Before(profile.blockedUntil) {
		return &AnomalyResult{
			Score:     100,
			IsAnomaly: true,
			Reasons:   []string{profile.blockReason},
		}
	}

	// 计算间隔
	interval := 0.0
	ipChanged := false
	if !profile.lastTime.IsZero() {
		interval = requestTime.Sub(profile.lastTime).Seconds()
	}
	if profile.lastIP != "" && profile.lastIP != clientIP {
		ipChanged = true
	}

	// 写入环形缓冲区
	sample := BehaviorSample{
		Timestamp:    requestTime,
		Interval:     interval,
		Success:      success,
		IPChanged:    ipChanged,
		RequestCount: 0,
	}
	profile.Samples[profile.Idx%profile.WindowSize] = sample
	profile.Idx++
	profile.lastIP = clientIP
	profile.lastTime = requestTime

	// 采样不足，不检测
	actualCount := profile.Idx
	if actualCount < ad.config.MinSamplesForDetect {
		return &AnomalyResult{Score: 0, IsAnomaly: false}
	}
	sampleSize := min(actualCount, profile.WindowSize)

	// 提取有效采样
	validSamples := make([]BehaviorSample, sampleSize)
	for i := 0; i < sampleSize; i++ {
		pos := (profile.Idx - 1 - i + profile.WindowSize) % profile.WindowSize
		validSamples[sampleSize-1-i] = profile.Samples[pos]
	}

	// 计算行为指标
	freqPerSec := float64(sampleSize) / max(validSamples[sampleSize-1].Timestamp.Sub(validSamples[0].Timestamp).Seconds(), 0.1)
	intervalMean, intervalStd := calcIntervalStats(validSamples)
	successCount := 0
	ipChangeCount := 0
	for _, s := range validSamples {
		if s.Success {
			successCount++
		}
		if s.IPChanged {
			ipChangeCount++
		}
	}
	successRate := float64(successCount) / float64(sampleSize)

	// 更新全局统计基准
	ad.updateGlobalStats(freqPerSec, intervalMean)

	// ===== Z-Score 异常检测 =====
	var reasons []string
	anomalyScore := 0.0

	ad.globalStatsMu.RLock()
	gFreqMean := ad.globalFreqMean
	gFreqStd := ad.globalFreqStd
	gIntervalMean := ad.globalIntervalMean
	gIntervalStd := ad.globalIntervalStd
	ad.globalStatsMu.RUnlock()

	// 1. 请求频率 Z-Score（频率过高 = 脚本/黄牛）
	if gFreqStd > 0 && ad.globalStatsCount > 10 {
		freqZScore := (freqPerSec - gFreqMean) / gFreqStd
		if freqZScore > ad.config.FreqThreshold {
			reasons = append(reasons, "请求频率异常偏高")
			anomalyScore += 25
		}
	}

	// 2. 请求间隔方差（方差过小 = 机器定时请求，过大 = 行为不稳定）
	if gIntervalStd > 0 && ad.globalStatsCount > 10 {
		intervalZScore := math.Abs(intervalStd-gIntervalMean) / gIntervalStd
		if intervalZScore > ad.config.IntervalThreshold {
			if intervalStd < gIntervalStd {
				reasons = append(reasons, "请求过于规律（疑似脚本）")
			} else {
				reasons = append(reasons, "请求间隔波动异常")
			}
			anomalyScore += 20
		}
	}

	// 3. 成功率异常（成功率过低 = 可能是撞库/恶意试探）
	if successRate < ad.config.SuccessThreshold {
		reasons = append(reasons, "成功率异常偏低")
		anomalyScore += 25
	}

	// 4. IP 切换频率（频繁切换 IP = 代理/VPN 规避限流）
	if ipChangeCount > sampleSize/2 {
		reasons = append(reasons, "IP 频繁切换")
		anomalyScore += 15
	}

	// 5. 极端高频（秒级 > 50 次 = 确定攻击）
	if freqPerSec > 50 {
		reasons = append(reasons, "极端高频请求")
		anomalyScore += 40
	}

	isAnomaly := anomalyScore >= 50

	// 自动封禁
	if isAnomaly && anomalyScore >= 70 {
		profile.blockedUntil = time.Now().Add(ad.config.BlockDuration)
		profile.blockReason = "AI异常检测自动封禁: " + joinReasons(reasons)
	}

	return &AnomalyResult{
		Score:       math.Min(anomalyScore, 100),
		IsAnomaly:   isAnomaly,
		Reasons:     reasons,
		FreqZScore:  (freqPerSec - gFreqMean) / max(gFreqStd, 0.001),
		IntervalStd: intervalStd,
		SuccessRate: successRate,
	}
}

// IsBlocked 检查用户是否处于自动封禁状态
func (ad *AnomalyDetector) IsBlocked(userID int64) (bool, string) {
	ad.mu.RLock()
	profile, exists := ad.profiles[userID]
	ad.mu.RUnlock()

	if !exists {
		return false, ""
	}

	profile.mu.Lock()
	defer profile.mu.Unlock()

	if time.Now().Before(profile.blockedUntil) {
		return true, profile.blockReason
	}
	return false, ""
}

// GetProfile 获取用户行为画像（用于调试/监控）
func (ad *AnomalyDetector) GetProfile(userID int64) *UserProfile {
	ad.mu.RLock()
	defer ad.mu.RUnlock()
	return ad.profiles[userID]
}

// ProfileCount 获取当前监控的用户数
func (ad *AnomalyDetector) ProfileCount() int {
	ad.mu.RLock()
	defer ad.mu.RUnlock()
	return len(ad.profiles)
}

// updateGlobalStats 更新全局统计基准（指数移动平均，避免历史数据影响）
func (ad *AnomalyDetector) updateGlobalStats(freq, intervalMean float64) {
	ad.globalStatsMu.Lock()
	defer ad.globalStatsMu.Unlock()

	ad.globalStatsCount++
	alpha := 0.01 // 平滑因子，对新数据敏感

	if ad.globalStatsCount == 1 {
		ad.globalFreqMean = freq
		ad.globalFreqStd = 0
		ad.globalIntervalMean = intervalMean
		ad.globalIntervalStd = 0
		return
	}

	// EMA 更新均值和标准差
	ad.globalFreqMean = alpha*freq + (1-alpha)*ad.globalFreqMean
	ad.globalFreqStd = alpha*math.Abs(freq-ad.globalFreqMean) + (1-alpha)*ad.globalFreqStd
	ad.globalIntervalMean = alpha*intervalMean + (1-alpha)*ad.globalIntervalMean
	ad.globalIntervalStd = alpha*math.Abs(intervalMean-ad.globalIntervalMean) + (1-alpha)*ad.globalIntervalStd
}

// cleanupLoop 定期清理长时间未活跃的 profile，防止内存泄漏
func (ad *AnomalyDetector) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ad.mu.Lock()
		for uid, profile := range ad.profiles {
			profile.mu.Lock()
			inactive := time.Since(profile.lastTime) > 30*time.Minute
			profile.mu.Unlock()
			if inactive {
				delete(ad.profiles, uid)
			}
		}
		ad.mu.Unlock()
	}
}

// calcIntervalStats 计算请求间隔的均值和标准差
func calcIntervalStats(samples []BehaviorSample) (mean, std float64) {
	if len(samples) < 2 {
		return 0, 0
	}

	// 跳过第一个（无间隔数据）
	intervals := make([]float64, 0, len(samples)-1)
	for i := 1; i < len(samples); i++ {
		if samples[i].Interval > 0 && samples[i].Interval < 300 { // 忽略 > 5min 的间隔
			intervals = append(intervals, samples[i].Interval)
		}
	}

	if len(intervals) == 0 {
		return 0, 0
	}

	sum := 0.0
	for _, v := range intervals {
		sum += v
	}
	mean = sum / float64(len(intervals))

	variance := 0.0
	for _, v := range intervals {
		variance += (v - mean) * (v - mean)
	}
	std = math.Sqrt(variance / float64(len(intervals)))

	return mean, std
}

func joinReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	result := reasons[0]
	for i := 1; i < len(reasons); i++ {
		result += "; " + reasons[i]
	}
	return result
}