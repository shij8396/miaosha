package service

import (
	"context"
	"net"
	"net/url"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miaosha/config"
	"github.com/miaosha/dao"
	"github.com/miaosha/model"
	"github.com/miaosha/monitor"
	"github.com/miaosha/mq"
	redisClient "github.com/miaosha/redis"
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
	slowAPIMu         sync.RWMutex
	slowAPIRecords    []SlowAPIRecord
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
	// [增强] 慢接口同时写入告警缓冲（30 秒去重），大屏可见
	monitor.RecordAlarm("warning", "慢接口", path+" 耗时 "+strconv.FormatInt(durationMs, 10)+"ms")
}

type MonitorService struct {
	startTime time.Time
}

func NewMonitorService() *MonitorService {
	return &MonitorService{startTime: time.Now()}
}

// [增强] GetMetrics 监控指标（真实数据）
// - PassCount/AvgRt/QPS：来自实时统计引擎埋点
// - RejectCount：Sentinel 限流拒绝累计（内存计数）
// - MemUsage/ActiveConns：runtime 真实值
// - CPUUsage：进程级无第三方库无法精确获取，返回 -1 由前端展示为不可用
func (s *MonitorService) GetMetrics() *model.MonitorMetrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	total, avgMs := monitor.GetRequestStats()
	memUsage := 0.0
	if memStats.Sys > 0 {
		memUsage = float64(memStats.Alloc) / float64(memStats.Sys) * 100
	}
	return &model.MonitorMetrics{
		RejectCount: monitor.GetSentinelRejectTotal(),
		PassCount:   total,
		AvgRt:       avgMs,
		QPS:         monitor.GetQPS(),
		CPUUsage:    -1, // 进程级 CPU 未采集（需第三方库），前端显示为 --
		MemUsage:    memUsage,
		ActiveConns: int64(runtime.NumGoroutine()),
	}
}

// ==================== [增强] 中间件真实健康检查 ====================
// 原实现硬编码全部 "up"，属于假数据。现改为 TCP 拨号探测（1.5s 超时），
// 结果缓存 5 秒避免大屏每 5 秒轮询期间大量拨号。

type mwProbeTarget struct {
	name    string
	address string
}

var (
	mwStatusCache   []model.MiddlewareStatus
	mwStatusCacheAt time.Time
	mwStatusMu      sync.Mutex
)

// [增强] buildMWTargets 从配置构建探测目标（地址全部来自 yaml 配置，无硬编码）
func buildMWTargets() []mwProbeTarget {
	cfg := config.GetConfig()
	targets := []mwProbeTarget{}

	// MySQL 主库
	if cfg.MySQL.Master.Host != "" {
		targets = append(targets, mwProbeTarget{"MySQL", net.JoinHostPort(cfg.MySQL.Master.Host, strconv.Itoa(cfg.MySQL.Master.Port))})
	}
	// Redis（取第一个地址）
	if len(cfg.Redis.Addrs) > 0 {
		targets = append(targets, mwProbeTarget{"Redis Cluster", cfg.Redis.Addrs[0]})
	}
	// RabbitMQ（解析 amqp URL 提取 host:port）
	if len(cfg.RabbitMQ.URLs) > 0 {
		if host, port, err := parseAMQPAddress(cfg.RabbitMQ.URLs[0]); err == nil {
			targets = append(targets, mwProbeTarget{"RabbitMQ", net.JoinHostPort(host, port)})
		}
	}
	// Kafka（取第一个 broker）
	if len(cfg.Kafka.Brokers) > 0 {
		targets = append(targets, mwProbeTarget{"Kafka", cfg.Kafka.Brokers[0]})
	}
	// Etcd（取第一个 endpoint）
	if len(cfg.Etcd.Endpoints) > 0 {
		targets = append(targets, mwProbeTarget{"Etcd", cfg.Etcd.Endpoints[0]})
	}
	// Sentinel 为进程内置组件，进程存活即视为可用
	targets = append(targets, mwProbeTarget{"Sentinel", "内置"})
	return targets
}

