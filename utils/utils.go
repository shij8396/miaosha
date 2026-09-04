package utils

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	TraceID string      `json:"trace_id,omitempty"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{Code: 200, Message: "success", Data: data, TraceID: GetTraceID(c)})
}
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{Code: 200, Message: message, Data: data, TraceID: GetTraceID(c)})
}
func Error(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, Response{Code: code, Message: message, TraceID: GetTraceID(c)})
}
func InternalError(c *gin.Context, message string) { Error(c, 500, message) }
func BadRequest(c *gin.Context, message string)    { Error(c, 400, message) }
func Unauthorized(c *gin.Context, message string)  { Error(c, 401, message) }
func Forbidden(c *gin.Context, message string)     { Error(c, 403, message) }
func NotFound(c *gin.Context, message string)      { Error(c, 404, message) }

// [修复] JWT Claims 增加 Role 字段，用于管理员白名单等场景识别用户角色
type JWTClaims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	secret      []byte
	expireHours int
	issuer      string
}

func NewJWTManager(secret string, expireHours int, issuer string) *JWTManager {
	return &JWTManager{secret: []byte(secret), expireHours: expireHours, issuer: issuer}
}

// [修复] GenerateToken 增加 role 参数，将用户角色写入 JWT，便于中间件和控制器识别管理员
func (m *JWTManager) GenerateToken(userID int64, username, role string) (string, error) {
	now := time.Now()
	claims := &JWTClaims{
		UserID: userID, Username: username, Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: m.issuer, IssuedAt: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(m.expireHours) * time.Hour)),
			Subject:   fmt.Sprintf("%d", userID),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *JWTManager) ParseToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("非预期的签名方法: %v", token.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("JWT解析失败: %w", err)
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("无效的JWT Token")
	}
	return claims, nil
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("密码加密失败: %w", err)
	}
	return string(bytes), nil
}

func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

const (
	epoch         = int64(1704067200000)
	workerIDBits  = 10
	sequenceBits  = 12
	workerIDMax   = -1 ^ (-1 << workerIDBits)
	sequenceMax   = -1 ^ (-1 << sequenceBits)
	timeShift     = workerIDBits + sequenceBits
	workerIDShift = sequenceBits
)

type Snowflake struct {
	mu        sync.Mutex
	workerID  int64
	sequence  int64
	lastStamp int64
}

func NewSnowflake(workerID int64) (*Snowflake, error) {
	if workerID < 0 || workerID > workerIDMax {
		return nil, fmt.Errorf("workerID必须在0到%d之间", workerIDMax)
	}
	return &Snowflake{workerID: workerID}, nil
}

func (s *Snowflake) NextID() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	if now < s.lastStamp {
		return 0, fmt.Errorf("时钟回拨，拒绝生成ID")
	}
	if now == s.lastStamp {
		s.sequence = (s.sequence + 1) & sequenceMax
		if s.sequence == 0 {
			for now <= s.lastStamp {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		s.sequence = 0
	}
	s.lastStamp = now
	return (now-epoch)<<timeShift | (s.workerID << workerIDShift) | s.sequence, nil
}

// [速度优化] IDGenerator 接口，支持 Snowflake 和 BufferedSnowflake 两种实现
// BufferedSnowflake 通过预生成通道消除每次调用 NextID 的 mutex 锁竞争
type IDGenerator interface {
	NextID() (int64, error)
}

// [速度优化] BufferedSnowflake 预生成 ID 到缓冲通道，消除高并发 mutex 竞争
// 后台 goroutine 持续填充 channel，NextID() 直接从 channel 取，无需锁定
type BufferedSnowflake struct {
	ch   chan int64
	sf   *Snowflake
	done chan struct{}
}

func NewBufferedSnowflake(sf *Snowflake, bufferSize int) *BufferedSnowflake {
	bs := &BufferedSnowflake{
		ch:   make(chan int64, bufferSize),
		sf:   sf,
		done: make(chan struct{}),
	}
	go bs.fill()
	return bs
}

func (bs *BufferedSnowflake) fill() {
	for {
		id, err := bs.sf.NextID()
		if err != nil {
			// 时钟回拨等异常跳过，等待下次生成
			time.Sleep(time.Millisecond)
			continue
		}
		select {
		case bs.ch <- id:
		case <-bs.done:
			return
		}
	}
}

func (bs *BufferedSnowflake) NextID() (int64, error) {
	select {
	case id := <-bs.ch:
		return id, nil
	default:
		// channel 为空时回退直接生成（极端高并发下缓冲区耗尽）
		return bs.sf.NextID()
	}
}

func (bs *BufferedSnowflake) Close() {
	close(bs.done)
}

func GenerateOrderNo(g IDGenerator) (string, error) {
	id, err := g.NextID()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("MS%d", id), nil
}

const TraceIDKey = "trace_id"

func GetTraceID(c *gin.Context) string {
	traceID, _ := c.Get(TraceIDKey)
	if s, ok := traceID.(string); ok {
		return s
	}
	return ""
}

func GenerateTraceID() string {
	now := time.Now().UnixNano()
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d", now)))
	return fmt.Sprintf("%x", hash[:16])
}

// [修复] ClampPageSize 分页上限限制：防止客户端传入超大 pageSize 导致数据库压力
// 默认最大 100 条/页，导出场景使用 ExtendedPageSize 限制（10000）
const MaxPageSize = 100
const MaxExportPageSize = 10000

func ClampPageSize(pageSize int) int {
	if pageSize <= 0 {
		return 10
	}
	if pageSize > MaxPageSize {
		return MaxPageSize
	}
	return pageSize
}

func DingTalkSign(secret string, timestamp int64) (string, error) {
	msg := fmt.Sprintf("%d\n%s", timestamp, secret)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

// [修复] 禁用 HTML 转义，确保中文正常显示不变成 \uXXXX 乱码
func ToJSON(v interface{}) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.Encode(v)
	// [修复] TrimSpace 去除 json.NewEncoder 自动追加的换行符
	return strings.TrimSpace(buf.String())
}

func FromJSON(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}

func GetOrderTableName(userID int64, shardCount int) string {
	return fmt.Sprintf("t_order_%d", userID%int64(shardCount))
}
