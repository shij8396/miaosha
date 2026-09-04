package dao

import (
	"fmt"
	"sync"
	"time"

	"github.com/miaosha/config"
	miaoshaLog "github.com/miaosha/log"
	"github.com/miaosha/model"
	"github.com/miaosha/utils"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
)

var (
	db     *gorm.DB
	dbOnce sync.Once
)

func InitDB(cfg *config.MySQLConfig) error {
	var initErr error
	dbOnce.Do(func() {
		masterDSN := cfg.Master.GetDSN()
		// [修复] DSN 添加 readTimeout/writeTimeout 参数，防止数据库僵死阻塞请求
		masterDSN = masterDSN + fmt.Sprintf("&readTimeout=5s&writeTimeout=5s")

		// [优化] 生产环境关闭 GORM SQL 日志（减少每条 SQL ~50μs 日志写入开销）
		// 开发/调试时可改为 logger.Warn 查看慢查询
		slowLogger := logger.New(
			miaoshaLog.NewGormWriter(),
			logger.Config{
				SlowThreshold:             200 * time.Millisecond, // 慢查询阈值
				LogLevel:                  logger.Silent,          // [优化] 生产静默，压测时关闭 SQL 日志
				IgnoreRecordNotFoundError: true,                   // 忽略 RecordNotFound 错误
				Colorful:                  false,                  // 禁用颜色输出
			},
		)

		gormDB, err := gorm.Open(mysql.Open(masterDSN), &gorm.Config{
			Logger: slowLogger,
		})
		if err != nil {
			initErr = fmt.Errorf("连接MySQL主库失败: %w", err)
			return
		}
		sqlDB, _ := gormDB.DB()
		sqlDB.SetMaxIdleConns(cfg.Master.MaxIdleConns)
		sqlDB.SetMaxOpenConns(cfg.Master.MaxOpenConns)
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.Master.ConnMaxLifetime) * time.Second)
		if len(cfg.Slaves) > 0 {
			var replicas []gorm.Dialector
			for _, slave := range cfg.Slaves {
				replicas = append(replicas, mysql.Open(slave.GetDSN()))
			}
			err = gormDB.Use(dbresolver.Register(dbresolver.Config{Replicas: replicas, Policy: dbresolver.RandomPolicy{}}))
			if err != nil {
				initErr = fmt.Errorf("配置读写分离失败: %w", err)
				return
			}
		}
		// [修复] 仅对基础表（用户、商品、对账差异、黑名单）执行 AutoMigrate
		// 订单分表（t_order_0 ~ t_order_15）的建表逻辑由 init.sql 统一管理
		// 不再在此处手动创建分表，避免与 SQL 初始化脚本冲突
		err = gormDB.AutoMigrate(&model.User{}, &model.Product{}, &model.ProductSKU{}, &model.ReconDiff{}, &model.Blacklist{}, &model.AuditLog{})
		if err != nil {
			fmt.Printf("[DAO] 自动迁移警告（表可能已存在）: %v\n", err)
		}
		// [修复] 订单分表（t_order_0 ~ t_order_15）由项目根目录的 init.sql 统一管理
		// 请确保在首次部署时执行 init.sql 完成建表初始化
		fmt.Println("[DAO] 数据库初始化完成（基础表 AutoMigrate，分表由 init.sql 管理）")
		db = gormDB
	})
	return initErr
}

func GetDB() *gorm.DB      { return db }
func GetReadDB() *gorm.DB  { return db.Clauses(dbresolver.Read) }
func GetWriteDB() *gorm.DB { return db.Clauses(dbresolver.Write) }

// [修复] Ping 检查数据库连接是否正常，用于健康检查探针
func Ping() error {
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	return sqlDB.Ping()
}

// User DAO
func CreateUser(user *model.User) error { return GetWriteDB().Create(user).Error }
func GetUserByUsername(username string) (*model.User, error) {
	var user model.User
	err := GetWriteDB().Where("username = ?", username).First(&user).Error
	if err != nil { return nil, err }
	return &user, nil
}
func GetUserByID(userID int64) (*model.User, error) {
	var user model.User
	err := GetReadDB().Where("id = ?", userID).First(&user).Error
	if err != nil { return nil, err }
	return &user, nil
}

