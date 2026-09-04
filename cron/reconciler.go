package cron

import (
	"context"
	"fmt"
	"time"

	"github.com/miaosha/config"
	"github.com/miaosha/dao"
	"github.com/miaosha/log"
	"github.com/miaosha/model"
	redisClient "github.com/miaosha/redis"
)

func StartReconciler() {
	cfg := config.GetConfig()
	if !cfg.Reconciler.Enabled {
		log.L().Info("数据对账任务已禁用")
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.Reconciler.IntervalSec) * time.Second)
		defer ticker.Stop()
		log.L().Infow("数据对账任务已启动", "interval_sec", cfg.Reconciler.IntervalSec)
		for range ticker.C {
			runReconciliation(cfg)
		}
	}()
}

func runReconciliation(cfg *config.Config) {
	ctx := context.Background()
	// [修复] 对账开始时间，用于计算耗时
	startTime := time.Now()

	// [修复] 使用 Redis SETNX 分布式锁，防止多实例重复执行对账任务
	lockKey := fmt.Sprintf("reconciler:lock:%s", cfg.Server.InstanceID)
	lockTimeout := 60 * time.Second
	locked, err := redisClient.GetClient().SetNX(ctx, lockKey, "1", lockTimeout).Result()
	if err != nil {
		log.L().Errorw("对账任务获取分布式锁失败", "error", err)
		return
	}
	if !locked {
		log.L().Debug("对账任务已被其他实例持有，跳过本次执行")
		return
	}
	// [修复] 对账完成后释放锁，避免锁被提前释放导致重复执行
	defer func() {
		if err := redisClient.GetClient().Del(ctx, lockKey).Err(); err != nil {
			log.L().Warnw("对账任务释放分布式锁失败", "error", err)
		}
	}()

	products, err := dao.GetAllProducts()
	if err != nil {
		log.L().Errorw("对账任务获取商品列表失败", "error", err)
		return
	}

	// [修复] 对账统计：差异数量和修复数量
	var diffCount, fixCount int
	for _, product := range products {
		// [修复] 对单个商品的对账支持重试（最多 3 次）
		reconciled := false
		for attempt := 1; attempt <= 3; attempt++ {
			err := reconcileSingleProduct(ctx, product, &diffCount, &fixCount)
			if err == nil {
				reconciled = true
				break
			}
			log.L().Warnw("对账失败，准备重试",
				"product_id", product.ID,
				"attempt", attempt,
				"error", err)
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
		if !reconciled {
			log.L().Errorw("对账多次重试后仍失败，跳过该商品", "product_id", product.ID)
		}
	}

	// [修复] 对账完成后记录耗时、差异数量和修复数量
	elapsed := time.Since(startTime)
	log.L().Infow("对账任务完成", "耗时_ms", elapsed.Milliseconds(), "商品总数", len(products), "差异数量", diffCount, "修复数量", fixCount)
}

// [修复] reconcileSingleProduct 对单个商品执行对账，返回 error 表示需要重试
func reconcileSingleProduct(ctx context.Context, product model.Product, diffCount, fixCount *int) error {
	redisStock, err := redisClient.GetStock(ctx, product.ID)
	if err != nil {
		return fmt.Errorf("获取Redis库存失败: %w", err)
	}
	mysqlStock := product.RemainStock
	if redisStock != mysqlStock {
		*diffCount++
		diff := redisStock - mysqlStock
		log.L().Warnw("发现库存差异", "product_id", product.ID, "redis_stock", redisStock, "mysql_stock", mysqlStock, "diff", diff)
		reconDiff := &model.ReconDiff{ProductID: product.ID, RedisStock: redisStock, MySQLStock: mysqlStock, Diff: diff}
		if err := dao.InsertReconDiff(reconDiff); err != nil {
			return fmt.Errorf("记录对账差异失败: %w", err)
		}
		if err := redisClient.PreloadStock(ctx, product.ID, mysqlStock); err != nil {
			return fmt.Errorf("自动修正Redis库存失败: %w", err)
		}
		*fixCount++
		log.L().Infow("已自动修正Redis库存", "product_id", product.ID, "stock", mysqlStock)
		_ = dao.UpdateReconDiffCorrected(reconDiff.ID)
	}
	return nil
}
