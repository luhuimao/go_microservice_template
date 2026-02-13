
# Go Microservice Template

一个生产可扩展的 **Go 微服务基础模板**，基于 Clean Architecture 设计，内置：

* HTTP API（Gin）
* MySQL（GORM）
* 配置管理（Viper）
* 结构化日志（Zap）
* Docker & Docker Compose
* 优雅关闭（Graceful Shutdown）
* 分层架构设计
* 可扩展为 gRPC / Kafka / K8s 部署

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
go-micro-template/
├── cmd/
│   └── api/
│       └── main.go          # 应用入口
├── internal/
│   ├── config/              # 配置管理
│   ├── domain/              # 领域模型
│   ├── repository/          # 数据访问层
│   ├── service/             # 业务逻辑层
│   ├── transport/           # HTTP / gRPC 入口层
│   └── pkg/                 # 公共组件（数据库/日志等）
├── configs/
│   └── config.yaml          # 配置文件
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── go.mod
```

---

# 🧠 架构设计

采用 **Clean Architecture 分层结构**：

```
Transport Layer  →  Service Layer  →  Repository Layer  →  Database
```

### 分层说明

| 层级         | 作用        |
| ---------- | --------- |
| transport  | 接收请求，参数校验 |
| service    | 业务逻辑      |
| repository | 数据持久化     |
| domain     | 纯业务模型     |

设计原则：

* 依赖单向
* Interface 解耦
* 业务逻辑与框架无关
* 可测试性强

---

# 🛠 技术栈

| 组件     | 技术             |
| ------ | -------------- |
| Web 框架 | Gin            |
| ORM    | GORM           |
| 数据库    | MySQL          |
| 配置管理   | Viper          |
| 日志     | Zap            |
| 容器化    | Docker         |
| 编排     | Docker Compose |

---

# ⚙️ 本地运行

## 1️⃣ 启动数据库 + 服务

```bash
docker-compose up --build
```

默认端口：

```
API:  http://localhost:8080
MySQL: localhost:3306
```

---

## 2️⃣ 手动运行（非 Docker）

```bash
go mod tidy
go run cmd/api/main.go
```

---

# 📌 API 示例

## 创建用户

```
POST /users
```

请求体：

```json
{
  "name": "ben",
  "age": 30
}
```

---

## 查询用户

```
GET /users/{id}
```

示例：

```
GET /users/1
```

---

# 🐳 Docker 说明

## Dockerfile

* 使用 Alpine 轻量镜像
* 静态编译
* 可直接部署到 Kubernetes

---

## docker-compose.yml

包含：

* MySQL 8
* 应用服务
* 自动依赖启动

---

# 📦 Makefile 命令

```bash
make run       # 本地运行
make build     # 编译
make docker    # Docker 启动
```

---

# 🧩 可扩展方向

本模板支持快速升级为企业级微服务架构：

### 可扩展能力

* gRPC 双协议支持
* JWT 鉴权
* RBAC 权限控制
* Redis 缓存
* Kafka 事件驱动
* Prometheus 监控
* Jaeger 链路追踪
* Kubernetes 部署
* CI/CD Pipeline
* 分布式事务（Saga）

---

# 🛡 生产级改进建议

1. 增加：

   * 中间件统一异常处理
   * 请求日志 TraceID
   * OpenTelemetry

2. 数据层：

   * 连接池优化
   * 分库分表
   * 读写分离

3. 高可用：

   * 多副本部署
   * 健康检查
   * 滚动更新

---

# 🧪 单元测试建议

建议添加：

```
internal/service/user_service_test.go
```

配合：

* gomock
* testify

目标覆盖：

* 业务逻辑
* Repository Mock 测试

---

# 📈 未来升级路线

| 阶段 | 能力    |
| -- | ----- |
| V1 | 单体微服务 |
| V2 | 多服务拆分 |
| V3 | 事件驱动  |
| V4 | 服务网格  |
| V5 | 高并发架构 |

---

# 🎯 适合人群

* 想系统掌握 Go 微服务架构的工程师
* 准备面试高级 Go 的开发者
* 需要企业级服务模板的团队
* 构建交易/订单/用户系统的项目

---

# 📜 License

MIT License

---

# ✨ 总结

这是一个：

* 可直接运行
* 结构清晰
* 可扩展
* 可容器化
* 可生产演进

的 Go 微服务基础模板。
