package monitor

import (
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// atomicFloat64 原子 float64（基于位运算实现，Go 标准库未直接提供）
type atomicFloat64 uint64

func (f *atomicFloat64) Add(v float64) {
	for {
		old := atomic.LoadUint64((*uint64)(f))
		newV := math.Float64bits(math.Float64frombits(old) + v)
		if atomic.CompareAndSwapUint64((*uint64)(f), old, newV) {
			return
		}
	}
}

func (f *atomicFloat64) Load() float64 {
	return math.Float64frombits(atomic.LoadUint64((*uint64)(f)))
}

// atomicInt64 原子 int64 封装（统一与 atomicFloat64 的使用方式）
type atomicInt64 int64

func (i *atomicInt64) Add(v int64) { atomic.AddInt64((*int64)(i), v) }
func (i *atomicInt64) Load() int64 { return atomic.LoadInt64((*int64)(i)) }

// ==================== [增强] 实时统计引擎 ====================
// 大屏数据增强核心：为数据大屏提供真实的第一方统计数据
// - PV/UV：按日累计，UV 按 用户ID/客户端IP 去重，跨天自动重置
// - QPS：每秒采样 + 60 秒滑动窗口，环形缓冲保留 120 个历史点
// - 热销排行：按商品累计秒杀成功次数与销售件数（内存计数，单机部署精确）
// - 告警缓冲：环形缓冲最近 100 条运营告警，同源告警 30 秒内去重防刷屏

// StatsAlarmItem 内存告警记录（供 /monitor/alarms 实时读取）
type StatsAlarmItem struct {
	Level   string    `json:"level"`   // warning / critical / info
	Message string    `json:"message"` // 告警内容
	Source  string    `json:"source"`  // 告警来源（组件名）
	Time    time.Time `json:"time"`
}

// StatsHotItem 热销商品实时统计
type StatsHotItem struct {
	ProductID    int64  `json:"product_id"`
	ProductName  string `json:"product_name"`
	SuccessCount int64  `json:"success_count"` // 秒杀成功次数
	SoldQuantity int64  `json:"sold_quantity"` // 累计销售件数
}

// PVUVSnapshot PV/UV 与流量快照
type PVUVSnapshot struct {
	PV          int64   `json:"pv"`            // 当日累计 PV
	UV          int64   `json:"uv"`            // 当日累计 UV
	QPS         float64 `json:"qps"`           // 当前 QPS（5 秒滑动平均）
	PVPerSecond []int64 `json:"pv_per_second"` // 最近 60 秒每秒请求数（图表用）
}

const (
	statsHistoryLen = 120 // QPS 历史环形缓冲长度（每秒 1 点，约 2 分钟）
	statsAlarmMax   = 100 // 告警环形缓冲容量
	statsAlarmDedup = 30 * time.Second
	statsUvMaxTrack = 500000 // UV 去重集合容量上限（防内存膨胀，超出后 UV 改为近似计数）
)

type statsEngine struct {
	mu sync.Mutex

	// PV/UV（按日）
	pv     int64 // atomic
	uv     int64 // atomic
	uvSet  map[string]struct{}
	uvDate string // 当前统计日期 YYYY-MM-DD，跨天重置

	// QPS：每秒请求计数，由 ticker 每秒滚入历史
	reqThisSecond   int64         // atomic
	reqTotal        int64         // atomic 累计请求数
	reqTotalMs      atomicFloat64 // 累计请求总耗时(ms)
	reqLatencyCount atomicInt64   // 有耗时的请求数
	qpsHistory      []float64
	qpsTimeRing     []string

	// 热销商品
	hot map[int64]*StatsHotItem

	// 告警
	alarms      []StatsAlarmItem
	alarmSeenAt map[string]time.Time // source+level+message → 上次告警时间
}

var engine = &statsEngine{
	uvSet:       make(map[string]struct{}),
	uvDate:      time.Now().Format("2006-01-02"),
	hot:         make(map[int64]*StatsHotItem),
	alarmSeenAt: make(map[string]time.Time),
}

// [增强] startStatsEngine 启动每秒采样定时器（由 Init 调用，只启动一次）
var statsOnce sync.Once

func startStatsEngine() {
	statsOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				tickStats()
			}
		}()
	})
}

// tickStats 每秒执行：滚动 QPS 桶 + 写入历史环形缓冲 + 跨天重置 PV/UV
func tickStats() {
	count := atomic.SwapInt64(&engine.reqThisSecond, 0)

	engine.mu.Lock()
	defer engine.mu.Unlock()

	// QPS 历史环形缓冲
	engine.qpsHistory = append(engine.qpsHistory, float64(count))
	engine.qpsTimeRing = append(engine.qpsTimeRing, time.Now().Format("15:04:05"))
	if len(engine.qpsHistory) > statsHistoryLen {
		engine.qpsHistory = engine.qpsHistory[len(engine.qpsHistory)-statsHistoryLen:]
		engine.qpsTimeRing = engine.qpsTimeRing[len(engine.qpsTimeRing)-statsHistoryLen:]
	}

	// 跨天重置 PV/UV（UV 集合重建，避免长期运行内存膨胀）
	if today := time.Now().Format("2006-01-02"); today != engine.uvDate {
		engine.uvDate = today
		atomic.StoreInt64(&engine.pv, 0)
		atomic.StoreInt64(&engine.uv, 0)
		engine.uvSet = make(map[string]struct{})
	}
}

