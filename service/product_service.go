package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/miaosha/dao"
	"github.com/miaosha/model"
	redisClient "github.com/miaosha/redis"
)

type ProductService struct{}

func NewProductService() *ProductService { return &ProductService{} }

// [修复] buildSKUs 将前端 SKU 输入转为实体并序列化 spec
func buildSKUs(productID int64, inputs []model.SKUInput) []*model.ProductSKU {
	skus := make([]*model.ProductSKU, 0, len(inputs))
	for _, in := range inputs {
		specJSON, err := json.Marshal(in.Spec)
		if err != nil || len(in.Spec) == 0 {
			continue
		}
		skus = append(skus, &model.ProductSKU{
			ProductID: productID, Spec: string(specJSON), Price: in.Price, Stock: in.Stock, Status: 1,
		})
	}
	return skus
}

func (s *ProductService) CreateProduct(req *model.CreateProductRequest) (*model.Product, error) {
	startTime, err := time.ParseInLocation("2006-01-02 15:04:05", req.StartTime, time.Local)
	if err != nil {
		return nil, fmt.Errorf("活动开始时间格式错误")
	}
	endTime, err := time.ParseInLocation("2006-01-02 15:04:05", req.EndTime, time.Local)
	if err != nil {
		return nil, fmt.Errorf("活动结束时间格式错误")
	}
	if endTime.Before(startTime) {
		return nil, fmt.Errorf("结束时间不能早于开始时间")
	}
	if req.LimitPerUser <= 0 {
		req.LimitPerUser = 1
	}
	product := &model.Product{
		Name: req.Name, Description: req.Description, Price: req.Price, SeckillPrice: req.SeckillPrice,
		TotalStock: req.TotalStock, RemainStock: req.TotalStock, StartTime: startTime, EndTime: endTime,
		Status: 1, ImageURL: req.ImageURL, LimitPerUser: req.LimitPerUser,
	}
	if err := dao.CreateProduct(product); err != nil {
		return nil, fmt.Errorf("创建商品失败: %w", err)
	}
	// [修复] 保存商品配置（SKU）：不同配置对应不同价格
	if len(req.SKUs) > 0 {
		if err := dao.ReplaceProductSKUs(product.ID, buildSKUs(product.ID, req.SKUs)); err != nil {
			return nil, fmt.Errorf("保存商品配置失败: %w", err)
		}
	}
	// 同步库存到Redis缓存，保证秒杀时能正确扣减库存
	_ = redisClient.PreloadStock(context.Background(), product.ID, product.TotalStock)
	return product, nil
}

func (s *ProductService) UpdateProduct(id int64, req *model.UpdateProductRequest) error {
	// [修复] 先获取旧商品信息，避免误重置 remain_stock
	oldProduct, err := dao.GetProductByID(id)
	if err != nil {
		return fmt.Errorf("商品不存在: %w", err)
	}
	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Price != nil {
		updates["price"] = *req.Price
	}
	if req.SeckillPrice != nil {
		updates["seckill_price"] = *req.SeckillPrice
	}
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
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.LimitPerUser != nil {
		updates["limit_per_user"] = *req.LimitPerUser
	}
	if req.ImageURL != nil {
		updates["image_url"] = *req.ImageURL
	}
	if req.StartTime != nil {
		t, err := time.ParseInLocation("2006-01-02 15:04:05", *req.StartTime, time.Local)
		if err != nil {
			return fmt.Errorf("开始时间格式错误")
		}
		updates["start_time"] = t
	}
	if req.EndTime != nil {
		t, err := time.ParseInLocation("2006-01-02 15:04:05", *req.EndTime, time.Local)
		if err != nil {
			return fmt.Errorf("结束时间格式错误")
		}
		updates["end_time"] = t
	}
	// [修复] 仅更新 SKU 配置（不传其他字段）也是有效更新
	if len(updates) == 0 && req.SKUs == nil {
		return fmt.Errorf("无更新内容")
	}
	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		if err := dao.UpdateProduct(id, updates); err != nil {
			return err
		}
	}
	// [修复] SKU 配置整体替换（req.SKUs 为 nil 时不动，传空数组表示清空配置）
	if req.SKUs != nil {
		if err := dao.ReplaceProductSKUs(id, buildSKUs(id, req.SKUs)); err != nil {
			return fmt.Errorf("保存商品配置失败: %w", err)
		}
	}
	// [修复] 商品状态变更时删除 Redis 商品缓存，确保下次秒杀读取最新数据
	ctx := context.Background()
	_ = redisClient.DeleteProductCache(ctx, id)
	// [修复] 更新后同步 Redis 库存
	updatedProduct, _ := dao.GetProductByID(id)
	if updatedProduct != nil && updatedProduct.Status == 1 {
		_ = redisClient.PreloadStock(ctx, id, updatedProduct.RemainStock)
	}
	return nil
}

func (s *ProductService) GetProductList(page, pageSize int) ([]model.Product, int64, error) {
	return dao.GetProductList(page, pageSize)
}

// [修复] BuildSKUAttrs 从 SKU 列表反推属性分组，用于前端渲染属性选择器
// 例如 [{颜色:黑},{颜色:白}] → [{name:"颜色",values:["黑","白"]}]
func BuildSKUAttrs(skus []model.ProductSKU) []model.SKUAttr {
	order := []string{}
	groups := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, sku := range skus {
		var spec map[string]string
		if err := json.Unmarshal([]byte(sku.Spec), &spec); err != nil {
			continue
		}
		for name, value := range spec {
			if _, ok := groups[name]; !ok {
				groups[name] = []string{}
				order = append(order, name)
				seen[name] = map[string]bool{}
			}
			if !seen[name][value] {
				seen[name][value] = true
				groups[name] = append(groups[name], value)
			}
		}
	}
	attrs := make([]model.SKUAttr, 0, len(order))
	for _, name := range order {
		attrs = append(attrs, model.SKUAttr{Name: name, Values: groups[name]})
	}
	return attrs
}

// [修复] GetProductDetail 返回商品详情 + SKU 配置列表 + 属性分组
func (s *ProductService) GetProductDetail(productID int64) (*model.ProductDetailResponse, error) {
	product, err := dao.GetProductByID(productID)
	if err != nil {
		return nil, err
	}
	skus, err := dao.GetProductSKUs(productID)
	if err != nil {
		skus = nil
	}
	return &model.ProductDetailResponse{Product: *product, SKUs: skus, Attrs: BuildSKUAttrs(skus)}, nil
}

// [修复] ProductWithSKU 秒杀首页列表项：商品 + 配置选项
type ProductWithSKU struct {
	model.Product
	SKUs  []model.ProductSKU `json:"skus"`
	Attrs []model.SKUAttr    `json:"attrs"`
}

// [修复] GetActiveProductsWithSKU 上架商品列表附带 SKU 配置（有配置的商品前端展示属性选择器）
func (s *ProductService) GetActiveProductsWithSKU() ([]ProductWithSKU, error) {
	products, err := dao.GetActiveProducts()
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(products))
	for _, p := range products {
		ids = append(ids, p.ID)
	}
	skuMap, err := dao.GetProductSKUsMap(ids)
	if err != nil {
		skuMap = nil
	}
	result := make([]ProductWithSKU, 0, len(products))
	for _, p := range products {
		item := ProductWithSKU{Product: p}
		if skuMap != nil {
			item.SKUs = skuMap[p.ID]
			item.Attrs = BuildSKUAttrs(item.SKUs)
		}
		result = append(result, item)
	}
	return result, nil
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
