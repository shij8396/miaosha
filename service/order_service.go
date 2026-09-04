package service

import (
	"context"
	"fmt"
	"time"

	"github.com/miaosha/config"
	"github.com/miaosha/dao"
	"github.com/miaosha/log"
	"github.com/miaosha/model"
	"github.com/miaosha/monitor"
	redisClient "github.com/miaosha/redis"
)

type OrderService struct{}

func NewOrderService() *OrderService { return &OrderService{} }

func (s *OrderService) GetUserOrders(userID int64, page, pageSize int, status string) ([]model.Order, int64, error) {
	cfg := config.GetConfig()
	// [修复] 支持按状态筛选订单，参数顺序：userID, page, pageSize, shardCount, status
	return dao.GetUserOrders(userID, page, pageSize, cfg.MySQL.OrderTableShardCount, status)
}

func (s *OrderService) GetOrderDetail(orderNo string, userID int64) (*model.Order, error) {
	cfg := config.GetConfig()
	return dao.GetOrderByOrderNo(orderNo, userID, cfg.MySQL.OrderTableShardCount)
}

func (s *OrderService) CreateOrder(msg map[string]interface{}) error {
	cfg := config.GetConfig()
	orderNo := msg["order_no"].(string)
	userID := int64(msg["user_id"].(float64))
	productID := int64(msg["product_id"].(float64))
	productName := msg["product_name"].(string)
	seckillPrice := msg["seckill_price"].(float64)
	quantity := int(msg["quantity"].(float64))
	totalAmount := msg["total_amount"].(float64)

	// [修复] 幂等性校验：检查订单是否已存在，防止重复消费
	existOrder, _ := dao.GetOrderByOrderNo(orderNo, userID, cfg.MySQL.OrderTableShardCount)
	if existOrder != nil {
		log.L().Warnw("订单已存在，跳过重复创建", "order_no", orderNo)
		return nil
	}

	order := &model.Order{
		OrderNo: orderNo, UserID: userID, ProductID: productID, ProductName: productName,
		SeckillPrice: seckillPrice, Quantity: quantity, TotalAmount: totalAmount, Status: model.OrderStatusPending,
	}
	if err := dao.CreateOrder(order, cfg.MySQL.OrderTableShardCount); err != nil {
		return fmt.Errorf("创建订单失败: %w", err)
	}
	if err := dao.DeductProductStock(productID, quantity); err != nil {
		return fmt.Errorf("扣减MySQL库存失败: %w", err)
	}
	// [修复] 订单创建成功后上报 Prometheus 指标
	monitor.IncOrderCreated()
	return nil
}

func (s *OrderService) CancelOrder(orderNo string, userID int64, reason string) error {
	cfg := config.GetConfig()
	// [修复] 先查询订单获取商品信息，用于回滚库存
	order, err := dao.GetOrderByOrderNo(orderNo, userID, cfg.MySQL.OrderTableShardCount)
	if err != nil {
		return fmt.Errorf("查询订单失败: %w", err)
	}
	if order == nil {
		return fmt.Errorf("订单不存在")
	}
	// [修复] 仅待支付状态的订单可以取消
	if order.Status != model.OrderStatusPending {
		return fmt.Errorf("订单状态不允许取消")
	}
	// [修复] 先更新订单状态为已取消
	if err := dao.CancelOrder(orderNo, userID, reason, cfg.MySQL.OrderTableShardCount); err != nil {
		return fmt.Errorf("取消订单失败: %w", err)
	}
	// [修复] 回滚 MySQL 库存
	if err := dao.RollbackProductStock(order.ProductID, order.Quantity); err != nil {
		log.L().Errorw("取消订单后回滚MySQL库存失败", "order_no", orderNo, "product_id", order.ProductID, "error", err)
	} else {
		log.L().Infow("取消订单后MySQL库存已回滚", "order_no", orderNo, "product_id", order.ProductID, "quantity", order.Quantity)
	}
	// [修复] 归还 Redis 库存
	ctx := context.Background()
	if err := redisClient.IncrStock(ctx, order.ProductID, order.Quantity); err != nil {
		log.L().Errorw("取消订单后回滚Redis库存失败", "order_no", orderNo, "product_id", order.ProductID, "error", err)
	} else {
		log.L().Infow("取消订单后Redis库存已归还", "order_no", orderNo, "product_id", order.ProductID, "quantity", order.Quantity)
	}
	// [修复] 按订单数量递减用户限购计数，允许用户重新参与秒杀
	// 原逻辑 RemoveUserPurchased 整键删除：限购>1且用户有多笔订单时，
	// 取消一单会清零全部已购计数，导致用户可购买超出限购总量
	if _, err := redisClient.DecrUserPurchaseCount(ctx, userID, order.ProductID, order.Quantity); err != nil {
		log.L().Warnw("取消订单后递减用户限购计数失败", "order_no", orderNo, "user_id", userID, "product_id", order.ProductID, "quantity", order.Quantity, "error", err)
	} else {
		log.L().Infow("取消订单后用户限购计数已递减", "order_no", orderNo, "user_id", userID, "product_id", order.ProductID, "quantity", order.Quantity)
	}
	return nil
}

