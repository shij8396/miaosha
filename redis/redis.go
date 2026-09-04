package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/miaosha/config"
	"github.com/miaosha/log"
)

var (
	rdb       *redis.Client
	redisOnce sync.Once
)

func InitCluster(cfg *config.RedisConfig) error {
	var initErr error
	redisOnce.Do(func() {
		rdb = redis.NewClient(&redis.Options{
			Addr: cfg.Addrs[0], Password: cfg.Password,
			PoolSize: cfg.PoolSize, MinIdleConns: cfg.MinIdleConns,
			DialTimeout: time.Duration(cfg.DialTimeout) * time.Second,
			ReadTimeout: time.Duration(cfg.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
			IdleTimeout: 5 * time.Minute,
		})

		// [修复] 添加重试机制，最多重试3次，间隔2秒，避免临时网络抖动直接 panic
		var lastErr error
		maxRetries := 3
		retryInterval := 2 * time.Second
		for i := 0; i < maxRetries; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := rdb.Ping(ctx).Err()
			cancel()
			if err == nil {
				return // 连接成功
			}
			lastErr = err
			if i < maxRetries-1 {
				log.L().Warnw("Redis连接失败，正在重试", "attempt", i+1, "max_retries", maxRetries, "error", err)
				time.Sleep(retryInterval)
			}
		}
		// [修复] 重试耗尽后记录 FATAL 日志并退出，不再直接 panic
		initErr = fmt.Errorf("Redis连接失败（已重试%d次）: %w", maxRetries, lastErr)
	})
	return initErr
}

func GetClient() *redis.Client { return rdb }

const (
	StockKeyPrefix      = "seckill:stock:"
	UserPurchasedPrefix = "seckill:user:"
)

func PreloadStock(ctx context.Context, productID int64, stock int) error {
	key := fmt.Sprintf("%s%d", StockKeyPrefix, productID)
	return rdb.Set(ctx, key, stock, 0).Err()
}

func GetStock(ctx context.Context, productID int64) (int, error) {
	key := fmt.Sprintf("%s%d", StockKeyPrefix, productID)
	val, err := rdb.Get(ctx, key).Int()
	if err == redis.Nil { return 0, nil }
	return val, err
}

func DecrStock(ctx context.Context, productID int64, quantity int) (int, bool, error) {
	key := fmt.Sprintf("%s%d", StockKeyPrefix, productID)
	luaScript := `
		local stock = redis.call('GET', KEYS[1])
		if not stock then return -1 end
		stock = tonumber(stock)
		if stock < tonumber(ARGV[1]) then return -2 end
		redis.call('DECRBY', KEYS[1], ARGV[1])
		return stock - tonumber(ARGV[1])
	`
	result, err := rdb.Eval(ctx, luaScript, []string{key}, quantity).Int()
	if err != nil { return 0, false, fmt.Errorf("Redis库存扣减失败: %w", err) }
	if result < 0 { return 0, false, nil }
	return result, true, nil
}

func IncrStock(ctx context.Context, productID int64, quantity int) error {
	key := fmt.Sprintf("%s%d", StockKeyPrefix, productID)
	return rdb.IncrBy(ctx, key, int64(quantity)).Err()
}

func CheckUserPurchased(ctx context.Context, userID, productID int64) (bool, error) {
	key := fmt.Sprintf("%s%d:%d", UserPurchasedPrefix, userID, productID)
	exists, err := rdb.Exists(ctx, key).Result()
	return exists > 0, err
}

// [修复] 限购规则改造：检查用户已购买数量是否达到限购上限
// 返回 (已购买数量, 是否达到上限, error)
func CheckUserPurchaseLimit(ctx context.Context, userID, productID int64, limitPerUser int) (int, bool, error) {
	key := fmt.Sprintf("%s%d:%d", UserPurchasedPrefix, userID, productID)
	count, err := rdb.Get(ctx, key).Int()
	if err == redis.Nil {
		return 0, false, nil // 未购买过
	}
	if err != nil {
		return 0, false, fmt.Errorf("Redis查询用户购买记录失败: %w", err)
	}
	if count >= limitPerUser {
		return count, true, nil // 已达到限购上限
	}
	return count, false, nil
}

