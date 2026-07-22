# 企业级分布式秒杀系统

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3.4+-4FC08D?logo=vue.js)](https://vuejs.org/)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-Supported-2496ED?logo=docker)](https://www.docker.com/)

基于 Go 1.26 构建的六层企业级秒杀系统，从零实现全链路高并发架构。单机实测 **1600 QPS、8000 并发 100% 成功率**，零超卖。

<p align="center">
  <img src="https://trae-api-cn.mchost.guru/api/ide/v1/text_to_image?prompt=A%20modern%20dashboard%20interface%20for%20a%20flash%20sale%20(seckill)%20system%2C%20showing%20real-time%20QPS%20metrics%2C%20order%20statistics%2C%20and%20stock%20levels%20in%20a%20dark%20theme%20with%20purple%20and%20blue%20accents%2C%20professional%20enterprise%20UI%20design&image_size=landscape_16_9" width="800" alt="系统截图">
</p>

## 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│                      接入层 Gateway                          │
│              Nginx (负载均衡) + CDN/WAF 前置                  │
├─────────────────────────────────────────────────────────────┤
│                     服务集群 (Gin)                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │ Instance │ │ Instance │ │ Instance │ │   ...    │      │
│  │    1     │ │    2     │ │    3     │ │          │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
├─────────────────────────────────────────────────────────────┤
│                  流量防护 Sentinel-Go                         │
│      全局限流 + 热点参数限流 + 熔断降级 + 系统自适应         │
├─────────────────────────────────────────────────────────────┤
│                   中间件集群                                  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐      │
│  │  Redis   │ │ RabbitMQ │ │  Kafka   │ │   Etcd   │      │
│  │ Cluster  │ │  TTL+DLX │ │ 行为追踪 │ │ 注册发现  │      │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘      │
├─────────────────────────────────────────────────────────────┤
│                数据高可用 MySQL 8.0                            │
│         主从复制 + 读写分离 + 16 分表                          │
├─────────────────────────────────────────────────────────────┤
│               可观测运维                                      │
│  Prometheus + Grafana + ELK + Jaeger + 钉钉告警              │
└─────────────────────────────────────────────────────────────┘
```

## 核心特性

### 高并发秒杀
- **Redis Cluster 库存预热** — 商品上架自动加载库存到 Redis，秒杀全程不查 MySQL
- **Lua 原子脚本** — 库存扣减 + 幂等校验 + 限购计数三合一，一次 RTT 完成
- **Singleflight 请求合并** — 256 分片并发合并，相同用户 50ms 内重复请求自动去重
- **Snowflake 预生成** — 1024 缓冲通道，无锁获取分布式 ID

### 流量防护
- **Sentinel 全局限流** — QPS 阈值限流 + 热点参数限流，管理员白名单豁免
- **分层熔断降级** — 慢调用/异常比例/异常数三层熔断，差异化提示文案
- **JWT + RBAC** — 运营/风控/监控三角色权限隔离，HMAC-SHA256 接口签名

### 数据一致性
- **幂等性校验** — 5 秒去重窗口，SETNX 防重复下单
- **TTL+DLX 延迟订单** — 30 分钟未支付自动取消，归还库存
- **定时对账** — Redis-MySQL 库存 5 分钟一次自动对账修复

### 可观测性
- **Prometheus + Grafana** — PV/UV、QPS、订单量、慢接口 TOP 排行实时监控
- **ELK 日志** — JSON 结构化日志，Filebeat 采集
- **Jaeger 全链路追踪** — 请求端到端耗时分布
- **钉钉告警** — 错误率/QPS/队列积压超阈值自动推送

### AI 智能
- **实时异常检测** — 滑动窗口 + Z-Score 多维度行为分析，识别黄牛/脚本
- **WebSocket 实时推送** — 秒杀结果毫秒级推送，用户无需刷新

## 性能基准

| 并发级别 | QPS | P50 | P99 | 成功率 |
|:---:|:---:|:---:|:---:|:---:|
| 100 | 806 | 97ms | 123ms | 100% |
| 500 | 850 | 456ms | 586ms | 100% |
| 1000 | 606 | 939ms | 1601ms | 100% |
| 2000 | 1357 | 875ms | 1421ms | 100% |
| 3000 | 1407 | 1204ms | 2087ms | 100% |
| 5000 | **1614** | 1702ms | 2967ms | 100% |
| 8000 | 1458 | 3068ms | 5346ms | 100% |

> 测试环境：Windows 单机，i7-12700H，32GB RAM，所有中间件本地运行。峰值 QPS=1614，拐点 8000 并发（P99 突破 5s）。

## 快速开始

### 前置条件

- Go 1.26+
- Docker & Docker Compose
- Node.js 18+（前端开发）

### Docker 一键启动（推荐）

```bash
# 1. 配置环境变量
cp .env.example .env
# 编辑 .env，修改密码

# 2. 启动全栈（16 个服务）
docker compose up -d

# 3. 查看状态
docker compose ps

# 4. 访问
# 前端: http://localhost
# 监控: http://localhost:3000 (admin/admin)
# 文档: http://localhost:8080/swagger/index.html
```

### 本地开发启动

```bash
# 1. 启动中间件
docker compose up -d mysql-master redis rabbitmq kafka etcd

# 2. 安装 Go 依赖
go mod tidy

# 3. 修改 config/config.yaml 中的连接信息

# 4. 启动后端
go run ./cmd/

# 5. 启动前端（新终端）
cd frontend
npm install
npm run dev
```

### 压测

```bash
# 初始化测试数据
go run ./stress_test/cmd/setup/

# 常规压测（100/500/1000/5000 并发）
go run ./stress_test/cmd/

# 极限压测（递增并发直到拐点）
go run ./stress_test/cmd/limit/
```

## 项目结构

```
miaosha/
├── cmd/                    # 入口
│   └── main.go
├── config/                 # 配置文件
│   ├── config.yaml         # 本地开发
│   ├── config.docker.yaml  # Docker 部署
│   └── config.go           # 配置结构体
├── controller/             # 控制器层（9 个）
├── service/                # 业务逻辑层（9 个）
├── dao/                    # 数据访问层
├── model/                  # 数据模型
├── redis/                  # Redis 操作（Lua 脚本 + 分布式锁）
├── mq/                     # RabbitMQ（ChannelPool + 消费者）
├── kafka/                  # Kafka 生产者/消费者
├── middleware/              # 中间件（认证/签名/限流/黑名单/追踪）
├── sentinel/               # Sentinel 流量防护
├── singleflight/           # 请求合并引擎（256 分片）
├── detector/               # AI 异常检测引擎
├── websocket/              # WebSocket 实时推送
├── monitor/                # Prometheus 指标
├── cron/                   # 定时任务（对账）
├── log/                    # 日志（Zap）
├── utils/                  # 工具（Snowflake/钉钉/错误码）
├── frontend/               # Vue 3 前端
├── stress_test/            # 压测工具
├── deploy/                 # 部署配置（Nginx/Prometheus/Grafana/Alertmanager）
├── scripts/                # SQL 初始化脚本
├── docker-compose.yml      # 容器编排
├── Dockerfile              # 应用镜像
└── .env.example            # 环境变量模板
```

## 技术栈

| 层级 | 技术 |
|:---|:---|
| 后端框架 | Go 1.26 + Gin |
| ORM | GORM + MySQL 8.0（主从 + 16 分表） |
| 缓存 | Redis Cluster（库存预热 + Lua 原子操作） |
| 消息队列 | RabbitMQ（TTL+DLX 延迟订单 + ChannelPool） |
| 流处理 | Kafka（行为追踪 + 异步消费者） |
| 注册中心 | Etcd（服务注册发现） |
| 流量防护 | Sentinel-Go（限流 + 熔断 + 热点参数） |
| 监控 | Prometheus + Grafana + ELK + Jaeger |
| 告警 | 钉钉机器人 |
| 前端 | Vue 3 + Vite 5 + Element Plus + Pinia + ECharts |
| 部署 | Docker Compose + Nginx |

## 贡献

欢迎提交 Issue 和 Pull Request。

1. Fork 本项目
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交修改 (`git commit -m 'Add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

## 许可证

[MIT](LICENSE)