// Package websocket 实现 WebSocket 实时推送服务
// 轻量级设计：连接池 + 心跳检测 + 分级消息广播
// 支持：秒杀结果推送、订单状态变更、系统告警通知
// 连接上限 5000，心跳 30s，自动清理僵尸连接
package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// ===== 消息类型定义 =====

// MessageType 消息类型
type MessageType string

const (
	MsgSeckillResult MessageType = "seckill_result" // 秒杀结果
	MsgOrderStatus   MessageType = "order_status"   // 订单状态变更
	MsgSystemAlert   MessageType = "system_alert"   // 系统告警
	MsgSeckillStart  MessageType = "seckill_start"  // 秒杀活动开始
	MsgSeckillEnd    MessageType = "seckill_end"    // 秒杀活动结束
	MsgHeartbeat     MessageType = "heartbeat"      // 心跳
)

// PushMessage WebSocket 推送消息
type PushMessage struct {
	Type      MessageType `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

// ===== 连接管理 =====

// Client WebSocket 客户端连接
type Client struct {
	ID       int64
	UserID   int64
	Conn     *websocket.Conn
	Send     chan []byte
	Hub      *Hub
	LastPing time.Time
	mu       sync.Mutex
}

const (
	// 写入超时
	writeWait = 10 * time.Second
	// 读取 pong 超时
	pongWait = 60 * time.Second
	// 心跳间隔（必须小于 pongWait）
	pingPeriod = 30 * time.Second
	// 最大消息大小
	maxMessageSize = 4096
	// 发送缓冲区
	sendBufSize = 256
	// 最大连接数
	maxConnections = 5000
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 由 CORS 中间件控制
	},
}

// Hub WebSocket 连接中心
type Hub struct {
	clients    map[int64]*Client // 用户级连接（一个用户一个连接）
	connCount  int64             // 原子计数器
	register   chan *Client
	unregister chan *Client
	broadcast  chan *PushMessage
	mu         sync.RWMutex
}

// NewHub 创建 WebSocket 连接中心
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[int64]*Client),
		register:   make(chan *Client, 256),
		unregister: make(chan *Client, 256),
		broadcast:  make(chan *PushMessage, 1024),
	}
}

// Run 启动 Hub 事件循环
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			// 同一用户踢掉旧连接
			if old, ok := h.clients[client.UserID]; ok {
				close(old.Send)
				old.Conn.Close()
			}
			h.clients[client.UserID] = client
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if c, ok := h.clients[client.UserID]; ok && c == client {
				delete(h.clients, client.UserID)
				close(client.Send)
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("[WebSocket] 消息序列化失败: %v", err)
				continue
			}
			h.mu.RLock()
			for _, client := range h.clients {
				select {
				case client.Send <- data:
				default:
					// 发送缓冲区满，跳过
					go h.unregisterClient(client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// HandleWebSocket 处理 WebSocket 升级请求
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request, userID int64) {
	// 连接数限制
	if atomic.LoadInt64(&h.connCount) >= maxConnections {
		http.Error(w, "WebSocket 连接数已达上限", http.StatusServiceUnavailable)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] 升级失败: %v", err)
		return
	}

	client := &Client{
		ID:       atomic.AddInt64(&h.connCount, 1),
		UserID:   userID,
		Conn:     conn,
		Send:     make(chan []byte, sendBufSize),
		Hub:      h,
		LastPing: time.Now(),
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

// writePump 向客户端写入消息
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump 读取客户端消息（目前仅处理 pong）
func (c *Client) readPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
		atomic.AddInt64(&c.Hub.connCount, -1)
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.mu.Lock()
		c.LastPing = time.Now()
		c.mu.Unlock()
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// unregisterClient 清理客户端连接
func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	if c, ok := h.clients[client.UserID]; ok && c == client {
		delete(h.clients, client.UserID)
		close(client.Send)
		client.Conn.Close()
		atomic.AddInt64(&h.connCount, -1)
	}
	h.mu.Unlock()
}

// ===== 推送 API =====

// PushToUser 推送给指定用户
func (h *Hub) PushToUser(userID int64, msgType MessageType, data interface{}) {
	h.mu.RLock()
	client, ok := h.clients[userID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	msg := PushMessage{
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now().UnixMilli(),
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}

	select {
	case client.Send <- payload:
	default:
		// 缓冲区满，丢弃消息
	}
}

// Broadcast 全量广播
func (h *Hub) Broadcast(msgType MessageType, data interface{}) {
	h.broadcast <- &PushMessage{
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now().UnixMilli(),
	}
}

// PushToRole 推送给指定角色用户
func (h *Hub) PushToRole(role string, msgType MessageType, data interface{}, roleChecker func(userID int64) bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	msg := PushMessage{
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now().UnixMilli(),
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for userID, client := range h.clients {
		if roleChecker(userID) {
			select {
			case client.Send <- payload:
			default:
			}
		}
	}
}

// Stats 获取连接统计
func (h *Hub) Stats() map[string]int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return map[string]int{
		"active_connections": len(h.clients),
		"total_connections":  int(atomic.LoadInt64(&h.connCount)),
		"max_connections":    maxConnections,
	}
}