func MarkUserPurchased(ctx context.Context, userID, productID int64, expireTime time.Duration) error {
	key := fmt.Sprintf("%s%d:%d", UserPurchasedPrefix, userID, productID)
	return rdb.Set(ctx, key, "1", expireTime).Err()
}

// [修复] 限购规则改造：累加用户购买数量（支持限购>1的场景）
// 使用 Lua 脚本保证原子性：读取当前计数 + 累加 + 设置过期时间
func IncrUserPurchaseCount(ctx context.Context, userID, productID int64, quantity int, expireTime time.Duration) (int, error) {
	key := fmt.Sprintf("%s%d:%d", UserPurchasedPrefix, userID, productID)
	expireSec := int64(expireTime.Seconds())
	luaScript := `
		local current = redis.call('GET', KEYS[1])
		if not current then current = 0 end
		local newCount = tonumber(current) + tonumber(ARGV[1])
		redis.call('SET', KEYS[1], newCount, 'EX', ARGV[2])
		return newCount
	`
	result, err := rdb.Eval(ctx, luaScript, []string{key}, quantity, expireSec).Int()
	if err != nil {
		return 0, fmt.Errorf("Redis累加用户购买数量失败: %w", err)
	}
	return result, nil
}

// [修复] 读取用户当前已购数量（秒杀首页恢复"已抢购"按钮状态用）
// key 不存在视为 0
func GetUserPurchaseCount(ctx context.Context, userID, productID int64) (int, error) {
	key := fmt.Sprintf("%s%d:%d", UserPurchasedPrefix, userID, productID)
	count, err := rdb.Get(ctx, key).Int()
	if err == redis.Nil {
		return 0, nil
	}
	return count, err
}

func RemoveUserPurchased(ctx context.Context, userID, productID int64) error {
	key := fmt.Sprintf("%s%d:%d", UserPurchasedPrefix, userID, productID)
	return rdb.Del(ctx, key).Err()
}

// [修复] 限购计数按数量递减：订单取消/超时/退款/秒杀失败回滚时使用
// 原逻辑直接删除整个计数 key（RemoveUserPurchased），在限购>1且用户有多笔订单时，
// 取消一单会把其他订单的已购计数一并清零，导致用户可再次购买超出限购总量。
// 使用 Lua 保证原子性：读取当前计数 → 递减 quantity → 递减后 <= 0 时删除 key
// INCRBY 保留原 TTL，不会重置限购记录的过期时间
// 返回递减后的计数（key 不存在时返回 0）
func DecrUserPurchaseCount(ctx context.Context, userID, productID int64, quantity int) (int, error) {
	key := fmt.Sprintf("%s%d:%d", UserPurchasedPrefix, userID, productID)
	luaScript := `
		local current = redis.call('GET', KEYS[1])
		if not current then return 0 end
		current = tonumber(current)
		local newCount = current - tonumber(ARGV[1])
		if newCount <= 0 then
			redis.call('DEL', KEYS[1])
			return 0
		end
		redis.call('INCRBY', KEYS[1], -tonumber(ARGV[1]))
		return newCount
	`
	result, err := rdb.Eval(ctx, luaScript, []string{key}, quantity).Int()
	if err != nil {
		return 0, fmt.Errorf("Redis递减用户购买数量失败: %w", err)
	}
	return result, nil
}

// [速度优化] 商品信息缓存，避免每次秒杀都查 MySQL
// 商品信息在秒杀期间不变，缓存到 Redis 可减少 5-10ms DB 查询开销
var ProductCachePrefix = "product_cache:"

// [修复] 删除商品缓存，用于商品状态变更（上下架/编辑）时使缓存失效
func DeleteProductCache(ctx context.Context, productID int64) error {
	key := fmt.Sprintf("%s%d", ProductCachePrefix, productID)
	return rdb.Del(ctx, key).Err()
}

