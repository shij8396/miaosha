package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/detector"
	"github.com/miaosha/service"
	"github.com/miaosha/utils"
	ws "github.com/miaosha/websocket"
)

type MonitorController struct {
	monitorService  *service.MonitorService
	anomalyDetector *detector.AnomalyDetector
	wsHub           *ws.Hub
}

func NewMonitorController(monitorService *service.MonitorService, ad *detector.AnomalyDetector, hub *ws.Hub) *MonitorController {
	return &MonitorController{
		monitorService:  monitorService,
		anomalyDetector: ad,
		wsHub:           hub,
	}
}

// GetMetrics 获取监控指标
func (ctl *MonitorController) GetMetrics(c *gin.Context) {
	metrics := ctl.monitorService.GetMetrics()
	utils.Success(c, metrics)
}

// GetQPS 获取 QPS 历史数据
func (ctl *MonitorController) GetQPS(c *gin.Context) {
	history := ctl.monitorService.GetQPSHistory()
	utils.Success(c, history)
}

// GetMiddlewareStatus 获取中间件状态
func (ctl *MonitorController) GetMiddlewareStatus(c *gin.Context) {
	status := ctl.monitorService.GetMiddlewareStatus()
	utils.Success(c, status)
}

// GetAlarms 获取告警列表
func (ctl *MonitorController) GetAlarms(c *gin.Context) {
	alarms := ctl.monitorService.GetAlarms()
	utils.Success(c, alarms)
}

// GetSeckillStats 获取秒杀统计
func (ctl *MonitorController) GetSeckillStats(c *gin.Context) {
	stats := ctl.monitorService.GetSeckillStats()
	utils.Success(c, stats)
}

// [修复] GetSlowAPIs 获取慢接口 TOP 排行 - GET /api/v1/monitor/slow-api
// 返回最近 50 条慢接口记录（耗时 > 500ms），按耗时降序排列
func (ctl *MonitorController) GetSlowAPIs(c *gin.Context) {
	slowAPIs := ctl.monitorService.GetSlowAPIs()
	utils.Success(c, slowAPIs)
}

// [创新] GetAnomalyStats 获取 AI 异常检测统计 - GET /api/v1/monitor/anomaly
// 返回当前监控用户数、全局统计基准等
func (ctl *MonitorController) GetAnomalyStats(c *gin.Context) {
	stats := gin.H{
		"profile_count":      ctl.anomalyDetector.ProfileCount(),
		"window_size":        20,
		"block_duration_min": 5,
		"description":        "AI实时异常检测引擎 — 滑动窗口 + Z-Score 统计模型",
	}
	utils.Success(c, stats)
}

// [创新] GetWSStats 获取 WebSocket 连接统计 - GET /api/v1/monitor/ws-stats
func (ctl *MonitorController) GetWSStats(c *gin.Context) {
	utils.Success(c, ctl.wsHub.Stats())
}

// [增强] GetPVUV 实时流量（PV/UV + 最近60秒每秒请求序列）- GET /api/v1/monitor/pvuv
func (ctl *MonitorController) GetPVUV(c *gin.Context) {
	data := ctl.monitorService.GetPVUV()
	utils.Success(c, data)
}

// [增强] GetHotProducts 热销商品排行 TOP N - GET /api/v1/monitor/hot-products?top=10
// 返回真实销售件数 + Redis/DB 实时库存
func (ctl *MonitorController) GetHotProducts(c *gin.Context) {
	topN := 10
	if v := c.Query("top"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			topN = n
		}
	}
	utils.Success(c, ctl.monitorService.GetHotProducts(topN))
}

// [增强] GetInventory 全量商品库存状态 - GET /api/v1/monitor/inventory
// 按剩余率升序返回，告急库存排前，供大屏库存监控面板
func (ctl *MonitorController) GetInventory(c *gin.Context) {
	utils.Success(c, ctl.monitorService.GetInventory())
}