package model

import (
	"time"
	"gorm.io/gorm"
)

type User struct {
	ID        int64          `gorm:"primaryKey;autoIncrement;comment:用户ID" json:"id"`
	Username  string         `gorm:"type:varchar(64);uniqueIndex;not null;comment:用户名" json:"username"`
	Password  string         `gorm:"type:varchar(256);not null;comment:密码" json:"-"`
	Nickname  string         `gorm:"type:varchar(128);default:'';comment:昵称" json:"nickname"`
	Phone     string         `gorm:"type:varchar(20);default:'';comment:手机号" json:"phone"`
	Email     string         `gorm:"type:varchar(128);default:'';comment:邮箱" json:"email"`
	Role      string         `gorm:"type:varchar(32);default:'user';comment:角色admin/user" json:"role"` // [修复] 默认值为"user"，确保新注册用户默认角色为普通用户
	Status    int8           `gorm:"type:tinyint;default:1;comment:状态1启用0禁用" json:"status"`
	CreatedAt time.Time      `gorm:"comment:创建时间" json:"created_at"`
	UpdatedAt time.Time      `gorm:"comment:更新时间" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index;comment:软删除" json:"-"`
}

func (User) TableName() string { return "t_user" }

type Product struct {
	ID           int64          `gorm:"primaryKey;autoIncrement;comment:商品ID" json:"id"`
	Name         string         `gorm:"type:varchar(256);not null;comment:商品名称" json:"name"`
	Description  string         `gorm:"type:text;comment:商品描述" json:"description"`
	Price        float64        `gorm:"type:decimal(10,2);not null;comment:原价" json:"price"`
	SeckillPrice float64        `gorm:"type:decimal(10,2);not null;comment:秒杀价" json:"seckill_price"`
	TotalStock   int            `gorm:"type:int;not null;comment:总库存" json:"total_stock"`
	RemainStock  int            `gorm:"type:int;not null;comment:剩余库存" json:"remain_stock"`
	StartTime    time.Time      `gorm:"type:datetime;not null;comment:开始时间" json:"start_time"`
	EndTime      time.Time      `gorm:"type:datetime;not null;comment:结束时间" json:"end_time"`
	Status       int8           `gorm:"type:tinyint;default:0;comment:1上架0下架" json:"status"`
	ImageURL     string         `gorm:"type:varchar(512);default:'';comment:图片" json:"image_url"`
	LimitPerUser int            `gorm:"type:int;default:1;comment:限购数量" json:"limit_per_user"`
	PayTimeout   int            `gorm:"type:int;default:30;comment:支付超时分钟数" json:"pay_timeout"` // [修复] 活动配置自定义支付超时
	CreatedAt    time.Time      `gorm:"comment:创建时间" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"comment:更新时间" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index;comment:软删除" json:"-"`
}

func (Product) TableName() string { return "t_product" }

type OrderStatus int8

const (
	OrderStatusPending   OrderStatus = 0
	OrderStatusPaid      OrderStatus = 1
	OrderStatusCancelled OrderStatus = 2
	OrderStatusRefunded  OrderStatus = 3
	OrderStatusTimeout   OrderStatus = 4 // [修复] 超时自动取消
)

type Order struct {
	ID           int64       `gorm:"primaryKey;autoIncrement;comment:订单ID" json:"id"`
	OrderNo      string      `gorm:"type:varchar(64);uniqueIndex;not null;comment:订单编号" json:"order_no"`
	UserID       int64       `gorm:"type:bigint;not null;index:idx_user_id;comment:用户ID" json:"user_id"`
	ProductID    int64       `gorm:"type:bigint;not null;comment:商品ID" json:"product_id"`
	ProductName  string      `gorm:"type:varchar(256);not null;comment:商品名称快照" json:"product_name"`
	SeckillPrice float64     `gorm:"type:decimal(10,2);not null;comment:秒杀价" json:"seckill_price"`
	Quantity     int         `gorm:"type:int;default:1;comment:数量" json:"quantity"`
	TotalAmount  float64     `gorm:"type:decimal(10,2);not null;comment:总金额" json:"total_amount"`
	Status       OrderStatus `gorm:"type:tinyint;default:0;comment:状态" json:"status"`
	PayTime      *time.Time  `gorm:"type:datetime;comment:支付时间" json:"pay_time,omitempty"`
	CancelTime   *time.Time  `gorm:"type:datetime;comment:取消时间" json:"cancel_time,omitempty"`
	CancelReason string      `gorm:"type:varchar(256);default:'';comment:取消原因" json:"cancel_reason,omitempty"`
	CreatedAt    time.Time   `gorm:"comment:创建时间" json:"created_at"`
	UpdatedAt    time.Time   `gorm:"comment:更新时间" json:"updated_at"`
}