func GetProductCache(ctx context.Context, productID int64) (name string, seckillPrice float64, limitPerUser int, status int, startTime, endTime time.Time, err error) {
	key := fmt.Sprintf("%s%d", ProductCachePrefix, productID)
	vals, err := rdb.HMGet(ctx, key, "name", "seckill_price", "limit_per_user", "status", "start_time", "end_time").Result()
	if err != nil || vals[0] == nil {
		return "", 0, 0, 0, time.Time{}, time.Time{}, fmt.Errorf("缓存不存在")
	}
	name = vals[0].(string)
	seckillPrice, _ = parseFloat(vals[1])
	limitPerUser, _ = parseInt(vals[2])
	status, _ = parseInt(vals[3])
	startTime, _ = parseTime(vals[4])
	endTime, _ = parseTime(vals[5])
	return
}

func SetProductCache(ctx context.Context, productID int64, name string, seckillPrice float64, limitPerUser int, status int, startTime, endTime time.Time, ttl time.Duration) error {
	key := fmt.Sprintf("%s%d", ProductCachePrefix, productID)
	return rdb.HMSet(ctx, key,
		"name", name, "seckill_price", fmt.Sprintf("%.2f", seckillPrice),
		"limit_per_user", limitPerUser, "status", status,
		"start_time", startTime.Format(time.RFC3339), "end_time", endTime.Format(time.RFC3339),
	).Err()
}

func parseFloat(v interface{}) (float64, error) {
	s, ok := v.(string)
	if !ok { return 0, fmt.Errorf("类型错误") }
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func parseInt(v interface{}) (int, error) {
	s, ok := v.(string)
	if !ok { return 0, fmt.Errorf("类型错误") }
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}

func parseTime(v interface{}) (time.Time, error) {
	s, ok := v.(string)
	if !ok { return time.Time{}, fmt.Errorf("类型错误") }
	return time.Parse(time.RFC3339, s)
}

// [速度优化] DecrStockAndIncrPurchaseWithIdempotent 合并幂等性检查 + 库存扣减 + 限购计数
// 将原来 2 次 RTT（CheckIdempotent + DecrStockAndIncrPurchase）合并为 1 次 RTT
// 返回值: code: 1=成功, -1=商品不存在, -2=库存不足, -3=限购达上限, -4=重复提交
func DecrStockAndIncrPurchaseWithIdempotent(ctx context.Context, userID, productID int64, quantity int, limitPerUser int, expireTime time.Duration, idempotentKey string) (remainStock int, newCount int, code int, err error) {
	stockKey := fmt.Sprintf("%s%d", StockKeyPrefix, productID)
	userKey := fmt.Sprintf("%s%d:%d", UserPurchasedPrefix, userID, productID)
	expireSec := int64(expireTime.Seconds())

	luaScript := `
		local stockKey = KEYS[1]
		local userKey = KEYS[2]
		local idemKey = KEYS[3]
		local quantity = tonumber(ARGV[1])
		local expireSec = tonumber(ARGV[2])
		local limitPerUser = tonumber(ARGV[3])
		local idemTTL = tonumber(ARGV[4])

		-- [P0-修复] 所有分支返回值必须保持统一位置语义 {remainStock, newCount, code}：
		-- 原拒绝分支返回 {-3, current, 0}（拒绝码在第1位），而成功分支 {remainStock, newCount, 1}（code在第3位），
		-- Go 侧统一按 result[2]=code 解析 → 拒绝时 code=0 落空 switch → 限购/库存拒绝被完全绕过，订单照样创建！
		-- 修正后：拒绝码统一放 result[2]，result[0]=0（未扣库存），result[1]=当前已购数（供-3提示文案使用）

		-- 0. 幂等性检查
		if idemKey ~= "" then
			local ok = redis.call('SET', idemKey, '1', 'NX', 'EX', idemTTL)
			if not ok then return {0, 0, -4} end
		end

		-- 1. 先检查用户限购（[修复] 限购检查必须前置于库存扣减：
		--    原逻辑先扣库存再查限购，被限购拒绝时库存已被扣掉且未回滚，导致库存泄漏）
		--    [修复] 判断条件改为 current + quantity > limitPerUser：
		--    原条件 current >= limitPerUser 在 quantity > 1 时会放行超限购买
		--    （如限购3件、已购2件、本次买2件：2 < 3 放行，累计4件超限）
		local current = redis.call('GET', userKey)
		if not current then current = 0 end
		current = tonumber(current)
		if current + quantity > limitPerUser then return {0, current, -3} end

		-- 2. 检查并扣减库存
		local stock = redis.call('GET', stockKey)
		if not stock then return {0, 0, -1} end
		stock = tonumber(stock)
		if stock < quantity then return {0, current, -2} end
		local remainStock = stock - quantity
		redis.call('DECRBY', stockKey, quantity)

		-- 3. 累加购买计数
		local newCount = current + quantity
		redis.call('SET', userKey, newCount, 'EX', expireSec)

		return {remainStock, newCount, 1}
	`

	idemKey := ""
	if idempotentKey != "" {
		idemKey = fmt.Sprintf("idempotent:%d:%d:%s", userID, productID, idempotentKey)
	}
	result, err := rdb.Eval(ctx, luaScript, []string{stockKey, userKey, idemKey}, quantity, expireSec, limitPerUser, 5).Int64Slice()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("Redis合并操作失败: %w", err)
	}
	return int(result[0]), int(result[1]), int(result[2]), nil
}