// [增强] parseAMQPAddress 解析 amqp://user:pass@host:port/vhost 提取 host:port
func parseAMQPAddress(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	port := u.Port()
	if port == "" {
		port = "5672"
	}
	return u.Hostname(), port, nil
}

// [增强] probeMWStatus 并发 TCP 拨号探测（带超时与缓存）
func probeMWStatus() []model.MiddlewareStatus {
	uptime := time.Since(startTimeGlobal).String()
	targets := buildMWTargets()
	results := make([]model.MiddlewareStatus, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		results[i] = model.MiddlewareStatus{Name: t.name, Status: "down", Address: t.address, Uptime: uptime}
		if t.address == "内置" {
			results[i].Status = "up"
			continue
		}
		wg.Add(1)
		go func(idx int, addr string) {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", addr, 1500*time.Millisecond)
			if err == nil {
				results[idx].Status = "up"
				conn.Close()
			}
		}(i, t.address)
	}
	wg.Wait()
	return results
}

// startTimeGlobal 服务启动时间（进程级）
var startTimeGlobal = time.Now()

// GetMiddlewareStatus 获取中间件状态（真实 TCP 探测 + 5 秒缓存）
func (s *MonitorService) GetMiddlewareStatus() []model.MiddlewareStatus {
	mwStatusMu.Lock()
	defer mwStatusMu.Unlock()
	if time.Since(mwStatusCacheAt) < 5*time.Second && mwStatusCache != nil {
		return mwStatusCache
	}
	mwStatusCache = probeMWStatus()
	mwStatusCacheAt = time.Now()
	return mwStatusCache
}

// GetAlarms 获取告警列表（真实来源：实时告警缓冲）
// 覆盖：慢接口、限流触发、Redis/MQ 异常、订单超时等关键运营事件
func (s *MonitorService) GetAlarms() []model.Alarm {
	items := monitor.GetStatsAlarms(50)
	alarms := make([]model.Alarm, 0, len(items))
	for _, it := range items {
		alarms = append(alarms, model.Alarm{
			Level:     it.Level,
			Message:   it.Message,
			Source:    it.Source,
			CreatedAt: it.Time,
		})
	}
	// [增强] 中间件宕机动态生成 critical 告警
	for _, mw := range s.GetMiddlewareStatus() {
		if mw.Status != "up" {
			alarms = append(alarms, model.Alarm{
				Level:     "critical",
				Message:   mw.Name + " 连接失败（" + mw.Address + "）",
				Source:    "中间件探活",
				CreatedAt: time.Now(),
			})
		}
	}
	return alarms
}

// GetQPSHistory 获取 QPS 历史数据（真实时序：统计引擎每秒采样）
func (s *MonitorService) GetQPSHistory() []model.QPSDataPoint {
	times, values := monitor.GetQPSHistorySeconds(60)
	points := make([]model.QPSDataPoint, 0, len(times))
	for i := range times {
		points = append(points, model.QPSDataPoint{Time: times[i], Value: values[i]})
	}
	return points
}

// GetPVUV PV/UV 实时流量数据（真实埋点数据）
func (s *MonitorService) GetPVUV() map[string]interface{} {
	snap := monitor.GetPVUV()
	series := make([]model.PVUVData, 0, len(snap.PVPerSecond))
	// 补齐时间轴（与 QPS 历史对齐）
	times, _ := monitor.GetQPSHistorySeconds(len(snap.PVPerSecond))
	for i, v := range snap.PVPerSecond {
		t := ""
		if i < len(times) {
			t = times[i]
		}
		series = append(series, model.PVUVData{Time: t, PV: v})
	}
	return map[string]interface{}{
		"pv":     snap.PV,
		"uv":     snap.UV,
		"qps":    snap.QPS,
		"series": series,
	}
}