// Product DAO
func CreateProduct(product *model.Product) error { return GetWriteDB().Create(product).Error }
func UpdateProduct(id int64, updates map[string]interface{}) error {
	return GetWriteDB().Model(&model.Product{}).Where("id = ?", id).Updates(updates).Error
}
func GetProductByID(productID int64) (*model.Product, error) {
	var product model.Product
	err := GetReadDB().Where("id = ?", productID).First(&product).Error
	if err != nil { return nil, err }
	return &product, nil
}
func GetProductList(page, pageSize int) ([]model.Product, int64, error) {
	var products []model.Product
	var total int64
	offset := (page - 1) * pageSize
	if err := GetReadDB().Model(&model.Product{}).Count(&total).Error; err != nil { return nil, 0, err }
	err := GetReadDB().Offset(offset).Limit(pageSize).Order("id DESC").Find(&products).Error
	return products, total, err
}
func GetActiveProducts() ([]model.Product, error) {
	var products []model.Product
	now := time.Now()
	// [修复] 按创建时间倒序（ID 倒序），新添加的商品显示在最上面
	err := GetReadDB().Where("status = 1 AND start_time <= ? AND end_time >= ?", now, now).Order("id DESC").Find(&products).Error
	return products, err
}
func DeductProductStock(productID int64, quantity int) error {
	result := GetWriteDB().Model(&model.Product{}).Where("id = ? AND remain_stock >= ?", productID, quantity).Update("remain_stock", gorm.Expr("remain_stock - ?", quantity))
	if result.Error != nil { return result.Error }
	if result.RowsAffected == 0 { return fmt.Errorf("库存不足") }
	return nil
}
func RollbackProductStock(productID int64, quantity int) error {
	return GetWriteDB().Model(&model.Product{}).Where("id = ?", productID).Update("remain_stock", gorm.Expr("remain_stock + ?", quantity)).Error
}

// ProductSKU DAO：商品配置（不同配置不同价格）
// [修复] ReplaceProductSKUs 整体替换商品的 SKU 配置（删除旧 + 写入新，软删除保留历史）
func ReplaceProductSKUs(productID int64, skus []*model.ProductSKU) error {
	return GetWriteDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("product_id = ?", productID).Delete(&model.ProductSKU{}).Error; err != nil {
			return err
		}
		if len(skus) == 0 { return nil }
		for _, s := range skus { s.ProductID = productID }
		return tx.Create(&skus).Error
	})
}
func GetProductSKUs(productID int64) ([]model.ProductSKU, error) {
	var skus []model.ProductSKU
	err := GetReadDB().Where("product_id = ? AND status = 1", productID).Order("id ASC").Find(&skus).Error
	return skus, err
}
func GetProductSKUsMap(productIDs []int64) (map[int64][]model.ProductSKU, error) {
	result := make(map[int64][]model.ProductSKU)
	if len(productIDs) == 0 { return result, nil }
	var skus []model.ProductSKU
	err := GetReadDB().Where("product_id IN ? AND status = 1", productIDs).Order("id ASC").Find(&skus).Error
	if err != nil { return nil, err }
	for _, s := range skus {
		result[s.ProductID] = append(result[s.ProductID], s)
	}
	return result, nil
}
func GetSKUByID(skuID int64) (*model.ProductSKU, error) {
	var sku model.ProductSKU
	err := GetReadDB().Where("id = ? AND status = 1", skuID).First(&sku).Error
	if err != nil { return nil, err }
	return &sku, nil
}

// Order DAO
func CreateOrder(order *model.Order, shardCount int) error {
	tableName := utils.GetOrderTableName(order.UserID, shardCount)
	return GetWriteDB().Table(tableName).Create(order).Error
}

// [优化] CreateOrderWithStock 在同一个事务中创建订单 + 扣减库存
// 合并原来 2 次 DB 往返为 1 次事务提交，减少网络开销和锁竞争
func CreateOrderWithStock(order *model.Order, shardCount int) error {
	return GetWriteDB().Transaction(func(tx *gorm.DB) error {
		tableName := utils.GetOrderTableName(order.UserID, shardCount)
		if err := tx.Table(tableName).Create(order).Error; err != nil {
			return err
		}
		result := tx.Model(&model.Product{}).Where("id = ? AND remain_stock >= ?", order.ProductID, order.Quantity).
			Update("remain_stock", gorm.Expr("remain_stock - ?", order.Quantity))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("库存不足")
		}
		return nil
	})
}
func GetOrderByOrderNo(orderNo string, userID int64, shardCount int) (*model.Order, error) {
	var order model.Order
	tableName := utils.GetOrderTableName(userID, shardCount)
	err := GetReadDB().Table(tableName).Where("order_no = ?", orderNo).First(&order).Error
	if err != nil { return nil, err }
	return &order, nil
}
func GetUserOrders(userID int64, page, pageSize, shardCount int, status string) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64
	tableName := utils.GetOrderTableName(userID, shardCount)
	offset := (page - 1) * pageSize
	// [修复] 支持按订单状态筛选
	db := GetReadDB().Table(tableName).Where("user_id = ?", userID)
	switch status {
	case "pending":
		db = db.Where("status = ?", model.OrderStatusPending)
	case "paid":
		db = db.Where("status = ?", model.OrderStatusPaid)
	case "cancelled":
		db = db.Where("status = ?", model.OrderStatusCancelled)
	case "refunded":
		db = db.Where("status = ?", model.OrderStatusRefunded)
	case "timeout":
		db = db.Where("status = ?", model.OrderStatusTimeout)
	}
	if err := db.Count(&total).Error; err != nil { return nil, 0, err }
	err := db.Offset(offset).Limit(pageSize).Order("id DESC").Find(&orders).Error
	return orders, total, err
}
func UpdateOrderStatus(orderNo string, userID int64, status model.OrderStatus, shardCount int) error {
	tableName := utils.GetOrderTableName(userID, shardCount)
	return GetWriteDB().Table(tableName).Where("order_no = ?", orderNo).Updates(map[string]interface{}{"status": status, "updated_at": time.Now()}).Error
}