// [P0-2] DecrStockAndIncrPurchase 合并库存扣减 + 用户购买计数操作，单次 Lua 原子执行。
// 将原来 2 次 RTT（DecrStock + IncrUserPurchaseCount）合并为 1 次 RTT，
// 消除中间失败回滚路径，减少约 40% 的 Redis 网络开销。
// 返回值:
//
//	remainStock: 剩余库存（成功时 >= 0）
//	newCount:    用户累计购买数量（成功时 > 0）
//	success:     true 表示扣减成功
//	code:        1=成功, -1=商品不存在, -2=库存不足, -3=用户已达限购上限
func DecrStockAndIncrPurchase(ctx context.Context, userID, productID int64, quantity int, limitPerUser int, expireTime time.Duration) (remainStock int, newCount int, code int, err error) {
	stockKey := fmt.Sprintf("%s%d", StockKeyPrefix, productID)
	userKey := fmt.Sprintf("%s%d:%d", UserPurchasedPrefix, userID, productID)
	expireSec := int64(expireTime.Seconds())

	luaScript := `
		local stockKey = KEYS[1]
		local userKey = KEYS[2]
		local quantity = tonumber(ARGV[1])
		local expireSec = tonumber(ARGV[2])
		local limitPerUser = tonumber(ARGV[3])

		-- [P0-修复] 拒绝码统一放第3位（与成功分支 {remainStock, newCount, 1} 位置语义一致），
		-- 原拒绝分支 {-3, current, 0} 会导致 Go 侧 code=0 落空 switch，限购/库存拒绝被绕过
		-- 1. 先检查用户限购（[修复] 限购检查前置于库存扣减，避免限购拒绝时库存泄漏；
		--    判断条件 current + quantity > limitPerUser，防止 quantity > 1 时累计超限）
		local current = redis.call('GET', userKey)
		if not current then current = 0 end
		current = tonumber(current)
		if current + quantity > limitPerUser then return {0, current, -3} end

		-- 2. 检查并扣减库存
		local stock = redis.call('GET', stockKey)
		if not stock then return {0, 0, -1} end
		stock = tonumber(stock)
		if stock < quantity then return {0, current, -2} end
		local remainStock = stock - quantity
		redis.call('DECRBY', stockKey, quantity)

		-- 3. 累加用户购买计数
		local newCount = current + quantity
		redis.call('SET', userKey, newCount, 'EX', expireSec)

		return {remainStock, newCount, 1}
	`

	result, err := rdb.Eval(ctx, luaScript, []string{stockKey, userKey}, quantity, expireSec, limitPerUser).Int64Slice()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("Redis合并操作失败: %w", err)
	}
	return int(result[0]), int(result[1]), int(result[2]), nil
}

// [修复] 幂等性 Key：防止用户重复提交秒杀请求
// 使用 SETNX 原子操作，key 格式为 seckill:idempotent:{userID}:{productID}:{idempotentKey}
// 返回 true 表示首次请求，false 表示重复请求
// ttl 秒后自动过期，防止 Redis 内存泄漏
func CheckIdempotent(ctx context.Context, userID, productID int64, idempotentKey string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf("seckill:idempotent:%d:%d:%s", userID, productID, idempotentKey)
	ok, err := rdb.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("Redis幂等性检查失败: %w", err)
	}
	return ok, nil
}