func (s *OrderService) ProcessTimeoutOrder(msg map[string]interface{}) error {
	cfg := config.GetConfig()
	orderNo := msg["order_no"].(string)
	userID := int64(msg["user_id"].(float64))
	productID := int64(msg["product_id"].(float64))
	quantity := int(msg["quantity"].(float64))

	// [修复] 检查订单状态是否为待支付(0)，如果是则取消并回滚库存
	order, err := dao.GetOrderByOrderNo(orderNo, userID, cfg.MySQL.OrderTableShardCount)
	if err != nil {
		log.L().Errorw("查询超时订单失败", "order_no", orderNo, "error", err)
		return fmt.Errorf("查询订单失败: %w", err)
	}
	if order == nil {
		log.L().Warnw("超时订单不存在，可能已被删除", "order_no", orderNo)
		return nil
	}

	// [修复] 如果订单已支付，不处理
	if order.Status != model.OrderStatusPending {
		if order.Status == model.OrderStatusPaid {
			log.L().Infow("订单已支付，跳过超时取消", "order_no", orderNo, "status", order.Status)
		} else {
			log.L().Infow("订单非待支付状态，跳过超时取消", "order_no", orderNo, "status", order.Status)
		}
		return nil
	}

	// [修复] 更新订单状态为超时取消(4)，与用户手动取消(2)区分
	if err := dao.CancelOrderWithStatus(orderNo, userID, "超时未支付，系统自动取消", model.OrderStatusTimeout, cfg.MySQL.OrderTableShardCount); err != nil {
		log.L().Errorw("取消超时订单失败", "order_no", orderNo, "error", err)
		return fmt.Errorf("取消订单失败: %w", err)
	}
	log.L().Infow("超时订单已取消", "order_no", orderNo, "user_id", userID, "product_id", productID)

	// [修复] 回滚 MySQL 库存
	if err := dao.RollbackProductStock(productID, quantity); err != nil {
		log.L().Errorw("回滚MySQL库存失败", "order_no", orderNo, "product_id", productID, "quantity", quantity, "error", err)
		return fmt.Errorf("回滚库存失败: %w", err)
	}
	log.L().Infow("MySQL库存已回滚", "order_no", orderNo, "product_id", productID, "quantity", quantity)

	// [修复] 归还 Redis 库存
	ctx := context.Background()
	if err := redisClient.IncrStock(ctx, productID, quantity); err != nil {
		log.L().Errorw("回滚Redis库存失败", "order_no", orderNo, "product_id", productID, "error", err)
	} else {
		log.L().Infow("Redis库存已归还", "order_no", orderNo, "product_id", productID, "quantity", quantity)
	}

	// [修复] 按订单数量递减用户限购计数（原逻辑整键删除，限购>1多订单场景会清零全部计数）
	if _, err := redisClient.DecrUserPurchaseCount(ctx, userID, productID, quantity); err != nil {
		log.L().Warnw("递减用户限购计数失败", "order_no", orderNo, "user_id", userID, "product_id", productID, "quantity", quantity, "error", err)
	} else {
		log.L().Infow("用户限购计数已递减", "order_no", orderNo, "user_id", userID, "product_id", productID, "quantity", quantity)
	}

	// [修复] 上报超时订单计数
	monitor.IncOrderTimeout()

	return nil
}

// GetAllOrders 管理员查询所有订单（跨分表）
func (s *OrderService) GetAllOrders(page, pageSize int, status *int8, orderNo string, userID *int64) ([]model.Order, int64, error) {
	return dao.GetAllOrders(page, pageSize, status, orderNo, userID)
}

// GetReconDiffList 获取对账差异列表
func (s *OrderService) GetReconDiffList(page, pageSize int) ([]model.ReconDiff, int64, error) {
	return dao.GetReconDiffList(page, pageSize)
}

// FixReconDiff 手动修复对账差异
func (s *OrderService) FixReconDiff(id int64) error {
	// 标记为已修正
	return dao.UpdateReconDiffCorrected(id)
}

