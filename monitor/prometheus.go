package monitor

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/miaosha/config"
	"github.com/miaosha/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestTotal          = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "miaosha_http_requests_total", Help: "HTTP请求总数"}, []string{"method", "path"})
	httpRequestDuration       = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "miaosha_http_request_duration_seconds", Help: "HTTP请求耗时", Buckets: prometheus.DefBuckets}, []string{"method", "path"})
	httpErrorTotal            = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "miaosha_http_errors_total", Help: "HTTP错误总数"}, []string{"method", "path", "status_code"})
	sentinelRejectTotal       = prometheus.NewCounter(prometheus.CounterOpts{Name: "miaosha_sentinel_reject_total", Help: "Sentinel限流拒绝总数"})
	sentinelCircuitBreakTotal = prometheus.NewCounter(prometheus.CounterOpts{Name: "miaosha_sentinel_circuit_break_total", Help: "Sentinel熔断总数"})
	seckillSuccessTotal       = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "miaosha_seckill_success_total", Help: "秒杀成功总数"}, []string{"product_id"})
	seckillFailTotal          = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "miaosha_seckill_fail_total", Help: "秒杀失败总数"}, []string{"product_id", "reason"})
	redisStockGauge           = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "miaosha_redis_stock_remaining", Help: "Redis库存余量"}, []string{"product_id"})
	mqBacklogGauge            = prometheus.NewGauge(prometheus.GaugeOpts{Name: "miaosha_mq_backlog_count", Help: "MQ消息堆积量"})

	// [修复] 新增 Prometheus 指标：秒杀计数、订单创建/超时计数、内存快照
	seckillCounter    = prometheus.NewCounter(prometheus.CounterOpts{Name: "miaosha_seckill_total", Help: "秒杀请求总数（含成功/失败）"})
	orderCreatedTotal = prometheus.NewCounter(prometheus.CounterOpts{Name: "miaosha_order_created_total", Help: "订单创建总数"})
	orderTimeoutTotal = prometheus.NewCounter(prometheus.CounterOpts{Name: "miaosha_order_timeout_total", Help: "超时订单总数"})

	// [修复] 内存计数指标，用于 GetSeckillMetrics() 快照
	seckillSuccessCount int64
	seckillFailCount    int64
	orderCreatedCount   int64
	orderTimeoutCount   int64
	sentinelRejectCount int64 // [增强] 限流拒绝累计（大屏真实数据）

	// [修复] 缓存 metrics HTTP Server 实例，支持优雅关闭
	metricsServer *http.Server
)

func Init() {
	// [修复] 注册新增指标：seckillCounter、orderCreatedTotal、orderTimeoutTotal
	prometheus.MustRegister(httpRequestTotal, httpRequestDuration, httpErrorTotal, sentinelRejectTotal, sentinelCircuitBreakTotal, seckillSuccessTotal, seckillFailTotal, redisStockGauge, mqBacklogGauge, seckillCounter, orderCreatedTotal, orderTimeoutTotal)
	// [增强] 启动实时统计引擎（PV/UV、QPS 采样、热销、告警缓冲）
	startStatsEngine()
}

// [修复] StartMetricsServer 使用 http.Server 替代 http.ListenAndServe，捕获错误并支持优雅关闭
func StartMetricsServer(cfg *config.PrometheusConfig) {
	if !cfg.Enabled {
		return
	}
	mux := http.NewServeMux()
	mux.Handle(cfg.MetricsPath, promhttp.Handler())
	addr := fmt.Sprintf(":%d", cfg.MetricsPort)
	metricsServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	go func() {
		log.L().Infow("Prometheus指标服务启动", "addr", addr)
		// [修复] 捕获 ListenAndServe 返回值，非正常关闭时记录 FATAL 日志
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.L().Fatalw("Prometheus指标服务启动失败", "error", err)
		}
	}()
}

// [修复] ShutdownMetricsServer 优雅关闭 Prometheus 指标服务
func ShutdownMetricsServer() {
	if metricsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metricsServer.Shutdown(ctx); err != nil {
			log.L().Warnw("Prometheus指标服务关闭异常", "error", err)
		} else {
			log.L().Info("Prometheus指标服务已优雅关闭")
		}
	}
}

func IncHTTPRequestTotal(method, path string) { httpRequestTotal.WithLabelValues(method, path).Inc() }
func ObserveHTTPRequestDuration(method, path string, duration float64) {
	httpRequestDuration.WithLabelValues(method, path).Observe(duration)
}
func IncHTTPErrorTotal(method, path, statusCode string) {
	httpErrorTotal.WithLabelValues(method, path, statusCode).Inc()
}
func IncSentinelReject() {
	sentinelRejectTotal.Inc()
	atomic.AddInt64(&sentinelRejectCount, 1) // [增强] 内存计数（大屏真实数据）
}

// [增强] GetSentinelRejectTotal 限流拒绝累计数（内存快照）
func GetSentinelRejectTotal() int64 { return atomic.LoadInt64(&sentinelRejectCount) }
func IncSentinelCircuitBreak()      { sentinelCircuitBreakTotal.Inc() }
func IncSeckillSuccess(productID string) {
	seckillSuccessTotal.WithLabelValues(productID).Inc()
	seckillCounter.Inc()                     // [修复] 同时增加秒杀总计数
	atomic.AddInt64(&seckillSuccessCount, 1) // [修复] 内存计数
}

func IncSeckillFail(productID, reason string) {
	seckillFailTotal.WithLabelValues(productID, reason).Inc()
	seckillCounter.Inc()                  // [修复] 同时增加秒杀总计数（失败也计入总请求）
	atomic.AddInt64(&seckillFailCount, 1) // [修复] 内存计数
}

// [修复] IncOrderCreated 订单创建计数
func IncOrderCreated() {
	orderCreatedTotal.Inc()
	atomic.AddInt64(&orderCreatedCount, 1)
}

// [修复] IncOrderTimeout 超时订单计数
func IncOrderTimeout() {
	orderTimeoutTotal.Inc()
	atomic.AddInt64(&orderTimeoutCount, 1)
}

// [修复] GetSeckillMetrics 返回当前秒杀指标快照（从内存计数）
func GetSeckillMetrics() map[string]int64 {
	return map[string]int64{
		"seckill_success": atomic.LoadInt64(&seckillSuccessCount),
		"seckill_fail":    atomic.LoadInt64(&seckillFailCount),
		"order_created":   atomic.LoadInt64(&orderCreatedCount),
		"order_timeout":   atomic.LoadInt64(&orderTimeoutCount),
	}
}
func SetRedisStock(productID string, stock int) {
	redisStockGauge.WithLabelValues(productID).Set(float64(stock))
}
func SetMQBacklog(count int) { mqBacklogGauge.Set(float64(count)) }
