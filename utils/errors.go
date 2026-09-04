package utils

import "errors"

// [修复] 业务错误码枚举：替代字符串匹配，支持 errors.Is() 精确判断
// 前端的差异化提示文案不再依赖 service 层返回的具体字符串，改为依赖错误类型
var (
	ErrStockInsufficient = errors.New("库存不足")
	ErrRedisDown         = errors.New("Redis服务异常")
	ErrMQDown            = errors.New("消息队列异常")
	ErrProductOffline    = errors.New("商品已下架")
	ErrNotInSeckillTime  = errors.New("不在秒杀活动时间内")
	ErrAlreadyPurchased  = errors.New("已参与过")
	ErrExceedLimit       = errors.New("超出单用户限购数量")
	ErrDuplicateSubmit   = errors.New("请勿重复提交")
	ErrSeckillOverloaded = errors.New("当前抢购人数过多")
)

// SeckillError 带详细信息的秒杀业务错误
type SeckillError struct {
	Err      error
	Message  string // 用户可见的提示信息
	HTTPCode int    // HTTP 状态码
}

func (e *SeckillError) Error() string {
	return e.Message
}

func (e *SeckillError) Unwrap() error {
	return e.Err
}

// NewSeckillError 创建带用户提示信息的秒杀错
func NewSeckillError(err error, message string, httpCode int) *SeckillError {
	return &SeckillError{Err: err, Message: message, HTTPCode: httpCode}
}
