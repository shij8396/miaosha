<p align="center">
  <h1 align="center">⚡ 企业级分布式秒杀系统</h1>
  <p align="center">
    Go 1.26 · Vue 3 · Redis Cluster · RabbitMQ · Sentinel · Prometheus
  </p>
</p>

<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go" alt="Go"></a>
  <a href="https://vuejs.org/"><img src="https://img.shields.io/badge/Vue-3.4+-4FC08D?style=flat-square&logo=vue.js" alt="Vue"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-green?style=flat-square" alt="License"></a>
  <a href="https://www.docker.com/"><img src="https://img.shields.io/badge/Docker-Supported-2496ED?style=flat-square&logo=docker" alt="Docker"></a>
  <a><img src="https://img.shields.io/badge/PRs-Welcome-brightgreen?style=flat-square" alt="PRs Welcome"></a>
  <img src="https://img.shields.io/github/stars/shij8396/miaosha?style=flat-square" alt="Stars">
  <img src="https://img.shields.io/github/forks/shij8396/miaosha?style=flat-square" alt="Forks">
</p>

---

##  项目亮点

> **单机 1600 QPS，8000 并发 100% 成功率，零超卖。** 从零手写六层企业级架构，不依赖任何秒杀框架。

<table>
<tr>
<td width="50%">

###  架构能力
-  **六层企业架构**：接入层 → 服务集群 → 流量防护 → 中间件 → 数据高可用 → 可观测运维
-  **Redis Cluster 库存预热**：上架即预热，秒杀全程不查 MySQL
-  **Lua 原子扣减**：库存扣减 + 幂等校验 + 限购计数三合一，一次 RTT
-  **Singleflight 请求合并**：256 分片，50ms 窗口去重
-  **Snowflake 分布式 ID**：1024 缓冲通道，无锁获取

</td>
<td width="50%">

###  工程实践
-  **Sentinel 全局限流**：QPS + 热点参数 + 管理员白名单豁免
-  **分层熔断降级**：慢调用 / 异常比例 / 异常数，差异化提示
-  **TTL+DLX 延迟订单**：30 分钟未支付自动取消，归还库存
-  **定时对账**：Redis-MySQL 库存 5 分钟自动修复
-  **AI 异常检测**：滑动窗口 + Z-Score，识别黄牛脚本

</td>
</tr>
</table>

---

##  性能基准

| 并发 | QPS | P50 | P99 | 成功率 |
|:---:|:---:|:---:|:---:|:---:|
| 100 | 806 | 97ms | 123ms | 100% |
| 500 | 850 | 456ms | 586ms | 100% |
| 1000 | 606 | 939ms | 1601ms | 100% |
| 2000 | 1357 | 875ms | 1421ms | 100% |
| 3000 | 1407 | 1204ms | 2087ms | 100% |
| **5000** | **1614** | 1702ms | 2967ms | **100%** |
| 8000 | 1458 | 3068ms | 5346ms | 100% |

>   Windows 单机，i7-12700H，32GB RAM。峰值 QPS=1614，拐点 8000 并发（P99 突破 5s）。

---

##  架构概览

```
                    ┌──────────────┐
                    │   用户请求    │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │  Nginx LB    │  ← 负载均衡 + WAF
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
        ┌─────▼─────┐┌────▼─────┐┌────▼─────┐
        │ Gin 实例 1││Gin 实例 2││Gin 实例 3│  ← 服务集群
        └─────┬─────┘└────┬─────┘└────┬─────┘
              │            │            │
        ┌─────▼────────────▼────────────▼─────┐
        │         Sentinel-Go 流量防护        │
        │   限流 · 熔断 · 降级 · 热点参数     │
        └─────┬────────────┬────────────┬─────┘
              │            │            │
    ┌─────────▼──┐ ┌───────▼──┐ ┌──────▼─────┐
    │   Redis    │ │ RabbitMQ │ │   Kafka    │
    │  Cluster   │ │ TTL+DLX  │ │  行为追踪  │
    └─────────┬──┘ └───────┬──┘ └──────┬─────┘
              │            │            │
        ┌─────▼────────────▼────────────▼─────┐
        │         MySQL 8.0 主从 + 16分表     │
        │           数据高可用层              │
        └────────────────┬────────────────────┘
                         │
        ┌────────────────▼────────────────────┐
        │  Prometheus · Grafana · ELK · Jaeger │
        │       可观测运维 + 钉钉告警          │
        └─────────────────────────────────────┘
```

