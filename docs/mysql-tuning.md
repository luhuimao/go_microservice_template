# MySQL 调优文档

> **所在路径**：`internal/pkg/database/` · `internal/repository/`
> **依赖版本**：`gorm.io/gorm v1.31` · `gorm.io/driver/mysql v1.6` · `MySQL 8`

---

## 目录

1. [连接池调优](#1-连接池调优)
2. [GORM 性能配置](#2-gorm-性能配置)
3. [DSN 参数调优](#3-dsn-参数调优)
4. [索引优化](#4-索引优化)
5. [查询优化（GORM 使用层面）](#5-查询优化gorm-使用层面)
6. [MySQL 服务端配置](#6-mysql-服务端配置)
7. [慢查询分析](#7-慢查询分析)
8. [调优优先级建议](#8-调优优先级建议)

---

## 1. 连接池调优

当前 `NewMySQL` 使用 GORM 默认连接池参数，生产环境必须显式配置：

```go
// internal/pkg/database/mysql.go
func NewMySQL(cfg *config.Config) *gorm.DB {
    db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{})
    if err != nil {
        panic(err)
    }

    sqlDB, _ := db.DB() // 获取底层 *sql.DB

    // ── 连接池 ──────────────────────────────────────
    sqlDB.SetMaxOpenConns(50)                    // 最大连接数（建议 = CPU 核心数 × 4~8）
    sqlDB.SetMaxIdleConns(10)                    // 最大空闲连接
    sqlDB.SetConnMaxLifetime(30 * time.Minute)   // 连接最大存活时间（防止 DB 端主动断开）
    sqlDB.SetConnMaxIdleTime(5 * time.Minute)    // 空闲连接超时回收

    return db
}
```

同步扩充配置结构体（`internal/config/config.go`）：

```go
MySQL struct {
    DSN             string
    MaxOpenConns    int
    MaxIdleConns    int
    ConnMaxLifetime int // 秒
    ConnMaxIdleTime int // 秒
}
```

对应 `configs/config.yaml`：

```yaml
mysql:
  dsn: "root:password@tcp(mysql:3306)/test?charset=utf8mb4&parseTime=True&loc=Local"
  maxOpenConns: 50
  maxIdleConns: 10
  connMaxLifetime: 1800
  connMaxIdleTime: 300
```

| 参数 | 推荐值 | 说明 |
|------|--------|------|
| `MaxOpenConns` | CPU 核心数 × 4~8 | 上限不超过 MySQL `max_connections` |
| `MaxIdleConns` | `MaxOpenConns` 的 20~30% | 保持热连接，避免冷启动 |
| `ConnMaxLifetime` | 30 分钟 | 防止数据库端（防火墙）主动断开空闲连接 |
| `ConnMaxIdleTime` | 5 分钟 | 回收长时间空闲的连接，释放 DB 资源 |

---

## 2. GORM 性能配置

```go
// internal/pkg/database/mysql.go
db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{
    // ① 跳过默认事务——单条写操作性能提升 ~30%
    //    注意：批量写或需要事务一致性时应手动开启事务
    SkipDefaultTransaction: true,

    // ② 缓存 PreparedStatement——减少反射和 SQL 解析开销
    PrepareStmt: true,

    // ③ 命名策略（可选）——统一表名/列名风格
    NamingStrategy: schema.NamingStrategy{
        SingularTable: true, // 表名不加复数 s（user 而非 users）
    },

    // ④ 慢查询日志——生产只打印 Warning 级别以上
    Logger: logger.New(
        log.New(os.Stdout, "\r\n", log.LstdFlags),
        logger.Config{
            SlowThreshold:             200 * time.Millisecond, // 慢查询阈值
            LogLevel:                  logger.Warn,
            IgnoreRecordNotFoundError: true,                   // 忽略"记录不存在"错误日志
            Colorful:                  false,
        },
    ),
})
```

| 配置项 | 默认 | 开启后效果 |
|--------|------|-----------|
| `SkipDefaultTransaction` | false | 单条写性能 +30%，减少事务开销 |
| `PrepareStmt` | false | 减少 SQL 解析，高并发下效果显著 |
| `IgnoreRecordNotFoundError` | false | 避免正常"查无此记录"刷屏日志 |

---

## 3. DSN 参数调优

```yaml
# configs/config.yaml —— 完整 DSN 示例
mysql:
  dsn: "root:password@tcp(mysql:3306)/test\
    ?charset=utf8mb4\
    &parseTime=True\
    &loc=Local\
    &timeout=10s\
    &readTimeout=30s\
    &writeTimeout=30s\
    &interpolateParams=true\
    &collation=utf8mb4_unicode_ci"
```

| 参数 | 推荐值 | 说明 |
|------|--------|------|
| `timeout` | `10s` | 建立 TCP 连接的超时 |
| `readTimeout` | `30s` | 读操作超时，防止慢查询挂住连接 |
| `writeTimeout` | `30s` | 写操作超时 |
| `interpolateParams` | `true` | 客户端拼接参数，减少一次 `Prepare` 往返 RTT |
| `charset` | `utf8mb4` | 支持 emoji，已正确设置 |
| `collation` | `utf8mb4_unicode_ci` | 大小写不敏感排序，与 charset 匹配 |

> **注意**：`interpolateParams=true` 会在客户端拼接 SQL，适合低并发/读多写少场景；高并发写入时建议关闭（默认 false），使用服务端 Prepare 防 SQL 注入。

---

## 4. 索引优化

当前 `User` 模型无任何索引声明，添加 GORM 索引标签：

```go
// internal/domain/user.go
type User struct {
    ID        uint           `gorm:"primarykey"`
    Name      string         `gorm:"index;size:64;not null"`  // 普通索引
    Age       int            `gorm:"index"`
    CreatedAt time.Time      `gorm:"index"`                   // 支持按时间范围查询
    DeletedAt gorm.DeletedAt `gorm:"index"`                   // 软删除必须加索引
}
```

### 联合索引

按多字段查询时使用联合索引（字段顺序遵循最左前缀原则）：

```go
type User struct {
    ID   uint   `gorm:"primarykey"`
    Name string `gorm:"index:idx_name_age,priority:1;size:64"`
    Age  int    `gorm:"index:idx_name_age,priority:2"`
}
```

### 唯一索引

```go
Email string `gorm:"uniqueIndex;size:128"`
```

### 索引设计原则

| 原则 | 说明 |
|------|------|
| 高选择性优先 | 选择性 = 不同值数 / 总行数，大于 0.1 才有意义 |
| 最左前缀匹配 | 联合索引 `(a, b, c)` 可命中 `(a)`、`(a,b)`，不能命中 `(b,c)` |
| 避免过多索引 | 每个索引占写开销，写多表不超过 5 个索引 |
| 覆盖索引 | `SELECT id, name FROM users WHERE age = ?` 可建 `(age, id, name)` 覆盖索引，避免回表 |

---

## 5. 查询优化（GORM 使用层面）

### 5.1 只查必要字段，避免 SELECT *

```go
// ❌ 当前实现
r.db.First(&user, id)

// ✅ 只取必要字段
r.db.Select("id", "name", "age").First(&user, id)
```

### 5.2 批量插入替代逐条写入

```go
// ❌ 循环单条
for _, u := range users {
    db.Create(&u)
}

// ✅ 批量（一次 INSERT 多行，性能 10x+）
db.CreateInBatches(users, 100) // 每批 100 条
```

### 5.3 避免 First 的隐式排序

`First` 会自动追加 `ORDER BY primary_key`，批量查询时有额外开销：

```go
// ❌ 不必要的排序开销
db.First(&user)

// ✅ 有明确主键时直接用 Take（无 ORDER BY）
db.Take(&user, id)

// ✅ 批量查询用 Find（无额外排序）
var users []domain.User
db.Where("age > ?", 18).Find(&users)
```

### 5.4 事务批量写入

```go
err := db.Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&user1).Error; err != nil {
        return err
    }
    if err := tx.Create(&user2).Error; err != nil {
        return err
    }
    return nil
})
```

### 5.5 分页查询（避免深翻页）

```go
// ❌ OFFSET 深翻页性能差（OFFSET 100000 需扫描 100000 行）
db.Offset(100000).Limit(10).Find(&users)

// ✅ 游标分页（基于上一页最后的 ID）
db.Where("id > ?", lastID).Limit(10).Order("id ASC").Find(&users)
```

---

## 6. MySQL 服务端配置

`docker-compose.yml` 中为 MySQL 添加服务端调优参数：

```yaml
mysql:
  image: mysql:8
  command: >
    --innodb_buffer_pool_size=256M
    --max_connections=200
    --slow_query_log=1
    --long_query_time=0.2
    --innodb_flush_log_at_trx_commit=2
    --sync_binlog=0
    --innodb_io_capacity=2000
    --innodb_read_io_threads=4
    --innodb_write_io_threads=4
  environment:
    MYSQL_ROOT_PASSWORD: password
    MYSQL_DATABASE: test
  ports:
    - "3306:3306"
```

| 参数 | 推荐值 | 说明 |
|------|--------|------|
| `innodb_buffer_pool_size` | 物理内存的 50~70% | **最重要参数**，缓存数据页和索引，减少磁盘 I/O |
| `max_connections` | 200 | 与应用连接池 `MaxOpenConns` 对齐，防止超限 |
| `slow_query_log` | 1 | 开启慢查询日志 |
| `long_query_time` | 0.2 | 超过 200ms 记为慢查询 |
| `innodb_flush_log_at_trx_commit` | 2 | 1=最安全（每事务刷盘），2=每秒刷盘（性能更好，宕机丢 ≤1s 数据） |
| `sync_binlog` | 0 | 非关键业务关闭 binlog 同步，降低写延迟（0=OS 自行决定） |
| `innodb_io_capacity` | 2000 | SSD 磁盘可调高（HDD 建议 200） |
| `innodb_read/write_io_threads` | 4 | 多核 CPU 可提高到 8 |

---

## 7. 慢查询分析

### 7.1 查看慢查询统计

```sql
-- 累计慢查询次数
SHOW GLOBAL STATUS LIKE 'Slow_queries';

-- 按耗时排序的 TOP 10 慢语句（MySQL 8）
SELECT DIGEST_TEXT, COUNT_STAR, AVG_TIMER_WAIT/1e12 AS avg_sec
FROM performance_schema.events_statements_summary_by_digest
ORDER BY SUM_TIMER_WAIT DESC
LIMIT 10;
```

### 7.2 EXPLAIN 分析执行计划

```sql
EXPLAIN SELECT * FROM users WHERE name = 'Alice';
```

重点关注字段：

| 字段 | 危险值 | 说明 |
|------|--------|------|
| `type` | `ALL` | 全表扫描，必须加索引 |
| `rows` | 过大 | 扫描行数多，索引选择性差 |
| `Extra` | `Using filesort` | 排序未走索引，考虑联合索引 |
| `Extra` | `Using temporary` | 使用临时表，GROUP BY / ORDER BY 优化 |
| `key` | `NULL` | 未使用任何索引 |

### 7.3 GORM 打印实际执行的 SQL

```go
// 开发环境临时开启完整日志
db.Session(&gorm.Session{Logger: db.Logger.LogMode(logger.Info)}).Find(&users)

// 或查看单条语句
db.Debug().Where("name = ?", "Alice").First(&user)
```

---

## 8. 调优优先级建议

| 优先级 | 项目 | 预期收益 | 改动成本 |
|--------|------|----------|----------|
| 🔴 P0 | 连接池参数（`MaxOpenConns` 等） | 防止连接耗尽，稳定性保障 | 低 |
| 🔴 P0 | `innodb_buffer_pool_size` | 减少磁盘 I/O，查询提速 2~10x | 低 |
| 🟠 P1 | `SkipDefaultTransaction` + `PrepareStmt` | 写性能 +30%，无副作用 | 低 |
| 🟠 P1 | 开启慢查询日志 | 定位问题必备 | 低 |
| 🟡 P2 | 索引补充（`Name`、`CreatedAt`、软删除） | 按需查询提速 | 中 |
| 🟡 P2 | DSN 超时参数 | 防止慢查询拖垮连接池 | 低 |
| 🟢 P3 | 批量插入 / 游标分页 | 高并发写入 / 大数据量场景 | 中 |
| 🟢 P3 | 覆盖索引 / 联合索引 | 精细化查询优化 | 高 |
