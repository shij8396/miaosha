package service

import (
	"context"
	"fmt"
	"time"

	"github.com/miaosha/dao"
	"github.com/miaosha/model"
	redisClient "github.com/miaosha/redis"
)

type ProductService struct{}

func NewProductService() *ProductService { return &ProductService{} }

func (s *ProductService) CreateProduct(req *model.CreateProductRequest) (*model.Product, error) {
	startTime, err := time.ParseInLocation("2006-01-02 15:04:05", req.StartTime, time.Local)
	if err != nil { return nil, fmt.Errorf("活动开始时间格式错误") }
	endTime, err := time.ParseInLocation("2006-01-02 15:04:05", req.EndTime, time.Local)
	if err != nil { return nil, fmt.Errorf("活动结束时间格式错误") }
	if endTime.Before(startTime) { return nil, fmt.Errorf("结束时间不能早于开始时间") }
	if req.LimitPerUser <= 0 { req.LimitPerUser = 1 }
	product := &model.Product{
		Name: req.Name, Description: req.Description, Price: req.Price, SeckillPrice: req.SeckillPrice,
		TotalStock: req.TotalStock, RemainStock: req.TotalStock, StartTime: startTime, EndTime: endTime,
		Status: 1, ImageURL: req.ImageURL, LimitPerUser: req.LimitPerUser,
	}
	if err := dao.CreateProduct(product); err != nil { return nil, fmt.Errorf("创建商品失败: %w", err) }
	// 同步库存到Redis缓存，保证秒杀时能正确扣减库存
	_ = redisClient.PreloadStock(context.Background(), product.ID, product.TotalStock)
	return product, nil
}

func (s *ProductService) UpdateProduct(id int64, req *model.UpdateProductRequest) error {
	// [修复] 先获取旧商品信息，避免误重置 remain_stock
	oldProduct, err := dao.GetProductByID(id)
	if err != nil { return fmt.Errorf("商品不存在: %w", err) }
	updates := make(map[string]interface{})
	if req.Name != nil { updates["name"] = *req.Name }
	if req.Description != nil { updates["description"] = *req.Description }
	if req.Price != nil { updates["price"] = *req.Price }
	if req.SeckillPrice != nil { updates["seckill_price"] = *req.SeckillPrice }
	if req.TotalStock != nil {
		updates["total_stock"] = *req.TotalStock
		// [修复] 仅当 total_stock 与旧值不同时才调整 remain_stock
		if *req.TotalStock != oldProduct.TotalStock {
			diff := *req.TotalStock - oldProduct.TotalStock
			if diff > 0 {
				// 增加库存：剩余库存同步增加
				updates["remain_stock"] = oldProduct.RemainStock + diff
			} else {
				// 减少库存：剩余库存保持当前值（不低于已售出数量）
				updates["remain_stock"] = oldProduct.RemainStock
			}
		}
	}
	if req.Status != nil { updates["status"] = *req.Status }
	if req.LimitPerUser != nil { updates["limit_per_user"] = *req.LimitPerUser }
	if req.ImageURL != nil { updates["image_url"] = *req.ImageURL }
	if req.StartTime != nil { t, err := time.ParseInLocation("2006-01-02 15:04:05", *req.StartTime, time.Local); if err != nil { return fmt.Errorf("开始时间格式错误") }; updates["start_time"] = t }
	if req.EndTime != nil { t, err := time.ParseInLocation("2006-01-02 15:04:05", *req.EndTime, time.Local); if err != nil { return fmt.Errorf("结束时间格式错误") }; updates["end_time"] = t }
	if len(updates) == 0 { return fmt.Errorf("无更新内容") }
	updates["updated_at"] = time.Now()
	if err := dao.UpdateProduct(id, updates); err != nil { return err }
	// [修复] 更新后同步 Redis 库存
	updatedProduct, _ := dao.GetProductByID(id)
	if updatedProduct != nil && updatedProduct.Status == 1 {
		ctx := context.Background()
		_ = redisClient.PreloadStock(ctx, id, updatedProduct.RemainStock)
	}
	return nil
}

func (s *ProductService) GetProductList(page, pageSize int) ([]model.Product, int64, error) {
	return dao.GetProductList(page, pageSize)
}

func (s *ProductService) GetProductDetail(productID int64) (*model.Product, error) {
	return dao.GetProductByID(productID)
}

func (s *ProductService) GetActiveProducts() ([]model.Product, error) {
	return dao.GetActiveProducts()
}

// [修复] BatchImportProducts 批量导入商品，返回成功和失败数量
func (s *ProductService) BatchImportProducts(reqs []model.CreateProductRequest) (int, int) {
	var successCount, failCount int
	for _, req := range reqs {
		_, err := s.CreateProduct(&req)
		if err != nil {
			failCount++
			continue
		}
		successCount++
	}
	return successCount, failCount
}