// [修复] UpdateOrderStatusWithPayTime 更新订单状态并支持设置支付时间
// payTime 为 nil 时不更新支付时间字段
func UpdateOrderStatusWithPayTime(orderNo string, userID int64, status model.OrderStatus, payTime *time.Time, shardCount int) error {
	tableName := utils.GetOrderTableName(userID, shardCount)
	updates := map[string]interface{}{"status": status, "updated_at": time.Now()}
	if payTime != nil {
		updates["pay_time"] = payTime
	}
	return GetWriteDB().Table(tableName).Where("order_no = ?", orderNo).Updates(updates).Error
}
func CancelOrder(orderNo string, userID int64, reason string, shardCount int) error {
	tableName := utils.GetOrderTableName(userID, shardCount)
	now := time.Now()
	return GetWriteDB().Table(tableName).Where("order_no = ?", orderNo).Updates(map[string]interface{}{
		"status": model.OrderStatusCancelled, "cancel_time": now, "cancel_reason": reason, "updated_at": now,
	}).Error
}

// [修复] CancelOrderWithStatus 支持指定取消状态（用户取消/超时取消）
func CancelOrderWithStatus(orderNo string, userID int64, reason string, status model.OrderStatus, shardCount int) error {
	tableName := utils.GetOrderTableName(userID, shardCount)
	now := time.Now()
	return GetWriteDB().Table(tableName).Where("order_no = ?", orderNo).Updates(map[string]interface{}{
		"status": status, "cancel_time": now, "cancel_reason": reason, "updated_at": now,
	}).Error
}
func GetUnpaidOrdersByUser(userID int64, shardCount int) ([]model.Order, error) {
	var orders []model.Order
	tableName := utils.GetOrderTableName(userID, shardCount)
	err := GetReadDB().Table(tableName).Where("user_id = ? AND status = ?", userID, model.OrderStatusPending).Find(&orders).Error
	return orders, err
}

// ReconDiff DAO
func InsertReconDiff(diff *model.ReconDiff) error { return GetWriteDB().Create(diff).Error }
func UpdateReconDiffCorrected(id int64) error {
	now := time.Now()
	return GetWriteDB().Model(&model.ReconDiff{}).Where("id = ?", id).Updates(map[string]interface{}{"auto_corrected": true, "corrected_at": now}).Error
}
func GetReconDiffList(page, pageSize int) ([]model.ReconDiff, int64, error) {
	var diffs []model.ReconDiff
	var total int64
	offset := (page - 1) * pageSize
	if err := GetReadDB().Model(&model.ReconDiff{}).Count(&total).Error; err != nil { return nil, 0, err }
	err := GetReadDB().Offset(offset).Limit(pageSize).Order("id DESC").Find(&diffs).Error
	return diffs, total, err
}
func GetAllProducts() ([]model.Product, error) {
	var products []model.Product
	err := GetReadDB().Find(&products).Error
	return products, err
}

// ==================== 用户管理 DAO ====================

// GetUserList 获取用户列表（分页）
func GetUserList(page, pageSize int) ([]model.User, int64, error) {
	var users []model.User
	var total int64
	offset := (page - 1) * pageSize
	if err := GetReadDB().Model(&model.User{}).Count(&total).Error; err != nil { return nil, 0, err }
	err := GetReadDB().Offset(offset).Limit(pageSize).Order("id DESC").Find(&users).Error
	return users, total, err
}

// UpdateUserRole 更新用户角色
func UpdateUserRole(userID int64, role string) error {
	return GetWriteDB().Model(&model.User{}).Where("id = ?", userID).Update("role", role).Error
}

