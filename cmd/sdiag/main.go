package main

// 临时诊断：用与生产完全相同的 Sentinel 规则独立验证 Entry 行为（用后即删）

import (
	"fmt"

	sentinel "github.com/alibaba/sentinel-golang/api"
	sentinelConfig "github.com/alibaba/sentinel-golang/core/config"
	"github.com/alibaba/sentinel-golang/core/circuitbreaker"
	"github.com/alibaba/sentinel-golang/core/flow"
	"github.com/alibaba/sentinel-golang/core/hotspot"
)

func main() {
	sc := sentinelConfig.NewDefaultConfig()
	sc.Sentinel.App.Name = "diag"
	sc.Sentinel.Log.Dir = "./logs/sdiag"
	if err := sentinel.InitWithConfig(sc); err != nil {
		fmt.Println("init err:", err)
		return
	}

	// 与生产 sentinel.go 完全一致的规则
	flow.LoadRules([]*flow.Rule{
		{Resource: "seckill_api", TokenCalculateStrategy: flow.Direct, ControlBehavior: flow.Reject, Threshold: 5000, StatIntervalInMs: 1000},
	})
	hotspot.LoadRules([]*hotspot.Rule{
		{Resource: "seckill_product", MetricType: hotspot.QPS, ControlBehavior: hotspot.Reject, ParamIndex: 0, Threshold: 100, DurationInSec: 1},
	})
	circuitbreaker.LoadRules([]*circuitbreaker.Rule{
		{Resource: "seckill_api", Strategy: circuitbreaker.SlowRequestRatio, RetryTimeoutMs: 3000, MinRequestAmount: 10, StatIntervalMs: 1000, MaxAllowedRtMs: 1000, Threshold: 0.5},
		{Resource: "seckill_api", Strategy: circuitbreaker.ErrorRatio, RetryTimeoutMs: 3000, MinRequestAmount: 10, StatIntervalMs: 1000, Threshold: 0.5},
	})

	for i := 1; i <= 3; i++ {
		e, blockErr := sentinel.Entry("seckill_api")
		if blockErr != nil {
			fmt.Printf("第%d次: 被拒绝 BlockType=%s(%d) BlockMsg=%q rule=%T\n",
				i, blockErr.BlockType().String(), blockErr.BlockType(), blockErr.BlockMsg(), blockErr.TriggeredRule())
			continue
		}
		fmt.Printf("第%d次: 通过\n", i)
		e.Exit()
	}

	// [诊断] 热点参数限流：与生产 EntryWithArgs 完全一致
	fmt.Println("== 热点参数限流 EntryWithArgs(seckill_product, int64(20)) ==")
	for i := 1; i <= 3; i++ {
		e, blockErr := sentinel.Entry("seckill_product", sentinel.WithArgs(int64(20)))
		if blockErr != nil {
			fmt.Printf("热点第%d次: 被拒绝 BlockType=%s BlockMsg=%q rule=%+v\n",
				i, blockErr.BlockType().String(), blockErr.BlockMsg(), blockErr.TriggeredRule())
			continue
		}
		fmt.Printf("热点第%d次: 通过\n", i)
		e.Exit()
	}
	fmt.Println("诊断完成")
}