---

##  快速开始

### 一键部署（Docker）

```bash
# 克隆项目
git clone https://github.com/shij8396/miaosha.git
cd miaosha

# 配置环境变量
cp .env.example .env
# 编辑 .env，修改密码

# 启动全栈 16 个服务
docker compose up -d

# 访问
#  前端:      http://localhost
#  Grafana:   http://localhost:3000 (admin/admin)
#  RabbitMQ:  http://localhost:15672
```

### 本地开发

```bash
# 启动中间件
docker compose up -d mysql-master redis rabbitmq kafka etcd

# 后端
go mod tidy && go run ./cmd/

# 前端（新终端）
cd frontend && npm install && npm run dev
```

### 压测

```bash
go run ./stress_test/cmd/setup/    # 初始化测试数据
go run ./stress_test/cmd/          # 常规压测
go run ./stress_test/cmd/limit/    # 极限压测
```

---

##  技术栈

| 层级 | 技术选型 | 说明 |
|:---|:---|:---|
| 后端框架 | **Go 1.26 + Gin** | 高性能 HTTP 框架 |
| 数据库 | **MySQL 8.0** | 主从复制 + 读写分离 + 16 分表 |
| 缓存 | **Redis Cluster** | 库存预热 + Lua 原子操作 |
| 消息队列 | **RabbitMQ** | TTL+DLX 延迟订单 + ChannelPool |
| 流处理 | **Kafka** | 行为追踪 + 异步消费 |
| 注册中心 | **Etcd** | 服务注册发现 |
| 流量防护 | **Sentinel-Go** | 限流 + 熔断 + 热点参数 |
| 可观测性 | **Prometheus + Grafana + ELK + Jaeger** | 全链路监控 |
| 告警 | **钉钉机器人** | 异常实时推送 |
| 前端 | **Vue 3 + Vite 5 + Element Plus + Pinia + ECharts** | SPA |
| 部署 | **Docker Compose + Nginx** | 一键编排 |

---

##  项目结构

```
miaosha/
├── cmd/                  # 入口
├── config/               # 多环境配置（dev / docker / prod）
├── controller/           # 控制器层（9 个）
├── service/              # 业务逻辑层（9 个）
├── dao/                  # 数据访问层
├── model/                # 数据模型
├── redis/                # Redis 操作（Lua 脚本 + 分布式锁）
├── mq/                   # RabbitMQ（ChannelPool + 消费者）
├── kafka/                # Kafka 生产者/消费者
├── middleware/            # 中间件（JWT 认证 / HMAC签名 / 限流 / 黑名单 / 追踪）
├── sentinel/             # Sentinel 流量防护
├── singleflight/         # 请求合并引擎（256 分片）
├── detector/             # AI 异常检测引擎
├── websocket/            # WebSocket 实时推送
├── monitor/              # Prometheus 指标
├── cron/                 # 定时任务（对账）
├── log/                  # 日志（Zap）
├── utils/                # 工具（Snowflake / 钉钉 / 错误码）
├── frontend/             # Vue 3 前端
├── stress_test/          # 压测工具
├── deploy/               # 部署配置（Nginx/Prometheus/Grafana/Alertmanager）
├── scripts/              # SQL 初始化脚本
├── docker-compose.yml    # 容器编排
└── Dockerfile            # 应用镜像
```

---

##  核心设计详解

<details>
<summary><b>  秒杀流程时序图</b></summary>

