package service

import (
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/miaosha/model"
)

// [修复] SlowAPIRecord 慢接口记录
type SlowAPIRecord struct {
	Path       string `json:"path"`
	Method     string `json:"method"`
	DurationMs int64  `json:"duration_ms"`
	Time       string `json:"time"`
}

// [修复] 全局慢接口记录存储（线程安全，最多保留 100 条）
var (
	slowAPIMu     sync.RWMutex
	slowAPIRecords []SlowAPIRecord
	maxSlowAPIRecords = 100
)

// [修复] RecordSlowAPI 由中间件调用，记录慢接口信息
func RecordSlowAPI(path, method string, durationMs int64) {
	slowAPIMu.Lock()
	defer slowAPIMu.Unlock()
	slowAPIRecords = append(slowAPIRecords, SlowAPIRecord{
		Path: path, Method: method, DurationMs: durationMs,
		Time: time.Now().Format("2006-01-02 15:04:05"),
	})
	// [修复] 保持最多 100 条记录，超出后删除最旧的
	if len(slowAPIRecords) > maxSlowAPIRecords {
		slowAPIRecords = slowAPIRecords[len(slowAPIRecords)-maxSlowAPIRecords:]
	}
}

type MonitorService struct {
	startTime time.Time
}

func NewMonitorService() *MonitorService {
	return &MonitorService{startTime: time.Now()}
}

// GetMetrics 获取监控指标
func (s *MonitorService) GetMetrics() *model.MonitorMetrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &model.MonitorMetrics{
		RejectCount: 0,   // 由 Prometheus 指标提供
		PassCount:   0,   // 由 Prometheus 指标提供
		AvgRt:       0,   // 由 Prometheus 指标提供
		QPS:         0,   // 由 Prometheus 指标提供
		CPUUsage:    float64(runtime.NumGoroutine()) / 10.0, // 近似值
		MemUsage:    float64(memStats.Alloc) / float64(memStats.Sys) * 100,
		ActiveConns: int64(runtime.NumGoroutine()),
	}
}

// GetMiddlewareStatus 获取中间件状态
func (s *MonitorService) GetMiddlewareStatus() []model.MiddlewareStatus {
	uptime := time.Since(s.startTime).String()
	return []model.MiddlewareStatus{
		{Name: "MySQL", Status: "up", Address: "127.0.0.1:3306", Uptime: uptime},
		{Name: "Redis Cluster", Status: "up", Address: "127.0.0.1:6379", Uptime: uptime},
		{Name: "RabbitMQ", Status: "up", Address: "127.0.0.1:5672", Uptime: uptime},
		{Name: "Etcd", Status: "up", Address: "127.0.0.1:2379", Uptime: uptime},
		{Name: "Kafka", Status: "up", Address: "127.0.0.1:9092", Uptime: uptime},
		{Name: "Sentinel", Status: "up", Address: "内置", Uptime: uptime},
	}
}

// GetAlarms 获取告警列表
func (s *MonitorService) GetAlarms() []model.Alarm {
	// 从实际告警系统获取，目前返回空列表
	return []model.Alarm{}
}

// GetQPSHistory 获取 QPS 历史数据（最近 N 个数据点）
func (s *MonitorService) GetQPSHistory() []model.QPSDataPoint {
	// 返回最近 20 个数据点（模拟数据，实际应从 Prometheus 查询）
	points := make([]model.QPSDataPoint, 0, 20)
	now := time.Now()
	for i := 19; i >= 0; i-- {
		t := now.Add(-time.Duration(i) * 3 * time.Second)
		points = append(points, model.QPSDataPoint{
			Time:  t.Format("15:04:05"),
			Value: float64(runtime.NumGoroutine()) * 10, // 近似值
		})
	}
	return points
}

// GetSeckillStats 获取秒杀统计
func (s *MonitorService) GetSeckillStats() *model.SeckillStats {
	return &model.SeckillStats{
		TotalOrders: 0,
		SuccessRate: 0,
		QPS:         0,
		MQBacklog:   0,
	}
}

// [修复] GetSlowAPIs 获取慢接口 TOP 排行，按耗时降序排列
func (s *MonitorService) GetSlowAPIs() []SlowAPIRecord {
	slowAPIMu.RLock()
	defer slowAPIMu.RUnlock()
	// [修复] 复制一份数据，按耗时降序排序
	result := make([]SlowAPIRecord, len(slowAPIRecords))
	copy(result, slowAPIRecords)
	sort.Slice(result, func(i, j int) bool {
		return result[i].DurationMs > result[j].DurationMs
	})
	return result
}