// RecordVisit PV/UV 埋点：由 HTTP 指标中间件对每个业务请求调用
// visitorKey 优先取登录用户ID，未登录用客户端IP
func RecordVisit(visitorKey string) {
	if visitorKey == "" {
		return
	}
	atomic.AddInt64(&engine.pv, 1)

	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.uvSet != nil {
		if _, ok := engine.uvSet[visitorKey]; !ok {
			// 防御性上限：集合超限时不再精确去重，UV 计数继续累加（近似值）
			if len(engine.uvSet) < statsUvMaxTrack {
				engine.uvSet[visitorKey] = struct{}{}
			}
			atomic.AddInt64(&engine.uv, 1)
		}
	}
}

// RecordRequest QPS 埋点：每个 HTTP 请求 +1（与 PV 同点调用，独立计数便于扩展）
func RecordRequest() {
	atomic.AddInt64(&engine.reqThisSecond, 1)
	atomic.AddInt64(&engine.reqTotal, 1)
}

// RecordRequestLatency 接口耗时埋点：累计总耗时与请求数（计算平均响应时间）
func RecordRequestLatency(ms float64) {
	engine.reqTotalMs.Add(ms)
	engine.reqLatencyCount.Add(1)
}

// GetRequestStats 累计请求数、平均响应时间(ms)
func GetRequestStats() (total int64, avgMs float64) {
	total = atomic.LoadInt64(&engine.reqTotal)
	count := engine.reqLatencyCount.Load()
	if count == 0 {
		return total, 0
	}
	return total, engine.reqTotalMs.Load() / float64(count)
}

// GetQPS 当前 QPS（最近 5 秒滑动平均，避免单秒毛刺）
func GetQPS() float64 {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	n := len(engine.qpsHistory)
	if n == 0 {
		return 0
	}
	sum := 0.0
	for i := n - 5; i < n; i++ {
		if i >= 0 {
			sum += engine.qpsHistory[i]
		}
	}
	if n < 5 {
		return sum / float64(n)
	}
	return sum / 5.0
}

// GetQPSHistorySeconds 返回最近 seconds 秒的 QPS 时序（供图表渲染）
func GetQPSHistorySeconds(seconds int) ([]string, []float64) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	n := len(engine.qpsHistory)
	if seconds <= 0 || seconds > n {
		seconds = n
	}
	times := make([]string, 0, seconds)
	values := make([]float64, 0, seconds)
	for i := n - seconds; i < n; i++ {
		if i >= 0 {
			times = append(times, engine.qpsTimeRing[i])
			values = append(values, engine.qpsHistory[i])
		}
	}
	return times, values
}

// GetPVUV PV/UV 与流量快照（含最近 60 秒每秒请求数）
func GetPVUV() PVUVSnapshot {
	engine.mu.Lock()
	n := len(engine.qpsHistory)
	start := n - 60
	if start < 0 {
		start = 0
	}
	perSec := make([]int64, 0, n-start)
	for i := start; i < n; i++ {
		perSec = append(perSec, int64(engine.qpsHistory[i]))
	}
	engine.mu.Unlock()

	return PVUVSnapshot{
		PV:          atomic.LoadInt64(&engine.pv),
		UV:          atomic.LoadInt64(&engine.uv),
		QPS:         GetQPS(),
		PVPerSecond: perSec,
	}
}

// IncProductSales 热销商品埋点：秒杀成功后按商品累计成功次数与销售件数
func IncProductSales(productID int64, productName string, quantity int) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	item, ok := engine.hot[productID]
	if !ok {
		item = &StatsHotItem{ProductID: productID, ProductName: productName}
		engine.hot[productID] = item
	}
	if productName != "" && item.ProductName == "" {
		item.ProductName = productName
	}
	item.SuccessCount++
	item.SoldQuantity += int64(quantity)
}

// GetHotProducts 热销商品 TOP N（按销售件数降序）
func GetHotProducts(topN int) []StatsHotItem {
	engine.mu.Lock()
	items := make([]StatsHotItem, 0, len(engine.hot))
	for _, v := range engine.hot {
		items = append(items, *v)
	}
	engine.mu.Unlock()

	sort.Slice(items, func(i, j int) bool {
		return items[i].SoldQuantity > items[j].SoldQuantity
	})
	if topN > 0 && len(items) > topN {
		items = items[:topN]
	}
	return items
}

// RecordAlarm 告警埋点：同源同内容 30 秒内去重，防止刷屏
// level: warning / critical / info
func RecordAlarm(level, source, message string) {
	if message == "" {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()

	key := strings.TrimSpace(level) + "|" + strings.TrimSpace(source) + "|" + strings.TrimSpace(message)
	if last, ok := engine.alarmSeenAt[key]; ok && time.Since(last) < statsAlarmDedup {
		return // 30 秒内重复告警，丢弃
	}
	engine.alarmSeenAt[key] = time.Now()

	engine.alarms = append(engine.alarms, StatsAlarmItem{
		Level:   level,
		Message: message,
		Source:  source,
		Time:    time.Now(),
	})
	if len(engine.alarms) > statsAlarmMax {
		engine.alarms = engine.alarms[len(engine.alarms)-statsAlarmMax:]
	}
}

// GetStatsAlarms 读取告警缓冲（新的在前）
func GetStatsAlarms(limit int) []StatsAlarmItem {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	n := len(engine.alarms)
	if limit <= 0 || limit > n {
		limit = n
	}
	result := make([]StatsAlarmItem, 0, limit)
	for i := n - 1; i >= n-limit && i >= 0; i-- {
		result = append(result, engine.alarms[i])
	}
	return result
}