```
用户 → Nginx → Gin → Sentinel(限流) → Singleflight(去重)
                                        ↓
                              Redis Lua(库存扣减+幂等+限购)
                                        ↓
                              RabbitMQ(异步下单) → MySQL(持久化)
                                        ↓
                              WebSocket(实时推送结果)
```
</details>

<details>
<summary><b>️ 延迟订单自动取消流程</b></summary>

```
下单成功 → 发送延迟消息(TTL 30min) → 过期进入 DLX
                                        ↓
                              消费者检查订单状态
                              ├─ 已支付 → 忽略
                              └─ 未支付 → 关闭订单 + 归还库存
```
</details>

<details>
<summary><b>  分层熔断降级策略</b></summary>

| 层级 | 触发条件 | 降级策略 | 提示文案 |
|:---|:---|:---|:---|
| 限流层 | QPS 超阈值 | 拒绝请求 | "活动太火爆，请稍后再试" |
| 库存层 | Redis 库存不足 | 返回售罄 | "已售罄，下次早点来" |
| 中间件层 | Redis/MQ 故障 | 数据库兜底 | "系统繁忙，请稍后重试" |
| 服务层 | 异常率过高 | 快速失败 | "服务暂时不可用" |
</details>

---

##  贡献指南

欢迎所有形式的贡献！Star、Fork、Issue、PR 都是对项目的支持。

### 如何贡献

1. **Fork** 本项目
2. 创建特性分支：`git checkout -b feature/amazing-feature`
3. 提交修改：`git commit -m 'feat: add amazing feature'`
4. 推送分支：`git push origin feature/amazing-feature`
5. 创建 **Pull Request**

### 提交规范

本项目使用 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

- `feat:` 新功能
- `fix:` 修复 Bug
- `docs:` 文档更新
- `refactor:` 代码重构
- `perf:` 性能优化
- `test:` 测试相关
- `chore:` 构建/工具变动

详见 [CONTRIBUTING.md](CONTRIBUTING.md)

---

##  学习路线

如果你正在学习高并发系统设计，建议按以下顺序阅读代码：

1.  `cmd/main.go` — 了解系统启动流程和依赖注入
2.  `config/` — 理解多环境配置管理
3.  `model/` — 熟悉数据模型设计
4.  `controller/seckill_controller.go` — 秒杀接口入口
5.  `service/seckill_service.go` — 核心秒杀逻辑
6.  `redis/redis.go` — Lua 原子脚本
7.  `singleflight/` — 请求合并机制
8.  `mq/` — RabbitMQ 延迟队列
9.  `sentinel/` — 流量防护配置
10. `detector/` — AI 异常检测

---

##  常见问题

<details>
<summary><b>为什么选择 Go 而不是 Java？</b></summary>
Go 原生支持高并发（goroutine），编译为单一二进制文件部署简单，内存占用低。适合秒杀这类 IO 密集型场景。
</details>

<details>
<summary><b>为什么用 RabbitMQ 而不是 Kafka 做订单队列？</b></summary>
秒杀订单需要 TTL+DLX 实现 30 分钟延迟取消，RabbitMQ 对此有原生支持。Kafka 在本项目中用于用户行为追踪等流式场景。
</details>

<details>
<summary><b>如何保证不会超卖？</b></summary>
三层防护：① Lua 原子脚本扣减 Redis 库存（单线程执行）；② 数据库唯一索引兜底；③ 定时对账修复。
</details>

---

##  许可证

本项目采用 [MIT License](LICENSE) 开源，欢迎自由使用、修改和分发。

---

##  致谢

如果这个项目对你有帮助，请给个 ⭐ Star 支持一下！

<p align="center">
  <a href="https://github.com/shij8396/miaosha">
    <img src="https://img.shields.io/github/stars/shij8396/miaosha?style=social" alt="Star">
  </a>
  <a href="https://github.com/shij8396/miaosha/fork">
    <img src="https://img.shields.io/github/forks/shij8396/miaosha?style=social" alt="Fork">
  </a>
</p>