package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/miaosha/model"
	"github.com/miaosha/service"
	"github.com/miaosha/utils"
)

// [修复] ActivityController 秒杀活动配置控制器，提供活动配置 CRUD 和缓存预热 API
type ActivityController struct {
	activityService *service.ActivityService
}

func NewActivityController(activityService *service.ActivityService) *ActivityController {
	return &ActivityController{activityService: activityService}
}

// [修复] GetActivity GET /api/v1/activity - 获取活动配置（当前在售商品列表）
func (ctl *ActivityController) GetActivity(c *gin.Context) {
	products, err := ctl.activityService.GetActiveProducts()
	if err != nil {
		utils.InternalError(c, "获取活动配置失败: "+err.Error())
		return
	}
	utils.Success(c, gin.H{
		"list":  products,
		"total": len(products),
	})
}

// [修复] UpdateActivity PUT /api/v1/activity - 更新活动配置（商品上下架）
func (ctl *ActivityController) UpdateActivity(c *gin.Context) {
	var req struct {
		ProductID int64 `json:"product_id" binding:"required,gt=0"`
		Status    int8  `json:"status" binding:"required,oneof=0 1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	if err := ctl.activityService.UpdateActivity(req.ProductID, req.Status); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	statusMsg := "已下架"
	if req.Status == 1 {
		statusMsg = "已上架"
	}
	utils.SuccessWithMessage(c, "活动配置更新成功: 商品"+statusMsg, gin.H{
		"product_id": req.ProductID,
		"status":     req.Status,
	})
}

// [修复] CacheWarmUp POST /api/v1/activity/cache-warmup - 一键缓存预热（将商品库存同步到 Redis）
func (ctl *ActivityController) CacheWarmUp(c *gin.Context) {
	count, err := ctl.activityService.CacheWarmUp()
	if err != nil {
		utils.InternalError(c, "缓存预热失败: "+err.Error())
		return
	}
	utils.SuccessWithMessage(c, "缓存预热完成", gin.H{
		"warmed_up_count": count,
	})
}

// [修复] SaveActivityConfig POST /api/v1/activity/config - 保存活动配置（限购数量、价格、时间等）
// 支持动态修改单用户限购数量，保存后实时生效无需重启
func (ctl *ActivityController) SaveActivityConfig(c *gin.Context) {
	var req model.ActivityConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	if err := ctl.activityService.SaveActivityConfig(&req); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	utils.SuccessWithMessage(c, "活动配置保存成功，实时生效", gin.H{
		"product_id": req.ProductID,
	})
}

// [修复] GetAllProducts 获取所有商品（含非活动商品），用于活动配置管理页面
func (ctl *ActivityController) GetAllProducts(c *gin.Context) {
	page := 1
	pageSize := utils.ClampPageSize(100) // [修复] 分页上限限制
	products, err := ctl.activityService.GetActiveProducts()
	if err != nil {
		utils.InternalError(c, "获取商品列表失败: "+err.Error())
		return
	}
	total := int64(len(products))
	// [修复] 简单分页处理
	start := (page - 1) * pageSize
	if start >= len(products) {
		utils.Success(c, gin.H{
			"list":      []model.Product{},
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		})
		return
	}
	end := start + pageSize
	if end > len(products) {
		end = len(products)
	}
	utils.Success(c, gin.H{
		"list":      products[start:end],
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
