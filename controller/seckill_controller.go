package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// [创新] 秒杀地址隐藏：校验动态 Path Token，防止脚本提前构造请求
	// 借鉴 qiurunze123/miaosha（19k Stars）
	if !isAdmin {
		valid, err := redisClient.GetAndVerifySeckillPath(ctx, uid, req.ProductID, req.PathToken)
		if err != nil || !valid {
			log.L().Warnw("秒杀路径校验失败", "user_id", uid, "product_id", req.ProductID)
			utils.Error(c, 400, "秒杀地址已失效，请刷新页面重试")
			return
		}
	}

	// [创新] 数学验证码：后端校验算式答案，防止脚本自动化攻击
	// 借鉴 qiurunze123/miaosha（19k Stars）
	// [修复] 验证码改为强制校验：原逻辑 req.CaptchaCode > 0 时才校验，
	// 恶意脚本不传 captcha 字段即可绕过验证直接秒杀，属于逻辑漏洞
	if !isAdmin {
		if req.CaptchaID == "" || req.CaptchaCode == 0 {
			log.L().Warnw("缺少验证码参数被拒绝", "user_id", uid, "product_id", req.ProductID)
			utils.Error(c, 400, "请完成数学验证码后再参与秒杀")
			return
		}
		valid, err := redisClient.GetAndVerifyCaptcha(ctx, req.CaptchaID, req.CaptchaCode)
		if err != nil || !valid {
			log.L().Warnw("数学验证码校验失败", "user_id", uid, "product_id", req.ProductID)
			utils.Error(c, 400, "验证码错误或已过期，请刷新后重试")
			return
		}
	}

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
		return ctl.seckillService.ExecuteSeckill(ctx, uid, req.ProductID, req.Quantity, req.SKUID, req.IdempotentKey, c.ClientIP(), traceID)
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

