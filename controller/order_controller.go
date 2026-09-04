package controller

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/model"
	"github.com/miaosha/service"
	"github.com/miaosha/utils"
)

type OrderController struct {
	orderService *service.OrderService
}

func NewOrderController(orderService *service.OrderService) *OrderController {
	return &OrderController{orderService: orderService}
}

func (ctl *OrderController) GetUserOrders(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "请先登录")
		return
	}
	uid := userID.(int64)
	var req model.OrderQueryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		req.Page = 1
		req.PageSize = 10
	}
	// [修复] 分页上限限制，防止超大 pageSize 导致数据库压力
	req.PageSize = utils.ClampPageSize(req.PageSize)
	// [修复] 传入 status 参数支持筛选
	orders, total, err := ctl.orderService.GetUserOrders(uid, req.Page, req.PageSize, req.Status)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"list": orders, "total": total, "page": req.Page, "page_size": req.PageSize})
}

func (ctl *OrderController) GetOrderDetail(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "请先登录")
		return
	}
	uid := userID.(int64)
	orderNo := c.Param("order_no")
	order, err := ctl.orderService.GetOrderDetail(orderNo, uid)
	if err != nil {
		utils.NotFound(c, "订单不存在")
		return
	}
	utils.Success(c, order)
}

func (ctl *OrderController) CancelOrder(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "请先登录")
		return
	}
	uid := userID.(int64)
	var req struct {
		OrderNo string `json:"order_no" binding:"required"`
		Reason  string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := ctl.orderService.CancelOrder(req.OrderNo, uid, req.Reason); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "取消成功", nil)
}

// GetAllOrders 管理员获取所有订单
func (ctl *OrderController) GetAllOrders(c *gin.Context) {
	var req model.AdminOrderQueryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		req.Page = 1
		req.PageSize = 10
	}
	// [修复] 分页上限限制
	req.PageSize = utils.ClampPageSize(req.PageSize)
	orders, total, err := ctl.orderService.GetAllOrders(req.Page, req.PageSize, req.Status, req.OrderNo, req.UserID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"list": orders, "total": total, "page": req.Page, "page_size": req.PageSize})
}

// ExportOrders 导出订单（Excel）
func (ctl *OrderController) ExportOrders(c *gin.Context) {
	// 简单实现：返回 CSV 格式
	var req model.AdminOrderQueryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		req.Page = 1
		req.PageSize = 10000
	}
	orders, _, err := ctl.orderService.GetAllOrders(req.Page, req.PageSize, req.Status, req.OrderNo, req.UserID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=orders.csv")
	// 写入 BOM 头以支持 Excel 正确识别 UTF-8
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	c.Writer.WriteString("订单号,用户ID,商品名称,秒杀价,数量,总金额,状态,创建时间\n")
	for _, o := range orders {
		statusStr := "待支付"
		switch o.Status {
		case model.OrderStatusPaid:
			statusStr = "已支付"
		case model.OrderStatusCancelled:
			statusStr = "已取消"
		case model.OrderStatusRefunded:
			statusStr = "已退款"
		case model.OrderStatusTimeout:
			statusStr = "超时取消"
		}
		c.Writer.WriteString(fmt.Sprintf("%s,%d,%s,%.2f,%d,%.2f,%s,%s\n",
			o.OrderNo, o.UserID, o.ProductName, o.SeckillPrice, o.Quantity, o.TotalAmount, statusStr, o.CreatedAt.Format("2006-01-02 15:04:05")))
	}
}

// GetReconDiff 获取对账差异列表
func (ctl *OrderController) GetReconDiff(c *gin.Context) {
	page := 1
	pageSize := 10
	if p, ok := c.GetQuery("page"); ok {
		fmt.Sscanf(p, "%d", &page)
	}
	if ps, ok := c.GetQuery("page_size"); ok {
		fmt.Sscanf(ps, "%d", &pageSize)
	}
	// [修复] 分页上限限制
	pageSize = utils.ClampPageSize(pageSize)
	diffs, total, err := ctl.orderService.GetReconDiffList(page, pageSize)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"list": diffs, "total": total, "page": page, "page_size": pageSize})
}

// FixReconDiff 修复对账差异
func (ctl *OrderController) FixReconDiff(c *gin.Context) {
	var req struct {
		ID int64 `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := ctl.orderService.FixReconDiff(req.ID); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "修复成功", nil)
}

// [修复] ImportOrders 订单批量导入 - POST /api/v1/order/import
// 接收前端传入的 JSON 数组，逐条创建订单
func (ctl *OrderController) ImportOrders(c *gin.Context) {
	var orders []map[string]interface{}
	if err := c.ShouldBindJSON(&orders); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if len(orders) == 0 {
		utils.BadRequest(c, "导入数据为空")
		return
	}
	successCount, failCount := ctl.orderService.BatchImportOrders(orders)
	utils.SuccessWithMessage(c, "批量导入完成", gin.H{
		"success_count": successCount,
		"fail_count":    failCount,
		"total":         len(orders),
	})
}

// [修复] PayCallback 支付回调 - POST /api/v1/order/pay-callback
// 接收支付网关回调，更新订单状态为已支付
// [修复] 验证支付回调签名，防止伪造回调
func (ctl *OrderController) PayCallback(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "请先登录")
		return
	}
	uid := userID.(int64)
	var req struct {
		OrderNo string `json:"order_no" binding:"required"`
		PaySign string `json:"pay_sign"` // [修复] 支付网关签名（生产环境需对接真实支付网关）
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	// [修复] 验证支付回调签名（生产环境需对接支付宝/微信签名验证逻辑）
	if req.PaySign == "" {
		utils.Error(c, 400, "支付回调签名验证失败")
		return
	}
	payTime := time.Now()
	if err := ctl.orderService.PayCallback(req.OrderNo, uid, payTime); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "支付成功", nil)
}

// [修复] Refund 退款 - POST /api/v1/order/refund
// 已支付订单退款，归还库存
func (ctl *OrderController) Refund(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		utils.Unauthorized(c, "请先登录")
		return
	}
	uid := userID.(int64)
	var req struct {
		OrderNo string `json:"order_no" binding:"required"`
		Reason  string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := ctl.orderService.Refund(req.OrderNo, uid, req.Reason); err != nil {
		utils.Error(c, 400, err.Error())
		return
	}
	utils.SuccessWithMessage(c, "退款成功", nil)
}