// [修复] UpdatePassword 更新用户密码
func UpdatePassword(userID int64, hashedPassword string) error {
	return GetWriteDB().Model(&model.User{}).Where("id = ?", userID).Update("password", hashedPassword).Error
}

// [修复] 审计日志相关方法
// CreateAuditLog 写入操作审计日志
func CreateAuditLog(log *model.AuditLog) error {
	return GetWriteDB().Create(log).Error
}

// GetAuditLogs 分页查询审计日志
func GetAuditLogs(page, pageSize int, operatorID int64, action string) ([]model.AuditLog, int64, error) {
	var logs []model.AuditLog
	var total int64
	query := GetWriteDB().Model(&model.AuditLog{})
	if operatorID > 0 {
		query = query.Where("user_id = ?", operatorID)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// ==================== 订单管理 DAO（管理员跨分表查询） ====================

// GetAllOrders 管理员查询所有订单（跨所有16张分表）
func GetAllOrders(page, pageSize int, status *int8, orderNo string, userID *int64) ([]model.Order, int64, error) {
	var allOrders []model.Order
	var total int64

	// 如果指定了 userID，只查该用户的分表
	shardRange := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	if userID != nil {
		shardIndex := int(*userID % 16)
		shardRange = []int{shardIndex}
	}

	// 先统计总数
	for _, i := range shardRange {
		tableName := fmt.Sprintf("t_order_%d", i)
		query := GetReadDB().Table(tableName)
		if status != nil {
			query = query.Where("status = ?", *status)
		}
		if orderNo != "" {
			query = query.Where("order_no LIKE ?", "%"+orderNo+"%")
		}
		if userID != nil {
			query = query.Where("user_id = ?", *userID)
		}
		var shardTotal int64
		if err := query.Count(&shardTotal).Error; err != nil { return nil, 0, err }
		total += shardTotal
	}

	// 分页查询：简单实现 — 从各分表收集数据
	// 对于跨分表分页，采用 UNION ALL 方式
	offset := (page - 1) * pageSize
	if total > 0 {
		// 构建 UNION ALL 查询
		var unionQueries []string
		var args []interface{}
		for _, i := range shardRange {
			tableName := fmt.Sprintf("t_order_%d", i)
			whereClause := "1=1"
			extraArgs := []interface{}{}
			if status != nil {
				whereClause += " AND status = ?"
				extraArgs = append(extraArgs, *status)
			}
			if orderNo != "" {
				whereClause += " AND order_no LIKE ?"
				extraArgs = append(extraArgs, "%"+orderNo+"%")
			}
			if userID != nil {
				whereClause += " AND user_id = ?"
				extraArgs = append(extraArgs, *userID)
			}
			unionQueries = append(unionQueries, fmt.Sprintf("SELECT * FROM %s WHERE %s", tableName, whereClause))
			args = append(args, extraArgs...)
		}

		unionSQL := ""
		for idx, q := range unionQueries {
			if idx > 0 { unionSQL += " UNION ALL " }
			unionSQL += q
		}
		unionSQL += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
		args = append(args, pageSize, offset)

		if err := GetReadDB().Raw(unionSQL, args...).Scan(&allOrders).Error; err != nil {
			return nil, 0, err
		}
	}

	return allOrders, total, nil
}

// GetTotalOrderCount 获取所有订单总数
func GetTotalOrderCount() (int64, error) {
	var total int64
	for i := 0; i < 16; i++ {
		tableName := fmt.Sprintf("t_order_%d", i)
		var shardTotal int64
		if err := GetReadDB().Table(tableName).Count(&shardTotal).Error; err != nil { return 0, err }
		total += shardTotal
	}
	return total, nil
}

// ==================== 黑名单 DAO ====================

// CreateBlacklist 添加黑名单
func CreateBlacklist(bl *model.Blacklist) error {
	return GetWriteDB().Create(bl).Error
}

// GetBlacklist 获取黑名单列表
func GetBlacklist() ([]model.Blacklist, error) {
	var list []model.Blacklist
	err := GetReadDB().Order("id DESC").Find(&list).Error
	return list, err
}

// DeleteBlacklist 删除黑名单
func DeleteBlacklist(id int64) error {
	return GetWriteDB().Where("id = ?", id).Delete(&model.Blacklist{}).Error
}

// CheckBlacklist 检查是否在黑名单中
func CheckBlacklist(typ, value string) (bool, error) {
	var count int64
	err := GetReadDB().Model(&model.Blacklist{}).Where("type = ? AND value = ?", typ, value).Count(&count).Error
	return count > 0, err
}