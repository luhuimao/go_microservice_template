package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/luhuimao/microservice_mvp_demo/internal/domain"
	"github.com/redis/go-redis/v9"
)

const defaultUserTTL = 5 * time.Minute

// ErrCacheMiss 表示缓存中不存在该 key，调用方应回查数据库。
var ErrCacheMiss = errors.New("cache miss")

// UserCache 定义用户缓存的读写接口。
type UserCache interface {
	// Get 从缓存中获取用户，未命中时返回 ErrCacheMiss。
	Get(ctx context.Context, id uint) (*domain.User, error)
	// Set 将用户写入缓存，ttl 为 0 时使用默认 TTL。
	Set(ctx context.Context, id uint, user *domain.User, ttl time.Duration) error
	// Del 从缓存中删除用户条目（用于写操作后的缓存失效）。
	Del(ctx context.Context, id uint) error
}

type userRedisCache struct {
	rdb *redis.Client
}

// NewUserCache 创建基于 Redis 的 UserCache 实现。
func NewUserCache(rdb *redis.Client) UserCache {
	return &userRedisCache{rdb: rdb}
}

func userKey(id uint) string {
	return fmt.Sprintf("user:%d", id)
}

func (c *userRedisCache) Get(ctx context.Context, id uint) (*domain.User, error) {
	val, err := c.rdb.Get(ctx, userKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, fmt.Errorf("redis get user %d: %w", id, err)
	}

	var user domain.User
	if err = json.Unmarshal(val, &user); err != nil {
		return nil, fmt.Errorf("unmarshal user %d: %w", id, err)
	}
	return &user, nil
}

func (c *userRedisCache) Set(ctx context.Context, id uint, user *domain.User, ttl time.Duration) error {
	if ttl == 0 {
		ttl = defaultUserTTL
	}
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("marshal user %d: %w", id, err)
	}
	if err = c.rdb.Set(ctx, userKey(id), data, ttl).Err(); err != nil {
		return fmt.Errorf("redis set user %d: %w", id, err)
	}
	return nil
}

func (c *userRedisCache) Del(ctx context.Context, id uint) error {
	if err := c.rdb.Del(ctx, userKey(id)).Err(); err != nil {
		return fmt.Errorf("redis del user %d: %w", id, err)
	}
	return nil
}