// GetSeckillStats 获取秒杀统计（真实数据：内存计数器 + 统计引擎 + MQ 统计）
// [修复] 原实现返回全 0 硬编码假数据，大屏核心指标全部失真
func (s *MonitorService) GetSeckillStats() *model.SeckillStats {
	metrics := monitor.GetSeckillMetrics()
	success, fail := metrics["seckill_success"], metrics["seckill_fail"]
	requests := success + fail

	successRate := 0.0
	if requests > 0 {
		successRate = float64(success) / float64(requests) * 100
	}

	pv, uv := 0, 0
	snap := monitor.GetPVUV()
	pv, uv = int(snap.PV), int(snap.UV)

	// 转化率 = 秒杀成功数 / UV（访问→成交的漏斗末端）
	conversion := 0.0
	if uv > 0 {
		conversion = float64(success) / float64(uv) * 100
	}

	mqStats := mq.GetMQStats()

	return &model.SeckillStats{
		TotalOrders:     metrics["order_created"],
		SuccessRate:     successRate,
		QPS:             monitor.GetQPS(),
		MQBacklog:       mqStats.Backlog,
		PV:              int64(pv),
		UV:              int64(uv),
		ConversionRate:  conversion,
		SeckillRequests: requests,
		SeckillSuccess:  success,
		SeckillFail:     fail,
		OrderTimeout:    metrics["order_timeout"],
		MQPublished:     mqStats.Published,
		MQConsumed:      mqStats.Consumed,
		MQConnected:     mqStats.Connected,
	}
}

// GetHotProducts 热销商品排行 TOP N（真实销售数据 + 实时库存）
// [修复] 原前端硬编码假排行，现从统计引擎读取真实销售数据
func (s *MonitorService) GetHotProducts(topN int) []model.HotProduct {
	hot := monitor.GetHotProducts(topN)

	// 商品基础信息（含无销量的商品，用于补全名称/价格/库存）
	products, err := dao.GetActiveProducts()
	productMap := make(map[int64]model.Product)
	if err == nil {
		for _, p := range products {
			productMap[p.ID] = p
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := make([]model.HotProduct, 0, len(hot))
	for _, h := range hot {
		item := model.HotProduct{
			ProductID:    h.ProductID,
			ProductName:  h.ProductName,
			SoldQuantity: h.SoldQuantity,
		}
		if p, ok := productMap[h.ProductID]; ok {
			item.ProductName = p.Name
			item.SeckillPrice = p.SeckillPrice
			item.DBRemainStock = p.RemainStock
			item.TotalStock = p.TotalStock
			item.Status = p.Status
			// Redis 实时库存（预热 key 不存在时按 DB 剩余展示）
			if stock, err := redisClient.GetStock(ctx, p.ID); err == nil {
				item.RedisStock = stock
			} else {
				item.RedisStock = p.RemainStock
			}
			if item.TotalStock > 0 {
				item.StockPercent = float64(item.RedisStock) / float64(item.TotalStock) * 100
			}
		}
		result = append(result, item)
	}
	return result
}

// GetInventory 全量商品库存状态（数据大屏库存监控面板）
// WarningLevel：soldout(0) / danger(<10%) / warning(<30%) / normal
func (s *MonitorService) GetInventory() []model.InventoryItem {
	products, err := dao.GetActiveProducts()
	if err != nil {
		return []model.InventoryItem{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	items := make([]model.InventoryItem, 0, len(products))
	for _, p := range products {
		redisStock := p.RemainStock
		if stock, err := redisClient.GetStock(ctx, p.ID); err == nil {
			redisStock = stock
		}
		percent := 100.0
		if p.TotalStock > 0 {
			percent = float64(redisStock) / float64(p.TotalStock) * 100
		}
		level := "normal"
		switch {
		case redisStock <= 0:
			level = "soldout"
		case percent < 10:
			level = "danger"
		case percent < 30:
			level = "warning"
		}
		items = append(items, model.InventoryItem{
			ProductID:     p.ID,
			ProductName:   p.Name,
			RedisStock:    redisStock,
			DBRemainStock: p.RemainStock,
			TotalStock:    p.TotalStock,
			StockPercent:  percent,
			WarningLevel:  level,
		})
	}
	// 按剩余率升序，告急库存排前
	sort.Slice(items, func(i, j int) bool {
		return items[i].StockPercent < items[j].StockPercent
	})
	return items
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
	// [修复] 只返回 TOP 20，避免响应过大
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

// ensure strings import used (parseAMQPAddress uses url; strings kept for future use)
var _ = strings.TrimSpace