// [修复] BatchImportOrders 批量导入订单
func (s *OrderService) BatchImportOrders(orders []map[string]interface{}) (int, int) {
	var successCount, failCount int
	for _, msg := range orders {
		if err := s.CreateOrder(msg); err != nil {
			failCount++
			continue
		}
		successCount++
	}
	return successCount, failCount
}

// [修复] PayCallback 支付回调：将订单状态从待支付(0)更新为已支付(1)
// 支付成功后清除用户购买记录，允许用户重新参与限购
func (s *OrderService) PayCallback(orderNo string, userID int64, payTime time.Time) error {
	cfg := config.GetConfig()
	order, err := dao.GetOrderByOrderNo(orderNo, userID, cfg.MySQL.OrderTableShardCount)
	if err != nil {
		return fmt.Errorf("查询订单失败: %w", err)
	}
	if order == nil {
		return fmt.Errorf("订单不存在")
	}
	if order.Status != model.OrderStatusPending {
		return fmt.Errorf("订单状态不允许支付（当前状态: %d）", order.Status)
	}
	// 更新订单状态为已支付
	if err := dao.UpdateOrderStatusWithPayTime(orderNo, userID, model.OrderStatusPaid, &payTime, cfg.MySQL.OrderTableShardCount); err != nil {
		return fmt.Errorf("更新订单状态失败: %w", err)
	}
	// [修复] 订单支付成功：保留用户限购计数，支付行为计入限购总额
	// 原逻辑支付成功后清除限购记录（RemoveUserPurchased），导致用户可以
	// "秒杀→支付→限购计数清零→再次秒杀" 无限循环购买，限购完全失效。
	// 限购含义是"每人最多购买X件"，已支付的订单是真实成交，必须占用限购名额；
	// 只有取消订单/超时未支付/退款（未实际持有商品）时才递减限购计数。
	log.L().Infow("订单支付成功", "order_no", orderNo, "user_id", userID, "amount", order.TotalAmount)
	return nil
}

// [修复] Refund 退款：将订单状态从已支付(1)更新为已退款(3)
// 退款后归还 MySQL 库存和 Redis 库存，清除用户限购记录
func (s *OrderService) Refund(orderNo string, userID int64, reason string) error {
	cfg := config.GetConfig()
	order, err := dao.GetOrderByOrderNo(orderNo, userID, cfg.MySQL.OrderTableShardCount)
	if err != nil {
		return fmt.Errorf("查询订单失败: %w", err)
	}
	if order == nil {
		return fmt.Errorf("订单不存在")
	}
	if order.Status != model.OrderStatusPaid {
		return fmt.Errorf("订单状态不允许退款（当前状态: %d）", order.Status)
	}
	// 更新订单状态为已退款
	if err := dao.UpdateOrderStatusWithPayTime(orderNo, userID, model.OrderStatusRefunded, nil, cfg.MySQL.OrderTableShardCount); err != nil {
		return fmt.Errorf("更新订单状态失败: %w", err)
	}
	// 归还 MySQL 库存
	if err := dao.RollbackProductStock(order.ProductID, order.Quantity); err != nil {
		log.L().Errorw("退款后回滚MySQL库存失败", "order_no", orderNo, "product_id", order.ProductID, "error", err)
	} else {
		log.L().Infow("退款后MySQL库存已归还", "order_no", orderNo, "product_id", order.ProductID, "quantity", order.Quantity)
	}
	// 归还 Redis 库存
	ctx := context.Background()
	if err := redisClient.IncrStock(ctx, order.ProductID, order.Quantity); err != nil {
		log.L().Errorw("退款后回滚Redis库存失败", "order_no", orderNo, "product_id", order.ProductID, "error", err)
	} else {
		log.L().Infow("退款后Redis库存已归还", "order_no", orderNo, "product_id", order.ProductID, "quantity", order.Quantity)
	}
	// [修复] 按订单数量递减用户限购记录，允许重新购买（原逻辑整键删除会清零全部已购计数）
	if _, err := redisClient.DecrUserPurchaseCount(ctx, userID, order.ProductID, order.Quantity); err != nil {
		log.L().Warnw("退款后递减用户限购记录失败", "order_no", orderNo, "user_id", userID, "product_id", order.ProductID, "quantity", order.Quantity, "error", err)
	} else {
		log.L().Infow("退款后用户限购记录已递减", "order_no", orderNo, "user_id", userID, "product_id", order.ProductID, "quantity", order.Quantity)
	}
	log.L().Infow("订单退款成功", "order_no", orderNo, "user_id", userID, "reason", reason)
	return nil
}
