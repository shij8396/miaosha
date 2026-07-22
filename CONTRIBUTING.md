# 贡献指南

感谢你对企业级分布式秒杀系统的关注！所有形式的贡献都欢迎。

## 行为准则

请保持尊重和建设性的交流。我们致力于为所有人提供一个友好的协作环境。

## 我能做什么？

| 贡献方式 | 说明 |
|:---|:---|
|   Star | 给项目点赞，让更多人看到 |
|   Fork | 复刻项目到自己仓库，自由修改 |
|   Issue | 报告 Bug、提出新功能建议 |
|   PR | 提交代码改进 |
|   Review | 审查其他人的 PR |
|   文档 | 完善文档、翻译、教程 |
|   讨论 | 分享你的想法和经验 |

## 快速开始

```bash
# 1. Fork 本项目
# 2. 克隆你的 Fork
git clone https://github.com/YOUR_USERNAME/miaosha.git
cd miaosha

# 3. 添加上游仓库
git remote add upstream https://github.com/shij8396/miaosha.git

# 4. 创建特性分支
git checkout -b feature/your-feature-name

# 5. 启动开发环境
docker compose up -d mysql-master redis rabbitmq kafka etcd
go mod tidy && go run ./cmd/
```

## 开发规范

### 提交信息

使用 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/) 规范：

```
<type>(<scope>): <description>

[optional body]
```

**Type 类型：**

| Type | 说明 |
|:---|:---|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 文档更新 |
| `style` | 代码格式（不影响运行） |
| `refactor` | 重构（不新增功能，不修复 Bug） |
| `perf` | 性能优化 |
| `test` | 测试 |
| `chore` | 构建过程或辅助工具 |

**示例：**
```
feat(seckill): 增加活动预热缓存机制
fix(order): 修复并发创建订单的幂等性问题
docs(readme): 补充部署文档
perf(redis): 优化 Lua 脚本减少网络往返
```

### 代码风格

- **Go 代码**：遵循 Go 官方编码规范，使用 `gofmt` 格式化
- **Vue 代码**：使用 ESLint + Prettier，遵循 Vue 3 组合式 API 风格
- **注释**：核心逻辑使用中文注释，公开 API 使用英文注释
- **命名**：Go 使用驼峰命名，Vue 组件使用 PascalCase

### 目录规范

```
service/     # 业务逻辑层，不允许直接操作数据库
dao/         # 数据访问层，封装所有数据库操作
redis/       # Redis 操作，Lua 脚本独立存放
controller/  # 控制器层，仅做参数校验和响应封装
middleware/   # 中间件，洋葱模型，职责单一
```

### 提交前检查

```bash
# 格式化 Go 代码
gofmt -w .

# 编译检查
go build ./...

# 运行测试
go test ./...
```

## Pull Request 流程

1. **创建 Issue 先讨论**（可选但推荐）：重大变更建议先提 Issue 讨论
2. **保持 PR 小而专注**：一个 PR 只做一件事
3. **写清楚描述**：说明做了什么、为什么这么做
4. **关联 Issue**：在描述中使用 `Closes #123` 关联 Issue
5. **确保 CI 通过**：所有检查必须绿色
6. **等待 Review**：维护者会尽快审查

## PR 模板

```markdown
## 变更类型
- [ ]  新功能
- [ ]   Bug 修复
- [ ]   文档更新
- [ ]  重构
- [ ]  性能优化

## 变更说明
简要描述做了什么改动。

## 测试
- [ ] 本地测试通过
- [ ] 压测结果无退化
- [ ] 新增测试用例

## 截图（如适用）
```

## 如何提 Issue

### Bug 报告

```markdown
**描述**
清晰描述 Bug 现象。

**复现步骤**
1. 执行 '...'
2. 点击 '...'
3. 看到错误

**期望行为**
描述期望的正确行为。

**环境**
- OS: Windows 11
- Go: 1.26.5
- Docker: 27.x
```

### 功能建议

```markdown
**需求背景**
描述为什么需要这个功能。

**功能描述**
具体描述你期望的功能。

**备选方案**
是否考虑过替代方案。
```

## 项目分层

| 层级 | 职责 | 举例 |
|:---|:---|:---|
| 接入层 | 负载均衡、WAF、CDN | Nginx |
| 服务集群 | 接口处理、业务逻辑 | Gin Controller/Service |
| 流量防护 | 限流、熔断、降级 | Sentinel-Go |
| 中间件集群 | 缓存、消息、注册 | Redis/RabbitMQ/Kafka/Etcd |
| 数据层 | 持久化、读写分离 | MySQL 主从 |
| 运维层 | 监控、日志、追踪 | Prometheus/Grafana/ELK/Jaeger |

## 联系方式

-   GitHub Issues：[提交 Issue](https://github.com/shij8396/miaosha/issues)
-   GitHub Discussions：讨论交流

---

**再次感谢你的贡献！**