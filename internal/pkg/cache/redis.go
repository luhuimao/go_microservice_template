package cache

import (
	"context"
	"fmt"

	"github.com/luhuimao/microservice_mvp_demo/internal/config"
	"github.com/redis/go-redis/v9"
)

// NewRedisClient 根据配置创建并验证 Redis 客户端连接。
// 若连接失败则 panic，与 MySQL 初始化行为保持一致。
func NewRedisClient(cfg *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		panic(fmt.Sprintf("failed to connect to redis: %v", err))
	}

	return rdb
}