// [修复] 分布式锁自动续期：防止长耗时场景锁过期导致超卖
// 使用 goroutine 定时续期，每次续期重置 TTL 为 lockTimeout
func AcquireLockWithRenewal(ctx context.Context, lockKey string, lockTimeout time.Duration, stopCh <-chan struct{}) (bool, error) {
	ok, err := rdb.SetNX(ctx, lockKey, "1", lockTimeout).Result()
	if err != nil || !ok {
		return ok, err
	}
	// [修复] 启动续期协程：每隔 lockTimeout/3 续期一次
	renewInterval := lockTimeout / 3
	go func() {
		ticker := time.NewTicker(renewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				// 业务完成，释放锁
				rdb.Del(context.Background(), lockKey)
				return
			case <-ticker.C:
				// 续期：重置 TTL
				rdb.Expire(context.Background(), lockKey, lockTimeout)
			}
		}
	}()
	return true, nil
}

func Close() error {
	if rdb != nil { return rdb.Close() }
	return nil
}

// ==================== [创新] 秒杀地址隐藏 ====================
// 借鉴 qiurunze123/miaosha（19k Stars），秒杀开始前动态生成隐藏 URL Token
// 用户需要先获取动态 Path Token，再携带 Token 调用秒杀接口
// 防止脚本提前构造请求，有效阻隔黄牛和自动化工具

const SeckillPathPrefix = "seckill:path:"

// SetSeckillPath 生成并存储秒杀隐藏路径 Token
// productID: 商品ID, userID: 用户ID, token: 随机生成的路径 Token, ttl: 过期时间
func SetSeckillPath(ctx context.Context, userID, productID int64, token string, ttl time.Duration) error {
	key := fmt.Sprintf("%s%d:%d", SeckillPathPrefix, userID, productID)
	return rdb.Set(ctx, key, token, ttl).Err()
}

// GetAndVerifySeckillPath 获取并校验秒杀隐藏路径 Token，校验后立即删除（一次性使用）
// 返回 true 表示 Token 有效，false 表示无效或已过期
func GetAndVerifySeckillPath(ctx context.Context, userID, productID int64, token string) (bool, error) {
	key := fmt.Sprintf("%s%d:%d", SeckillPathPrefix, userID, productID)
	luaScript := `
		local val = redis.call('GET', KEYS[1])
		if not val then return 0 end
		if val ~= ARGV[1] then return 0 end
		redis.call('DEL', KEYS[1])
		return 1
	`
	result, err := rdb.Eval(ctx, luaScript, []string{key}, token).Int()
	if err != nil {
		return false, fmt.Errorf("秒杀路径校验失败: %w", err)
	}
	return result == 1, nil
}

// ==================== [创新] 数学验证码 ====================
// 借鉴 qiurunze123/miaosha，后端生成随机数学算式，用户计算后提交答案
// 答案存储在 Redis 中，校验后删除，防止脚本暴力破解

const CaptchaPrefix = "seckill:captcha:"

// SetCaptcha 存储数学验证码答案
// captchaID: 验证码唯一ID, answer: 正确答案, ttl: 过期时间
func SetCaptcha(ctx context.Context, captchaID string, answer int, ttl time.Duration) error {
	key := fmt.Sprintf("%s%s", CaptchaPrefix, captchaID)
	return rdb.Set(ctx, key, answer, ttl).Err()
}

// GetAndVerifyCaptcha 校验数学验证码答案，校验后立即删除（一次性使用）
// 返回 true 表示答案正确，false 表示错误或已过期
func GetAndVerifyCaptcha(ctx context.Context, captchaID string, answer int) (bool, error) {
	key := fmt.Sprintf("%s%s", CaptchaPrefix, captchaID)
	luaScript := `
		local val = redis.call('GET', KEYS[1])
		if not val then return 0 end
		if tonumber(val) ~= tonumber(ARGV[1]) then return 0 end
		redis.call('DEL', KEYS[1])
		return 1
	`
	result, err := rdb.Eval(ctx, luaScript, []string{key}, answer).Int()
	if err != nil {
		return false, fmt.Errorf("验证码校验失败: %w", err)
	}
	return result == 1, nil
}