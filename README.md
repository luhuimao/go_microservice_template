
# Go Microservice Template

一个生产可扩展的 **Go 微服务基础模板**，基于 Clean Architecture 设计，内置：

* HTTP API（Gin）
* MySQL（GORM）
* **Redis 缓存（Cache-Aside 模式）**
* 配置管理（Viper）
* 结构化日志（Zap）
* Docker & Docker Compose
* 优雅关闭（Graceful Shutdown）
* 单元测试（testify + miniredis）

📄 **模块文档**：[Redis 缓存模块详细设计](docs/redis-cache.md) · [MySQL 调优指南](docs/mysql-tuning.md)

---

# 🚀 项目目标

本项目不是 Demo，而是一个 **可直接用于企业级微服务开发的基础骨架**，适用于：

* 后台管理系统
* 交易系统
* 订单系统
* 用户系统
* SaaS 平台
* API 服务

---

# 📁 项目结构

```
microservice_mvp_demo/
├── cmd/
│   └── api/
│       └── main.go              # 应用入口
├── internal/
│   ├── config/                  # 配置管理
│   ├── domain/                  # 领域模型
│   ├── cache/                   # 缓存抽象层（UserCache接口 + Redis实现）
│   ├── repository/              # 数据访问层（MySQL）
│   ├── service/                 # 业务逻辑层（含缓存策略）
│   ├── transport/               # HTTP 入口层
│   └── pkg/
│       ├── cache/               # Redis 客户端工厂
│       ├── database/            # MySQL 客户端工厂
│       └── logger/              # Zap 日志
├── configs/
│   └── config.yaml              # 配置文件（Server / MySQL / Redis）
├── Dockerfile
├── docker-compose.yml           # MySQL 8 + Redis 7 + App
├── Makefile
└── go.mod
```

---

# 🧠 架构设计

采用 **Clean Architecture 分层结构**，新增 Cache 层：

```
Transport Layer  →  Service Layer  →  Cache Layer  →  Redis
                        │
                        └──────────→  Repository Layer  →  MySQL
```

### 分层说明

| 层级         | 作用                             |
| ---------- | ------------------------------ |
| transport  | 接收 HTTP 请求，参数校验                |
| service    | 业务逻辑 + Cache-Aside 缓存策略        |
| cache      | UserCache 接口 + Redis 实现（可替换）   |
| repository | 数据持久化（MySQL via GORM）          |
| domain     | 纯业务模型                          |
| pkg        | 基础设施工厂（Redis客户端、MySQL客户端、日志）   |

设计原则：

* 依赖单向，Interface 解耦
* 业务逻辑与框架无关
* 缓存层可独立 Mock / 替换
* Redis 出错时自动降级查 DB，保证高可用

---

# 🛠 技术栈

| 组件     | 技术                         |
| ------ | -------------------------- |
| Web 框架 | Gin v1.11                  |
| ORM    | GORM v1.31                 |
| 数据库    | MySQL 8                    |
| 缓存     | Redis 7（go-redis/v9 v9.18） |
| 配置管理   | Viper v1.21                |
| 日志     | Zap v1.27                  |
| 容器化    | Docker + Docker Compose    |
| 测试     | testify + miniredis         |

---

# ⚡️ Redis 缓存模块

## 缓存策略：Cache-Aside（旁路缓存）

```
GET /users/:id
      │
      ▼
 Service.Get()
      │
      ├─► [1] Redis.Get("user:<id>")
      │         ├── HIT  → 直接返回
      │         └── MISS ──────────────┐
      │                                ▼
      └─────────────── [2] MySQL.FindByID(id)
                                       │
                            [3] Redis.Set("user:<id>", ttl=5min)
                                       │
                                    返回结果
```

## 关键设计

| 特性      | 实现                                |
| ------- | --------------------------------- |
| Key 格式  | `user:<id>`                       |
| 序列化     | JSON                              |
| 默认 TTL  | 5 分钟                             |
| 缓存未命中   | `ErrCacheMiss` 哨兵错误               |
| Redis出错 | 自动降级查 DB，不返回错误（保证可用性）            |
| nil 缓存  | 退化为纯 DB 模式（方便测试/禁用缓存）            |

