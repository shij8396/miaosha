package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/config"
	"github.com/miaosha/dao"
	"github.com/miaosha/detector"
	"github.com/miaosha/log"
	"github.com/miaosha/model"
	"github.com/miaosha/monitor"
	"github.com/miaosha/mq"
	redisClient "github.com/miaosha/redis"
	sentinelClient "github.com/miaosha/sentinel"
	"github.com/miaosha/service"
	"github.com/miaosha/singleflight"
	"github.com/miaosha/utils"
	ws "github.com/miaosha/websocket"
)

type SeckillController struct {
	seckillService  *service.SeckillService
	anomalyDetector *detector.AnomalyDetector
	mergeGroup      *singleflight.ShardedGroup
	wsHub           *ws.Hub
}

func NewSeckillController(seckillService *service.SeckillService, ad *detector.AnomalyDetector, mg *singleflight.ShardedGroup, hub *ws.Hub) *SeckillController {
	return &SeckillController{
		seckillService:  seckillService,
		anomalyDetector: ad,
		mergeGroup:      mg,
		wsHub:           hub,
	}
}

// Seckill 秒杀下单
// @Summary      秒杀下单
// @Description  参与秒杀活动，经过 Sentinel 限流、热点参数防护、Redis 库存扣减、MQ 异步下单，管理员账号自动跳过限流
// @Tags         秒杀模块
// @Accept       json
// @Produce      json
// @Param        request body model.SeckillRequest true "秒杀请求（idempotent_key 为客户端生成的幂等 Key，防止重复提交）"
// @Security     BearerAuth
// @Success      200  {object}  utils.Response{data=model.SeckillResponse}  "秒杀成功"
// @Failure      400  {object}  utils.Response  "库存不足/商品已下架/不在活动时间"
// @Failure      429  {object}  utils.Response  "当前抢购人数过多，请稍后重试"
// @Failure      503  {object}  utils.Response  "Redis/MQ服务异常"
// @Router       /api/v1/seckill [post]
func (ctl *SeckillController) Seckill(c *gin.Context) {
	startTime := time.Now()

	// [修复] 管理员白名单：超级管理员和管理员账号跳过 Sentinel 限流
	userRole, _ := c.Get("role")
	isAdmin := userRole == "admin" || userRole == "super_admin"
	// [修复] 兜底：若 role 为空（旧版Token），通过数据库查询确认用户角色
	if !isAdmin {
		userID, exists := c.Get("user_id")
		if exists {
			uid := userID.(int64)
			// [修复] 查询数据库获取用户真实角色，兼容旧版Token
			user, err := dao.GetUserByID(uid)
			if err == nil && (user.Role == "admin" || user.Role == "super_admin") {
				isAdmin = true
			}
		}
	}

	// [修复] 非管理员需要经过 Sentinel 限流检查
	if !isAdmin {
		entry, err := sentinelClient.Entry("seckill_api")
		if err != nil {
			monitor.IncSentinelReject()
			log.L().Warnw("Sentinel限流拒绝", "trace_id", utils.GetTraceID(c))
			utils.Error(c, 429, "当前抢购人数过多，请稍后重试") // [修复] 分层提示：Sentinel限流
			return
		}
		if entry != nil {
			defer entry.Exit()
		}
	}

	var req model.SeckillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "请先登录")
		return
	}
	uid := userID.(int64)

	// [修复] 非管理员需要经过热点参数限流
	if !isAdmin {
		hotEntry, hotErr := sentinelClient.EntryWithArgs("seckill_product", req.ProductID)
		if hotErr != nil {
			monitor.IncSentinelReject()
			log.L().Warnw("Sentinel热点参数限流", "user_id", uid, "product_id", req.ProductID)
			utils.Error(c, 429, "当前抢购人数过多，请稍后重试") // [修复] 分层提示：热点参数限流
			return
		}
		if hotEntry != nil {
			defer hotEntry.Exit()
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	// [修复] 从请求上下文中提取 trace_id，传递给 service 层，确保链路追踪不中断
	traceID := utils.GetTraceID(c)

	// [创新] AI 实时异常检测：分析用户行为模式，识别黄牛/脚本/异常流量
	// 仅对非管理员用户进行检测，管理员不受影响
	if !isAdmin {
		anomalyResult := ctl.anomalyDetector.Record(uid, false, c.ClientIP(), time.Now())
		if anomalyResult.IsAnomaly && anomalyResult.Score >= 70 {
			log.L().Warnw("AI异常检测触发封禁",
				"trace_id", traceID,
				"user_id", uid,
				"score", anomalyResult.Score,
				"reasons", anomalyResult.Reasons,
				"ip", c.ClientIP(),
			)
			utils.Error(c, 429, fmt.Sprintf("行为异常，已被AI系统暂时限制 (%.0f分)", anomalyResult.Score))
			return
		}
	}

	// [创新] 请求合并：高并发下相同商品请求合并为一次 Redis 操作，结果广播所有等待者
	// 合并 Key = 商品ID + 用户ID，合并窗口 50ms
	mergeKey := fmt.Sprintf("seckill:%d:%d", uid, req.ProductID)
	mergeResult, mergeErr, waiterCount := ctl.mergeGroup.Do(mergeKey, func() (interface{}, error) {
		return ctl.seckillService.ExecuteSeckill(ctx, uid, req.ProductID, req.Quantity, req.IdempotentKey, c.ClientIP(), traceID)
	}, 50*time.Millisecond)

	if waiterCount > 1 {
		log.L().Infow("请求合并生效",
			"trace_id", traceID,
			"user_id", uid,
			"product_id", req.ProductID,
			"merged_count", waiterCount,
		)
	}

	var resp *model.SeckillResponse
	if mergeErr == nil {
		resp = mergeResult.(*model.SeckillResponse)
	}

	if mergeErr != nil {
		log.L().Warnw("秒杀失败", "trace_id", traceID, "user_id", uid, "error", mergeErr.Error())
		// [修复] 使用 errors.Is() 精确匹配业务错误码，替代脆弱的字符串匹配
		errMsg := mergeErr.Error()
		switch {
		case errors.Is(mergeErr, utils.ErrStockInsufficient):
			utils.Error(c, 400, "库存不足，秒杀失败")
		case errors.Is(mergeErr, utils.ErrRedisDown):
			utils.Error(c, 503, "Redis服务异常，请稍后重试")
		case errors.Is(mergeErr, utils.ErrMQDown):
			utils.Error(c, 503, "消息队列异常，请稍后重试")
		case errors.Is(mergeErr, utils.ErrProductOffline):
			utils.Error(c, 400, "商品已下架")
		case errors.Is(mergeErr, utils.ErrNotInSeckillTime):
			utils.Error(c, 400, "不在秒杀活动时间内")
		case errors.Is(mergeErr, utils.ErrAlreadyPurchased):
			utils.Error(c, 400, errMsg)
		case errors.Is(mergeErr, utils.ErrExceedLimit):
			utils.Error(c, 400, errMsg)
		case errors.Is(mergeErr, utils.ErrDuplicateSubmit):
			utils.Error(c, 400, errMsg)
		case errors.Is(mergeErr, utils.ErrSeckillOverloaded):
			utils.Error(c, 429, "当前抢购人数过多，请稍后重试")
		default:
			utils.Error(c, 500, errMsg)
		}
		return
	}

	// [创新] 秒杀成功后：记录成功行为到 AI 检测器 + WebSocket 实时推送结果
	if !isAdmin {
		ctl.anomalyDetector.Record(uid, true, c.ClientIP(), time.Now())
	}
	// WebSocket 推送秒杀结果
	ctl.wsHub.PushToUser(uid, ws.MsgSeckillResult, gin.H{
		"order_no":   resp.OrderNo,
		"product_id": req.ProductID,
		"status":     "success",
		"message":    "秒杀成功，订单已创建",
	})

	roleLabel := ""
	if isAdmin {
		roleLabel = "管理员"
	}
	log.L().Infow(roleLabel+"秒杀排队成功", "trace_id", traceID, "user_id", uid, "product_id", req.ProductID, "order_no", resp.OrderNo, "cost_ms", time.Since(startTime).Milliseconds())
	utils.Success(c, resp)
}

// Health 健康检查端点
// GET /health - Liveness Probe：仅检查进程是否存活，K8s 用于判断是否需要重启 Pod
// GET /health?type=readiness - Readiness Probe：检查所有依赖中间件连通性，K8s 用于判断是否将流量路由到该 Pod
func Health(c *gin.Context) {
	checkType := c.DefaultQuery("type", "liveness")

	// Liveness：仅检查进程存活，不检查依赖，避免级联重启
	if checkType == "liveness" {
		utils.Success(c, gin.H{
			"status":   "ok",
			"type":     "liveness",
			"time":     time.Now().Format("2006-01-02 15:04:05"),
			"instance": config.GetConfig().Server.InstanceID,
		})
		return
	}

	// Readiness：检查所有依赖中间件连通性
	deps := make(map[string]string)
	healthy := true

	// 检查 MySQL
	if err := dao.Ping(); err != nil {
		deps["mysql"] = "down: " + err.Error()
		healthy = false
	} else {
		deps["mysql"] = "up"
	}

	// 检查 Redis
	if err := redisClient.GetClient().Ping(context.Background()).Err(); err != nil {
		deps["redis"] = "down: " + err.Error()
		healthy = false
	} else {
		deps["redis"] = "up"
	}

	// 检查 RabbitMQ（通过 mq 包的连接状态）
	if err := mq.Ping(); err != nil {
		deps["rabbitmq"] = "down: " + err.Error()
		healthy = false
	} else {
		deps["rabbitmq"] = "up"
	}

	status := "ok"
	httpStatus := 200
	if !healthy {
		status = "degraded"
		httpStatus = 503
	}

	c.JSON(httpStatus, gin.H{
		"status":   status,
		"type":     "readiness",
		"time":     time.Now().Format("2006-01-02 15:04:05"),
		"instance": config.GetConfig().Server.InstanceID,
		"deps":     deps,
	})
}