// GetPurchasedCounts 获取用户各商品已购数量
// @Summary      获取用户已购数量
// @Description  查询当前用户在指定商品上的已购数量（Redis限购计数），秒杀首页用于恢复"已抢购"按钮状态
// @Tags         秒杀模块
// @Produce      json
// @Param        product_ids query string true "商品ID列表，逗号分隔（如 18,19,20）"
// @Security     BearerAuth
// @Success      200  {object}  utils.Response{data=map[string]int}  "商品ID→已购数量"
// @Router       /api/v1/seckill/purchased [get]
func (ctl *SeckillController) GetPurchasedCounts(c *gin.Context) {
	uid := c.GetInt64("user_id")

	// [修复] 解析 product_ids 查询参数（逗号分隔的商品ID列表）
	productIDs := make([]int64, 0)
	if ids := c.Query("product_ids"); ids != "" {
		for _, s := range strings.Split(ids, ",") {
			s = strings.TrimSpace(s)
			if s == "" { continue }
			id, err := strconv.ParseInt(s, 10, 64)
			if err != nil || id <= 0 {
				utils.BadRequest(c, "商品ID格式错误")
				return
			}
			productIDs = append(productIDs, id)
		}
	}
	if len(productIDs) == 0 {
		utils.BadRequest(c, "product_ids 参数不能为空")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	// [修复] 逐商品读取 Redis 限购计数，构造 商品ID→已购数量 映射
	result := make(map[string]int, len(productIDs))
	for _, pid := range productIDs {
		count, err := redisClient.GetUserPurchaseCount(ctx, uid, pid)
		if err != nil {
			// 单个商品读取失败不阻塞整体，计为 0（下次秒杀时 Lua 会再次强校验）
			log.L().Warnw("读取用户已购数量失败", "user_id", uid, "product_id", pid, "error", err)
			count = 0
		}
		result[strconv.FormatInt(pid, 10)] = count
	}
	utils.Success(c, result)
}

// GetSeckillPath 获取秒杀隐藏路径 Token
// @Summary      获取秒杀隐藏路径
// @Description  秒杀前获取动态 Path Token，防止脚本提前构造请求（借鉴 qiurunze123/miaosha）
// @Tags         秒杀模块
// @Produce      json
// @Param        product_id query int64 true "商品ID"
// @Security     BearerAuth
// @Success      200  {object}  utils.Response{data=model.SeckillPathResponse}  "获取成功"
// @Router       /api/v1/seckill/path [get]
func (ctl *SeckillController) GetSeckillPath(c *gin.Context) {
	productID, err := parseProductID(c)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	uid := c.GetInt64("user_id")

	// 生成随机 Path Token（32位十六进制）
	b := make([]byte, 16)
	rand.Read(b)
	token := hex.EncodeToString(b)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	ttl := 60 * time.Second // 60秒有效期
	if err := redisClient.SetSeckillPath(ctx, uid, productID, token, ttl); err != nil {
		log.L().Errorw("设置秒杀路径Token失败", "error", err, "user_id", uid, "product_id", productID)
		utils.Error(c, 500, "系统繁忙，请稍后重试")
		return
	}

	utils.Success(c, &model.SeckillPathResponse{
		PathToken: token,
		ExpireSec: 60,
	})
}

// GetCaptcha 获取数学验证码
// @Summary      获取数学验证码
// @Description  生成随机数学算式，后端存储答案，用户计算后提交（借鉴 qiurunze123/miaosha）
// @Tags         秒杀模块
// @Produce      json
// @Param        product_id query int64 true "商品ID"
// @Security     BearerAuth
// @Success      200  {object}  utils.Response{data=model.CaptchaResponse}  "获取成功"
// @Router       /api/v1/seckill/captcha [get]
func (ctl *SeckillController) GetCaptcha(c *gin.Context) {
	productID, err := parseProductID(c)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	uid := c.GetInt64("user_id")
	_ = uid // 预留用于日志追踪
	_ = productID // 预留用于扩展

	// 生成随机数学算式：num1 + op1 + num2 + op2 + num3
	// 借鉴 qiurunze123/miaosha 的验证码设计
	expr, answer := generateMathExpression()

	// 生成唯一 CaptchaID
	b := make([]byte, 8)
	rand.Read(b)
	captchaID := hex.EncodeToString(b)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	ttl := 120 * time.Second // 2分钟有效期
	if err := redisClient.SetCaptcha(ctx, captchaID, answer, ttl); err != nil {
		log.L().Errorw("设置验证码失败", "error", err, "user_id", uid)
		utils.Error(c, 500, "系统繁忙，请稍后重试")
		return
	}

	utils.Success(c, &model.CaptchaResponse{
		Expression: expr,
		ExpireSec:  120,
		CaptchaID:  captchaID,
	})
}

// parseProductID 从 query 参数中解析 product_id
func parseProductID(c *gin.Context) (int64, error) {
	pid := c.Query("product_id")
	if pid == "" {
		return 0, fmt.Errorf("缺少product_id参数")
	}
	var productID int64
	if _, err := fmt.Sscanf(pid, "%d", &productID); err != nil || productID <= 0 {
		return 0, fmt.Errorf("product_id参数无效")
	}
	return productID, nil
}

// generateMathExpression 生成随机数学算式，返回 (表达式字符串, 正确答案)
// 表达式格式: num1 op num2，其中 op ∈ {+, -}，简单加减题
// 借鉴 qiurunze123/miaosha 的验证码算法
func generateMathExpression() (string, int) {
	ops := []byte{'+', '-'}
	b := make([]byte, 1)
	rand.Read(b)
	n1 := int(b[0]%9) + 1  // 1~9
	rand.Read(b)
	n2 := int(b[0]%9) + 1  // 1~9
	rand.Read(b)
	op := int(b[0]) % 2

	// 减法确保结果非负
	if ops[op] == '-' && n2 > n1 {
		n1, n2 = n2, n1
	}

	expr := fmt.Sprintf("%d%c%d", n1, ops[op], n2)

	answer := 0
	switch ops[op] {
	case '+':
		answer = n1 + n2
	case '-':
		answer = n1 - n2
	}
	return expr, answer
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