---

# ⚙️ 本地运行

## 1️⃣ 启动所有服务

```bash
docker-compose up --build
```

默认端口：

```
API:   http://localhost:8080
MySQL: localhost:3306
Redis: localhost:6379
```

## 2️⃣ 手动运行（非 Docker）

> 本地须已启动 MySQL 和 Redis，并更新 `configs/config.yaml`。

```bash
go mod tidy
go run cmd/api/main.go
```

---

# ⚙️ 配置文件

`configs/config.yaml`：

```yaml
server:
  port: "8080"

mysql:
  dsn: "root:password@tcp(mysql:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"

redis:
  addr: "redis:6379"
  password: ""
  db: 0
```

---

# 📌 API 示例

## 创建用户

```
POST /users
Content-Type: application/json
```

请求体：

```json
{
  "Name": "ben",
  "Age": 30
}
```

响应：

```json
{ "status": "ok" }
```

## 查询用户

```
GET /users/{id}
```

响应（首次查 DB + 回填缓存，再次直接命中 Redis）：

```json
{ "ID": 1, "Name": "ben", "Age": 30 }
```

验证 Redis 缓存：

```bash
docker exec -it <redis-container> redis-cli GET "user:1"
```

---

# 🧪 测试

## 运行单元测试

```bash
go test ./internal/cache/... ./internal/service/... -v
```

## 测试覆盖

| 测试文件                        | 工具                  | 用例数 |
| --------------------------- | ------------------- | --- |
| `internal/cache/user_cache_test.go`   | miniredis（内嵌Redis） | 5   |
| `internal/service/user_service_test.go` | 手写 Mock             | 7   |

### cache 层测试场景

- `Set` / `Get` 正常写读
- `Get` 缓存未命中 → 返回 `ErrCacheMiss`
- `Del` 后 `Get` → Miss
- TTL 到期后 `Get` → Miss
- JSON 数据完整性校验

### service 层测试场景

- 缓存命中 → 不查 DB
- 缓存未命中 → 查 DB → 回填缓存
- Redis 出错 → 降级查 DB（不影响可用性）
- DB 查询失败 → 向上传递错误
- `nil` cache → 纯 DB 模式
- `Create` 成功 / 失败

---

# 🐳 Docker Compose 说明

```yaml
services:
  mysql:   # MySQL 8，端口 3306
  redis:   # Redis 7 Alpine，端口 6379
  app:     # Go 应用，依赖 mysql + redis
```

---

# 📦 Makefile 命令

```bash
make run       # 本地运行
make build     # 编译
make docker    # Docker 启动
```

---

# 🧩 可扩展方向

| 能力          | 状态      |
| ----------- | ------- |
| HTTP API    | ✅ 已实现   |
| MySQL       | ✅ 已实现   |
| Redis 缓存    | ✅ 已实现   |
| 单元测试        | ✅ 已实现   |
| gRPC 双协议    | 🔜 可扩展  |
| JWT 鉴权      | 🔜 可扩展  |
| Kafka 事件驱动  | 🔜 可扩展  |
| Prometheus 监控 | 🔜 可扩展 |
| Jaeger 链路追踪 | 🔜 可扩展  |
| Kubernetes 部署 | 🔜 可扩展 |

---

# 📈 未来升级路线

| 阶段 | 能力       |
| -- | -------- |
| V1 | 单体微服务    |
| V2 | 多服务拆分    |
| V3 | 事件驱动     |
| V4 | 服务网格     |
| V5 | 高并发架构    |

---

# 🎯 适合人群

* 想系统掌握 Go 微服务架构的工程师
* 准备面试高级 Go 的开发者
* 需要企业级服务模板的团队
* 构建交易 / 订单 / 用户系统的项目

---

# 📜 License

MIT License

---

# ✨ 总结

这是一个**可直接运行、结构清晰、可扩展、可容器化、可生产演进**的 Go 微服务基础模板。

包含完整的 **Redis Cache-Aside 缓存模块**与**单元测试体系**，开箱即用。
