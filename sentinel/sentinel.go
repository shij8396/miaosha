package sentinel

import (
	"fmt"
	"sync"

	sentinel "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/alibaba/sentinel-golang/core/circuitbreaker"
	sentinelConfig "github.com/alibaba/sentinel-golang/core/config"
	"github.com/alibaba/sentinel-golang/core/flow"
	"github.com/alibaba/sentinel-golang/core/hotspot"
	"github.com/miaosha/config"
	"github.com/miaosha/log"
)

var (
	initOnce sync.Once
	enabled  bool
)

func Init(cfg *config.SentinelConfig) error {
	if !cfg.Enabled { log.L().Info("Sentinel流量防护已禁用"); enabled = false; return nil }
	enabled = true
	var initErr error
	initOnce.Do(func() {
		sc := sentinelConfig.NewDefaultConfig()
		sc.Sentinel.Log.Dir = cfg.LogDir
		sc.Sentinel.App.Name = cfg.AppName
		if err := sentinel.InitWithConfig(sc); err != nil { initErr = fmt.Errorf("Sentinel初始化失败: %w", err); return }

		// 全局限流
		_, err := flow.LoadRules([]*flow.Rule{
			{Resource: "seckill_api", TokenCalculateStrategy: flow.Direct, ControlBehavior: flow.Reject, Threshold: float64(cfg.SeckillQPS), StatIntervalInMs: 1000},
			{Resource: "global_api", TokenCalculateStrategy: flow.Direct, ControlBehavior: flow.Reject, Threshold: float64(cfg.GlobalQPS), StatIntervalInMs: 1000},
		})
		if err != nil { initErr = fmt.Errorf("加载全局限流规则失败: %w", err); return }

		// 热点参数限流
		_, err = hotspot.LoadRules([]*hotspot.Rule{
			{Resource: "seckill_product", MetricType: hotspot.QPS, ControlBehavior: hotspot.Reject, ParamIndex: 0, Threshold: int64(cfg.HotParam.Threshold), DurationInSec: int64(cfg.HotParam.DurationSec)},
		})
		if err != nil { initErr = fmt.Errorf("加载热点参数规则失败: %w", err); return }

		// 熔断
		cbCfg := cfg.CircuitBreaker
		_, err = circuitbreaker.LoadRules([]*circuitbreaker.Rule{
			{Resource: "seckill_api", Strategy: circuitbreaker.SlowRequestRatio, RetryTimeoutMs: uint32(cbCfg.RecoveryTimeoutMs), MinRequestAmount: uint64(cbCfg.MinRequestAmount), StatIntervalMs: uint32(cbCfg.StatIntervalMs), MaxAllowedRtMs: uint64(cbCfg.MaxRTMs), Threshold: cbCfg.MaxRTRatio},
			{Resource: "seckill_api", Strategy: circuitbreaker.ErrorRatio, RetryTimeoutMs: uint32(cbCfg.RecoveryTimeoutMs), MinRequestAmount: uint64(cbCfg.MinRequestAmount), StatIntervalMs: uint32(cbCfg.StatIntervalMs), Threshold: 0.5},
		})
		if err != nil { initErr = fmt.Errorf("加载熔断规则失败: %w", err); return }

		log.L().Infow("Sentinel流量防护初始化完成", "global_qps", cfg.GlobalQPS, "seckill_qps", cfg.SeckillQPS)
	})
	return initErr
}

func Entry(resource string) (*base.SentinelEntry, error) {
	if !enabled { return nil, nil }
	return sentinel.Entry(resource)
}
func EntryWithArgs(resource string, args ...interface{}) (*base.SentinelEntry, error) {
	if !enabled { return nil, nil }
	return sentinel.Entry(resource, sentinel.WithArgs(args...))
}

// [修复] ReloadRules 从配置重新加载所有 Sentinel 规则，支持热更新
func ReloadRules(cfg *config.SentinelConfig) error {
	if !cfg.Enabled || !enabled {
		log.L().Info("Sentinel未启用，跳过规则热更新")
		return nil
	}

	// [修复] 重新加载全局限流规则
	_, err := flow.LoadRules([]*flow.Rule{
		{Resource: "seckill_api", TokenCalculateStrategy: flow.Direct, ControlBehavior: flow.Reject, Threshold: float64(cfg.SeckillQPS), StatIntervalInMs: 1000},
		{Resource: "global_api", TokenCalculateStrategy: flow.Direct, ControlBehavior: flow.Reject, Threshold: float64(cfg.GlobalQPS), StatIntervalInMs: 1000},
	})
	if err != nil {
		return fmt.Errorf("重新加载全局限流规则失败: %w", err)
	}

	// [修复] 重新加载热点参数限流规则
	_, err = hotspot.LoadRules([]*hotspot.Rule{
		{Resource: "seckill_product", MetricType: hotspot.QPS, ControlBehavior: hotspot.Reject, ParamIndex: 0, Threshold: int64(cfg.HotParam.Threshold), DurationInSec: int64(cfg.HotParam.DurationSec)},
	})
	if err != nil {
		return fmt.Errorf("重新加载热点参数规则失败: %w", err)
	}

	// [修复] 重新加载熔断规则
	cbCfg := cfg.CircuitBreaker
	_, err = circuitbreaker.LoadRules([]*circuitbreaker.Rule{
		{Resource: "seckill_api", Strategy: circuitbreaker.SlowRequestRatio, RetryTimeoutMs: uint32(cbCfg.RecoveryTimeoutMs), MinRequestAmount: uint64(cbCfg.MinRequestAmount), StatIntervalMs: uint32(cbCfg.StatIntervalMs), MaxAllowedRtMs: uint64(cbCfg.MaxRTMs), Threshold: cbCfg.MaxRTRatio},
		{Resource: "seckill_api", Strategy: circuitbreaker.ErrorRatio, RetryTimeoutMs: uint32(cbCfg.RecoveryTimeoutMs), MinRequestAmount: uint64(cbCfg.MinRequestAmount), StatIntervalMs: uint32(cbCfg.StatIntervalMs), Threshold: 0.5},
	})
	if err != nil {
		return fmt.Errorf("重新加载熔断规则失败: %w", err)
	}

	log.L().Infow("Sentinel规则热更新完成", "global_qps", cfg.GlobalQPS, "seckill_qps", cfg.SeckillQPS)
	return nil
}

// [修复] UpdateFlowRule 动态修改指定资源的 QPS 阈值，支持运行时调整
func UpdateFlowRule(resource string, count float64) error {
	if !enabled {
		return fmt.Errorf("Sentinel未启用")
	}
	_, err := flow.LoadRules([]*flow.Rule{
		{Resource: resource, TokenCalculateStrategy: flow.Direct, ControlBehavior: flow.Reject, Threshold: count, StatIntervalInMs: 1000},
	})
	if err != nil {
		return fmt.Errorf("更新限流规则失败: resource=%s, count=%.0f, error=%w", resource, count, err)
	}
	log.L().Infow("Sentinel限流规则已更新", "resource", resource, "threshold", count)
	return nil
}