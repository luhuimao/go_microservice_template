# Redis 缓存模块文档

> **所在路径**：`internal/cache/` · `internal/pkg/cache/`
> **依赖版本**：`github.com/redis/go-redis/v9 v9.18.0`

---

## 目录

1. [设计目标](#1-设计目标)
2. [模块结构](#2-模块结构)
3. [核心接口](#3-核心接口)
4. [缓存策略：Cache-Aside（旁路缓存）](#4-缓存策略cache-aside旁路缓存)
5. [Key 规范 & TTL](#5-key-规范--ttl)
6. [错误处理与降级](#6-错误处理与降级)
7. [配置说明](#7-配置说明)
8. [初始化与依赖注入](#8-初始化与依赖注入)
9. [单元测试](#9-单元测试)
10. [扩展指南](#10-扩展指南)
11. [性能调优](#11-性能调优)

---

## 1. 设计目标

| 目标           | 说明                                   |
| ------------ | ------------------------------------ |
| **降低 DB 压力** | 热点查询（`GET /users/:id`）优先走 Redis      |
| **接口解耦**     | `UserCache` 接口与 Redis 实现分离，方便 Mock 和替换 |
| **高可用降级**    | Redis 故障时自动降级查 DB，不影响业务              |
| **零侵入性**     | Cache 逻辑封装在 Service 层，transport/repo 层无感知 |

---

## 2. 模块结构

```
internal/
├── cache/
│   ├── user_cache.go        # UserCache 接口定义 + Redis 实现
│   └── user_cache_test.go   # 单元测试（miniredis）
└── pkg/
    └── cache/
        └── redis.go         # Redis 客户端工厂函数
```

### 各文件职责

| 文件 | 职责 |
|------|------|
| `internal/pkg/cache/redis.go` | 创建并验证 `*redis.Client`，Ping 失败即 panic（与 MySQL 工厂行为一致） |
| `internal/cache/user_cache.go` | 定义 `UserCache` 接口；提供 `userRedisCache` 实现，负责序列化、Key 拼接、TTL 管理 |
| `internal/cache/user_cache_test.go` | 使用 `miniredis` 对缓存层进行完整集成测试，无需真实 Redis |

---

## 3. 核心接口

```go
// internal/cache/user_cache.go

// ErrCacheMiss 表示 key 不存在，区别于 Redis 连接错误。
var ErrCacheMiss = errors.New("cache miss")

type UserCache interface {
    // Get 从缓存中查询用户，不存在时返回 ErrCacheMiss。
    Get(ctx context.Context, id uint) (*domain.User, error)

    // Set 写入缓存，ttl=0 时使用默认 TTL（5 分钟）。
    Set(ctx context.Context, id uint, user *domain.User, ttl time.Duration) error

    // Del 删除缓存条目（适用于写操作后缓存失效场景）。
    Del(ctx context.Context, id uint) error
}
```

> **为什么定义接口？**
> - 单元测试时可替换为 Mock，不依赖真实 Redis
> - 未来可切换为 Memcached、本地 LRU 等实现
> - 符合 Clean Architecture 的依赖倒置原则

---

## 4. 缓存策略：Cache-Aside（旁路缓存）

### 读操作流程

```
GET /users/:id
      │
      ▼
 UserService.Get(id)
      │
      ├─► ① Redis.Get("user:<id>")
      │         ├── HIT  ──────────────────────→ 返回 User ✓
      │         ├── MISS ──────────────────┐
      │         └── ERROR（降级）──────────┘
      │                                    ↓
      └──────────────── ② MySQL.FindByID(id)
                                           │
                              ③ Redis.Set("user:<id>", ttl=5min)
                                           │
                                        返回 User ✓
```

### 写操作说明

当前 `Create` 操作**不主动删除缓存**（新建用户不存在缓存 key，无需失效）。
若未来添加更新/删除用户接口，应在写入 DB 成功后调用 `cache.Del(ctx, id)` 使缓存失效。

```go
// 推荐的更新用户后缓存失效模式
func (s *userService) Update(id uint, ...) error {
    if err := s.repo.Update(id, ...); err != nil {
        return err
    }
    _ = s.cache.Del(ctx, id)  // 失效缓存，下次 Get 重新从 DB 加载
    return nil
}
```

---

## 5. Key 规范 & TTL

| 属性     | 值              |
| ------ | -------------- |
| Key 格式 | `user:<id>`    |
| 示例     | `user:1`、`user:42` |
| 序列化    | JSON           |
| 默认 TTL | **5 分钟**       |
| TTL 传参 | `Set` 的 `ttl` 参数；传 `0` 使用默认值 |

Key 生成函数：

```go
func userKey(id uint) string {
    return fmt.Sprintf("user:%d", id)
}
```

---

## 6. 错误处理与降级

### 哨兵错误：`ErrCacheMiss`

```go
user, err := s.cache.Get(ctx, id)
if err == nil {
    return user, nil          // ✅ 缓存命中
}
if errors.Is(err, cache.ErrCacheMiss) {
    // 继续查 DB
}
// 其他 err = Redis 连接异常，降级查 DB
```

### 降级策略

| 场景           | 处理方式                     |
| ------------ | ------------------------ |
| 缓存未命中（Miss）  | 查 DB，回填缓存                |
| Redis 连接异常   | 跳过缓存，直接查 DB，**不返回错误**    |
| DB 查询失败      | 返回错误，**不写缓存**            |
| 回填缓存失败       | 忽略错误（用 `_ =`），不影响主流程    |
| cache 为 nil  | 退化为纯 DB 模式（便于测试、禁用缓存场景）  |

---

## 7. 配置说明

`configs/config.yaml`：

```yaml
redis:
  addr: "redis:6379"      # Redis 服务地址（Docker Compose 服务名）
  password: ""            # 无密码留空
  db: 0                   # 使用默认 DB 0
```

本地开发修改为：

```yaml
redis:
  addr: "localhost:6379"
  password: ""
  db: 0
```

对应的 Go 结构体（`internal/config/config.go`）：

```go
Redis struct {
    Addr     string
    Password string
    DB       int
}
```

---

## 8. 初始化与依赖注入

`cmd/api/main.go` 中的完整初始化链：

```go
// 1. 加载配置
cfg := config.Load()

// 2. 初始化 Redis 客户端（Ping 失败则 panic）
rdb := pkgcache.NewRedisClient(cfg)

// 3. 创建 UserCache 实现
userCache := cache.NewUserCache(rdb)

// 4. 注入到 UserService
userRepo := repository.NewUserRepository(db)
userService := service.NewUserService(userRepo, userCache)
```

> **传入 nil 禁用缓存**：
> ```go
> userService := service.NewUserService(userRepo, nil)
> // Service 会退化为纯数据库查询模式
> ```

---

## 9. 单元测试

### 运行命令

```bash
# 仅测试缓存层
go test ./internal/cache/... -v

# 仅测试服务层
go test ./internal/service/... -v

# 全部测试
go test ./internal/cache/... ./internal/service/... -v
```

### 测试工具

| 工具 | 用途 |
|------|------|
| `github.com/alicebob/miniredis/v2` | 内嵌 Redis，无需启动真实服务 |
| `github.com/stretchr/testify` | 断言库（`assert`/`require`） |
| 手写 Mock | 实现 `UserCache`、`UserRepository` 接口 |

### 测试用例一览

**`internal/cache/user_cache_test.go`（5 个）**

| 测试名 | 验证内容 |
|--------|----------|
| `TestUserCache_SetAndGet` | 正常写入并读取 |
| `TestUserCache_Get_Miss` | 未命中返回 `ErrCacheMiss` |
| `TestUserCache_Del` | 删除后读取返回 Miss |
| `TestUserCache_TTL_Expiry` | TTL 到期后返回 Miss（`FastForward`） |
| `TestUserCache_DataIntegrity` | JSON 序列化/反序列化完整性 |

**`internal/service/user_service_test.go`（7 个）**

| 测试名 | 验证内容 |
|--------|----------|
| `TestUserService_Get_CacheHit` | 命中缓存，DB 不被调用 |
| `TestUserService_Get_CacheMiss_WriteThrough` | 未命中 → 查 DB → 回填缓存 |
| `TestUserService_Get_CacheError_Fallback` | Redis 异常 → 降级查 DB |
| `TestUserService_Get_DBError` | DB 失败 → 错误上传，不写缓存 |
| `TestUserService_Get_NilCache` | nil cache → 纯 DB 模式 |
| `TestUserService_Create_Success` | 正常创建用户 |
| `TestUserService_Create_DBError` | DB 写入失败时返回错误 |

---

## 10. 扩展指南

### 为新实体添加缓存

1. 在 `internal/cache/` 创建新文件，如 `order_cache.go`
2. 定义新接口 `OrderCache`，实现 `Get` / `Set` / `Del`
3. 在对应 Service 注入并套用 Cache-Aside 模式

### 切换缓存实现

只需实现 `UserCache` 接口即可替换为其他缓存后端：

```go
// 示例：切换为本地 LRU 缓存
type localLRUCache struct { /* ... */ }
func (c *localLRUCache) Get(...) (*domain.User, error) { /* ... */ }
func (c *localLRUCache) Set(...) error { /* ... */ }
func (c *localLRUCache) Del(...) error { /* ... */ }
```

### 推荐扩展方向

| 方向 | 说明 |
|------|------|
| 缓存预热 | 应用启动时批量加载热点数据到 Redis |
| 分布式锁 | 用 `SET NX` 防止缓存击穿（大量并发下同一 key 回源） |
| 多级缓存 | 本地内存（L1）+ Redis（L2）+ DB（L3） |
| 监控指标 | 统计缓存命中率，接入 Prometheus |
| 批量操作 | 使用 `MGET` / Pipeline 减少网络 RTT |

---

## 11. 性能调优

> **优先级建议**：连接池配置 → TTL 抖动 → Singleflight → 服务端 maxmemory 策略。前两项改动最小、收益最大，建议优先落地。

### 11.1 连接池调优

当前 `NewRedisClient` 使用全默认连接池参数，生产环境必须显式配置：

```go
// internal/pkg/cache/redis.go
rdb := redis.NewClient(&redis.Options{
    Addr:     cfg.Redis.Addr,
    Password: cfg.Redis.Password,
    DB:       cfg.Redis.DB,

    // ── 连接池 ──────────────────────────────────────
    PoolSize:     10,               // 最大连接数（建议 = CPU 核心数 × 2~4）
    MinIdleConns: 3,                // 最小空闲连接，避免冷启动延迟
    MaxIdleConns: 5,                // 最大空闲连接

    // ── 超时 ────────────────────────────────────────
    DialTimeout:  3 * time.Second,
    ReadTimeout:  500 * time.Millisecond,
    WriteTimeout: 500 * time.Millisecond,
    PoolTimeout:  1 * time.Second,  // 等待空闲连接的超时

    // ── 重试 ────────────────────────────────────────
    MaxRetries:      3,
    MinRetryBackoff: 8 * time.Millisecond,
    MaxRetryBackoff: 512 * time.Millisecond,
})
```

同时在 `internal/config/config.go` 中扩充配置结构体以支持外部配置：

```go
Redis struct {
    Addr            string
    Password        string
    DB              int
    PoolSize        int
    MinIdleConns    int
    DialTimeoutSec  int
    ReadTimeoutMs   int
    WriteTimeoutMs  int
}
```

---

### 11.2 TTL 随机抖动（防雪崩）

**问题**：大量 key 使用固定 TTL，会在同一时刻集中过期，导致 DB 瞬间压力激增（缓存雪崩）。

**方案**：在基础 TTL 上叠加随机抖动：

```go
// internal/cache/user_cache.go
import "math/rand"

const (
    baseTTL   = 5 * time.Minute
    jitterMax = 60 // 秒，抖动范围
)

func jitteredTTL() time.Duration {
    jitter := time.Duration(rand.Intn(jitterMax)) * time.Second
    return baseTTL + jitter  // 实际 TTL：5min ~ 6min 随机
}

func (c *userRedisCache) Set(ctx context.Context, id uint, user *domain.User, ttl time.Duration) error {
    if ttl == 0 {
        ttl = jitteredTTL() // 替换原固定 TTL
    }
    // ...
}
```

---

### 11.3 缓存穿透防护

**问题**：大量请求查询不存在的 ID（如爬虫攻击），每次均穿透到 DB。

**方案 A：缓存空值**

```go
// service 层 Get 方法中
user, err := s.repo.FindByID(id)
if err != nil {
    if isNotFoundError(err) {
        // 缓存空标记，短 TTL 防止穿透
        _ = s.cache.Set(ctx, id, &domain.User{ID: 0}, 1*time.Minute)
    }
    return nil, err
}
// Get 时识别空标记
if user.ID == 0 {
    return nil, ErrNotFound
}
```

**方案 B：布隆过滤器**（适合 ID 范围已知的场景）

```go
import "github.com/bits-and-blooms/bloom/v3"

// 启动时预热已存在的 ID 集合
filter := bloom.NewWithEstimates(1_000_000, 0.01)
filter.Add([]byte("1"))
// ...

// 查询前预判
if !filter.Test([]byte(fmt.Sprintf("%d", id))) {
    return nil, ErrNotFound // 直接拒绝，不查缓存也不查 DB
}
```

---

### 11.4 缓存击穿防护（Singleflight）

**问题**：热点 key 过期瞬间，大量并发请求同时回源 DB。

**方案**：使用 `singleflight` 合并并发请求，只允许一个请求穿透到 DB：

```go
import "golang.org/x/sync/singleflight"

type userService struct {
    repo  repository.UserRepository
    cache cache.UserCache
    sfg   singleflight.Group // 新增
}

func (s *userService) Get(id uint) (*domain.User, error) {
    ctx := context.Background()

    // 先查缓存
    if s.cache != nil {
        if user, err := s.cache.Get(ctx, id); err == nil {
            return user, nil
        }
    }

    // singleflight：相同 id 的并发请求共享一次 DB 查询
    key := fmt.Sprintf("user:%d", id)
    v, err, _ := s.sfg.Do(key, func() (interface{}, error) {
        user, err := s.repo.FindByID(id)
        if err != nil {
            return nil, err
        }
        _ = s.cache.Set(ctx, id, user, 0)
        return user, nil
    })
    if err != nil {
        return nil, err
    }
    return v.(*domain.User), nil
}
```

---

### 11.5 序列化方案选型

| 方案 | 相对性能 | 特点 | 推荐场景 |
|------|----------|------|----------|
| `encoding/json`（当前）| 基准 | 标准库，无依赖 | 开发/低流量 |
| `github.com/bytedance/sonic` | ~5-10x 更快 | 零拷贝，兼容 `encoding/json` | 高 QPS 服务 |
| `github.com/vmihailenco/msgpack` | ~3x 更快，体积小 30% | 二进制格式 | 存储空间敏感 |
| `google.golang.org/protobuf` | 最快，类型安全 | 需 proto 定义 | 已有 proto 的项目 |

切换 `sonic` 示例（接口兼容，无需修改调用方）：

```go
import "github.com/bytedance/sonic"

// 替换 json.Marshal / json.Unmarshal
data, err := sonic.Marshal(user)
err = sonic.Unmarshal(val, &user)
```

---

### 11.6 Redis 服务端配置

在 `docker-compose.yml` 中为 Redis 添加关键参数：

```yaml
redis:
  image: redis:7-alpine
  command: >
    redis-server
    --maxmemory 256mb
    --maxmemory-policy allkeys-lru
    --save ""
    --appendonly no
  ports:
    - "6379:6379"
```

| 参数 | 推荐值 | 说明 |
|------|--------|------|
| `maxmemory` | 物理内存的 50~70% | 防止 OOM |
| `maxmemory-policy` | `allkeys-lru` | 驱逐最久未使用的 key（缓存场景最优） |
| `save ""` | — | 纯缓存场景关闭 RDB 持久化，降低 I/O |
| `appendonly no` | — | 关闭 AOF，减少写放大 |

---

### 11.7 监控指标

生产环境必须监控以下指标，接入 Prometheus：

```go
// 使用 go-redis 官方 Prometheus Hook
import "github.com/redis/go-redis/extra/redisprometheus/v9"

rdb.AddHook(redisprometheus.NewHook(
    redisprometheus.WithInstanceName("user_service"),
))
```

| 指标 | 告警阈值参考 |
|------|-------------|
| 缓存命中率（`keyspace_hits / (hits+misses)`） | < 80% 需排查 |
| 连接池等待次数（`pool_stats.Waits`） | > 0 需扩充 `PoolSize` |
| 命令延迟 P99 | > 10ms 需优化网络或序列化 |
| 内存使用率 | > 80% `maxmemory` 需扩容 |

Redis CLI 快速查看：

```bash
redis-cli INFO stats | grep keyspace
redis-cli INFO memory | grep used_memory_human
```
