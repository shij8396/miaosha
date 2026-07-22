package controller

import (
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