type BehaviorTrack struct {
	TraceID    string `json:"trace_id"`
	UserID     int64  `json:"user_id"`
	ProductID  int64  `json:"product_id"`
	Action     string `json:"action"`
	RequestIP  string `json:"request_ip"`
	Result     int8   `json:"result"`
	CostMs     int64  `json:"cost_ms"`
	FailReason string `json:"fail_reason"`
	InstanceID string `json:"instance_id"`
	Timestamp  int64  `json:"timestamp"`
}

type ReconDiff struct {
	ID            int64      `gorm:"primaryKey;autoIncrement;comment:记录ID" json:"id"`
	ProductID     int64      `gorm:"type:bigint;not null;index;comment:商品ID" json:"product_id"`
	RedisStock    int        `gorm:"type:int;not null;comment:Redis库存" json:"redis_stock"`
	MySQLStock    int        `gorm:"type:int;not null;comment:MySQL库存" json:"mysql_stock"`
	Diff          int        `gorm:"type:int;not null;comment:差异值" json:"diff"`
	AutoCorrected bool       `gorm:"type:tinyint;default:0;comment:已修正" json:"auto_corrected"`
	CorrectedAt   *time.Time `gorm:"type:datetime;comment:修正时间" json:"corrected_at,omitempty"`
	CreatedAt     time.Time  `gorm:"comment:创建时间" json:"created_at"`
}

func (ReconDiff) TableName() string { return "t_recon_diff" }

type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=64"`
}

type LoginResponse struct {
	Token    string `json:"token"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=64"`
	Nickname string `json:"nickname" binding:"max=128"`
	Phone    string `json:"phone" binding:"max=20"`
}

type CreateProductRequest struct {
	Name         string  `json:"name" binding:"required,max=256"`
	Description  string  `json:"description"`
	Price        float64 `json:"price" binding:"required,gt=0"`
	SeckillPrice float64 `json:"seckill_price" binding:"required,gt=0"`
	TotalStock   int     `json:"total_stock" binding:"required,gt=0"`
	StartTime    string  `json:"start_time" binding:"required"`
	EndTime      string  `json:"end_time" binding:"required"`
	LimitPerUser int     `json:"limit_per_user"`
	ImageURL     string  `json:"image_url"`
}

type UpdateProductRequest struct {
	Name         *string  `json:"name"`
	Description  *string  `json:"description"`
	Price        *float64 `json:"price"`
	SeckillPrice *float64 `json:"seckill_price"`
	TotalStock   *int     `json:"total_stock"`
	StartTime    *string  `json:"start_time"`
	EndTime      *string  `json:"end_time"`
	Status       *int8    `json:"status"`
	LimitPerUser *int     `json:"limit_per_user"`
	ImageURL     *string  `json:"image_url"`
}

type SeckillRequest struct {
	ProductID     int64  `json:"product_id" binding:"required,gt=0"`
	Quantity      int    `json:"quantity" binding:"required,gt=0"`
	IdempotentKey string `json:"idempotent_key"` // [修复] 幂等性 Key，防止重复提交
}

