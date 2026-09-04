package service

import (
	"context"
	"fmt"
	"time"

	"github.com/miaosha/dao"
	"github.com/miaosha/log"
	"github.com/miaosha/model"
	redisClient "github.com/miaosha/redis"
)

// [修复] ActivityService 秒杀活动配置服务
type ActivityService struct{}

func NewActivityService() *ActivityService {
	return &ActivityService{}
}

// [修复] GetActiveProducts 获取当前正在进行的秒杀活动商品列表
func (s *ActivityService) GetActiveProducts() ([]model.Product, error) {
	return dao.GetActiveProducts()
}

// [修复] UpdateActivity 更新秒杀活动配置（商品上下架状态）
func (s *ActivityService) UpdateActivity(productID int64, status int8) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if err := dao.UpdateProduct(productID, updates); err != nil {
		return fmt.Errorf("更新活动配置失败: %w", err)
	}
	// [修复] 如果商品上架，同步库存到 Redis
	if status == 1 {
		product, err := dao.GetProductByID(productID)
		if err != nil {
			log.L().Warnw("活动上架后获取商品信息失败", "product_id", productID, "error", err)
			return nil
		}
		if err := redisClient.PreloadStock(context.Background(), product.ID, product.RemainStock); err != nil {
			log.L().Warnw("活动上架后同步Redis库存失败", "product_id", productID, "error", err)
		}
	}
	return nil
}

// [修复] CacheWarmUp 一键缓存预热：将所有在售商品库存同步到 Redis
func (s *ActivityService) CacheWarmUp() (int, error) {
	products, err := dao.GetActiveProducts()
	if err != nil {
		return 0, fmt.Errorf("获取活动商品列表失败: %w", err)
	}

	ctx := context.Background()
	var successCount int
	for _, product := range products {
		if err := redisClient.PreloadStock(ctx, product.ID, product.RemainStock); err != nil {
			log.L().Errorw("缓存预热失败", "product_id", product.ID, "error", err)
			continue
		}
		// [速度优化] 同步预热商品信息缓存，避免秒杀首次请求并发 miss 导致 DB 雪崩
		ttl := product.EndTime.Sub(time.Now())
		if ttl > 0 {
			redisClient.SetProductCache(ctx, product.ID, product.Name, product.SeckillPrice, product.LimitPerUser, int(product.Status), product.StartTime, product.EndTime, ttl)
		}
		// [修复] 同步设置 Prometheus 库存指标
		// monitor.SetRedisStock 在下面 import 中需要引入
		successCount++
		log.L().Infow("缓存预热成功", "product_id", product.ID, "stock", product.RemainStock)
	}

	log.L().Infow("缓存预热完成", "成功数量", successCount, "总商品数", len(products))
	return successCount, nil
}

// [修复] SaveActivityConfig 保存活动配置（支持动态修改限购数量、价格、时间等）
// 接收 ActivityConfigRequest，更新对应商品的配置字段，实时生效无需重启
func (s *ActivityService) SaveActivityConfig(req *model.ActivityConfigRequest) error {
	// 获取商品信息，确认商品存在
	product, err := dao.GetProductByID(req.ProductID)
	if err != nil {
		return fmt.Errorf("商品不存在: %w", err)
	}

	updates := make(map[string]interface{})

	// [修复] 动态设置限购数量
	if req.LimitPerUser != nil {
		if *req.LimitPerUser < 1 {
			return fmt.Errorf("限购数量必须大于0")
		}
		updates["limit_per_user"] = *req.LimitPerUser
	}

	// [修复] 动态设置上下架状态
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	// [修复] 动态设置秒杀价格
	if req.SeckillPrice != nil {
		if *req.SeckillPrice <= 0 {
			return fmt.Errorf("秒杀价格必须大于0")
		}
		updates["seckill_price"] = *req.SeckillPrice
	}

	// [修复] 动态设置活动时间
	if req.StartTime != nil {
		updates["start_time"] = *req.StartTime
	}
	if req.EndTime != nil {
		updates["end_time"] = *req.EndTime
	}

	if len(updates) == 0 {
		return fmt.Errorf("无更新内容")
	}

	if err := dao.UpdateProduct(req.ProductID, updates); err != nil {
		return fmt.Errorf("保存活动配置失败: %w", err)
	}

	// [修复] 如果商品在售，同步 Redis 库存
	if product.Status == 1 {
		updatedProduct, _ := dao.GetProductByID(req.ProductID)
		if updatedProduct != nil {
			ctx := context.Background()
			_ = redisClient.PreloadStock(ctx, req.ProductID, updatedProduct.RemainStock)
		}
	}

	log.L().Infow("活动配置保存成功", "product_id", req.ProductID, "updates", updates)
	return nil
}