type SeckillResponse struct {
	OrderNo string `json:"order_no"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type OrderQueryRequest struct {
	Page     int    `form:"page" binding:"min=1"`
	PageSize int    `form:"page_size" binding:"min=1,max=100"`
	Status   string `form:"status"` // [修复] 订单状态筛选：all/pending/paid/cancelled/timeout
}

// ==================== 黑名单模型 ====================

// Blacklist 黑名单表，用于封禁恶意用户或IP
type Blacklist struct {
	ID        int64     `gorm:"primaryKey;autoIncrement;comment:记录ID" json:"id"`
	Type      string    `gorm:"type:varchar(32);not null;index;comment:类型ip/user" json:"type"`
	Value     string    `gorm:"type:varchar(256);not null;uniqueIndex;comment:封禁值（IP地址或用户ID）" json:"value"`
	Reason    string    `gorm:"type:varchar(512);default:'';comment:封禁原因" json:"reason"`
	CreatedAt time.Time `gorm:"comment:创建时间" json:"created_at"`
}

func (Blacklist) TableName() string { return "t_blacklist" }

// ==================== Sentinel 规则模型（内存管理） ====================

// SentinelRule Sentinel 限流/熔断规则（仅用于前端展示和管理，实际规则存储在内存中）
type SentinelRule struct {
	ID               int64  `json:"id"`
	Resource         string `json:"resource"`          // 资源名
	Grade            int    `json:"grade"`             // 0=QPS, 1=线程数
	Count            int    `json:"count"`             // 阈值
	Strategy         int    `json:"strategy"`          // 0=直接, 1=关联, 2=链路
	ControlBehavior  int    `json:"control_behavior"`  // 0=快速失败, 1=WarmUp, 2=排队等待
	LimitApp         string `json:"limit_app"`         // 来源应用
	WarmUpPeriodSec  int    `json:"warm_up_period_sec"` // 预热时长(秒)
	MaxQueueingTimeMs int   `json:"max_queueing_time_ms"` // 排队超时(毫秒)
}

// SentinelRuleRequest 新增/修改 Sentinel 规则请求
type SentinelRuleRequest struct {
	Resource         string `json:"resource" binding:"required"`
	Grade            int    `json:"grade"`
	Count            int    `json:"count" binding:"required,gt=0"`
	Strategy         int    `json:"strategy"`
	ControlBehavior  int    `json:"control_behavior"`
	LimitApp         string `json:"limit_app"`
	WarmUpPeriodSec  int    `json:"warm_up_period_sec"`
	MaxQueueingTimeMs int   `json:"max_queueing_time_ms"`
}

// ==================== 告警模型 ====================

// Alarm 告警记录
type Alarm struct {
	ID        int64     `json:"id"`
	Level     string    `json:"level"`     // warning/critical
	Message   string    `json:"message"`   // 告警内容
	Source    string    `json:"source"`    // 告警来源
	CreatedAt time.Time `json:"created_at"`
}

// ==================== 管理员请求/响应结构体 ====================

// UserListResponse 用户列表响应
type UserListResponse struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	Phone     string    `json:"phone"`
	Role      string    `json:"role"`
	Status    int8      `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// UpdateUserRoleRequest 更新用户角色请求（四级角色：super_admin/operator/monitor_readonly/risk_control/user）
type UpdateUserRoleRequest struct {
	UserID int64  `json:"user_id" binding:"required,gt=0"`
	Role   string `json:"role" binding:"required,oneof=super_admin operator monitor_readonly risk_control admin user"`
}

// AdminOrderQueryRequest 管理员订单查询请求（支持跨用户查询）
type AdminOrderQueryRequest struct {
	Page     int    `form:"page" binding:"min=1"`
	PageSize int    `form:"page_size" binding:"min=1,max=100"`
	Status   *int8  `form:"status"`   // 订单状态筛选
	OrderNo  string `form:"order_no"` // 订单号搜索
	UserID   *int64 `form:"user_id"`  // 用户ID搜索
}

// ReconDiffQueryRequest 对账差异查询请求
type ReconDiffQueryRequest struct {
	Page     int `form:"page" binding:"min=1"`
	PageSize int `form:"page_size" binding:"min=1,max=100"`
}

// BlacklistRequest 黑名单操作请求
type BlacklistRequest struct {
	Type   string `json:"type" binding:"required,oneof=ip user"`
	Value  string `json:"value" binding:"required,max=256"`
	Reason string `json:"reason" binding:"max=512"`
}

// MonitorMetrics 监控指标
type MonitorMetrics struct {
	RejectCount  int64   `json:"reject_count"`   // 被拒绝请求数
	PassCount    int64   `json:"pass_count"`     // 通过请求数
	AvgRt        float64 `json:"avg_rt"`         // 平均响应时间(ms)
	QPS          float64 `json:"qps"`            // 当前QPS
	CPUUsage     float64 `json:"cpu_usage"`      // CPU使用率(%)
	MemUsage     float64 `json:"mem_usage"`      // 内存使用率(%)
	ActiveConns  int64   `json:"active_conns"`   // 活跃连接数
}

// MiddlewareStatus 中间件状态
type MiddlewareStatus struct {
	Name    string `json:"name"`     // 中间件名称
	Status  string `json:"status"`   // up/down
	Address string `json:"address"`  // 连接地址
	Uptime  string `json:"uptime"`   // 运行时间
}

// SeckillStats 秒杀统计
type SeckillStats struct {
	TotalOrders  int64   `json:"total_orders"`  // 总订单数
	SuccessRate  float64 `json:"success_rate"`  // 成功率(%)
	QPS          float64 `json:"qps"`           // 当前秒杀QPS
	MQBacklog    int64   `json:"mq_backlog"`    // MQ消息堆积数
}

// QPSDataPoint QPS时序数据点
type QPSDataPoint struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

// ==================== 活动配置模型 ====================

// [修复] ActivityConfigRequest 活动配置保存请求（支持动态设置限购数量等）
type ActivityConfigRequest struct {
	ProductID    int64  `json:"product_id" binding:"required,gt=0"`
	LimitPerUser *int   `json:"limit_per_user"` // 单用户限购数量
	Status       *int8  `json:"status"`         // 商品上下架状态
	StartTime    *string `json:"start_time"`    // 活动开始时间
	EndTime      *string `json:"end_time"`      // 活动结束时间
	SeckillPrice *float64 `json:"seckill_price"` // 秒杀价格
}

// [修复] ActivityConfigResponse 活动配置响应
type ActivityConfigResponse struct {
	ProductID    int64   `json:"product_id"`
	Name         string  `json:"name"`
	LimitPerUser int     `json:"limit_per_user"`
	Status       int8    `json:"status"`
	SeckillPrice float64 `json:"seckill_price"`
	RemainStock  int     `json:"remain_stock"`
	TotalStock   int     `json:"total_stock"`
	StartTime    string  `json:"start_time"`
	EndTime      string  `json:"end_time"`
}

// ==================== 审计日志模型 ====================

// [修复] AuditLog 操作审计日志，记录所有后台修改操作
type AuditLog struct {
	ID          int64     `gorm:"primaryKey;autoIncrement;comment:日志ID" json:"id"`
	UserID      int64     `gorm:"not null;index;comment:操作用户ID" json:"user_id"`
	Username    string    `gorm:"type:varchar(64);not null;comment:操作用户名" json:"username"`
	Action      string    `gorm:"type:varchar(64);not null;comment:操作类型(create/update/delete/export)" json:"action"`
	Module      string    `gorm:"type:varchar(64);not null;comment:操作模块(product/order/user/sentinel)" json:"module"`
	TargetID    string    `gorm:"type:varchar(128);comment:操作目标ID" json:"target_id"`
	Detail      string    `gorm:"type:text;comment:操作详情JSON" json:"detail"`
	ClientIP    string    `gorm:"type:varchar(64);comment:客户端IP" json:"client_ip"`
	TraceID     string    `gorm:"type:varchar(128);comment:TraceID" json:"trace_id"`
	CreatedAt   time.Time `gorm:"comment:操作时间" json:"created_at"`
}

func (AuditLog) TableName() string { return "t_audit_log" }

// [修复] AuditLogQueryRequest 审计日志查询请求
type AuditLogQueryRequest struct {
	Page     int    `form:"page" binding:"min=1"`
	PageSize int    `form:"page_size" binding:"min=1,max=100"`
	Module   string `form:"module"`
	Username string `form:"username"`
	Action   string `form:"action"`
}