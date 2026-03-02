package cache_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/luhuimao/microservice_mvp_demo/internal/cache"
	"github.com/luhuimao/microservice_mvp_demo/internal/domain"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestCache 启动一个内嵌的 miniredis，返回 UserCache 和清理函数。
func newTestCache(t *testing.T) (cache.UserCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return cache.NewUserCache(rdb), mr
}

// ──────────────────────────────────────────────
// Set / Get — 正常路径
// ──────────────────────────────────────────────

func TestUserCache_SetAndGet(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	user := &domain.User{ID: 1, Name: "Alice", Age: 30}

	// Set
	err := c.Set(ctx, user.ID, user, 5*time.Minute)
	require.NoError(t, err)

	// Get — 应命中缓存
	got, err := c.Get(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user, got)
}

// ──────────────────────────────────────────────
// Get — 缓存未命中
// ──────────────────────────────────────────────

func TestUserCache_Get_Miss(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	_, err := c.Get(ctx, 999)
	assert.ErrorIs(t, err, cache.ErrCacheMiss, "应返回 ErrCacheMiss")
}

// ──────────────────────────────────────────────
// Del — 删除后 Get 应返回 Miss
// ──────────────────────────────────────────────

func TestUserCache_Del(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	user := &domain.User{ID: 2, Name: "Bob", Age: 25}
	require.NoError(t, c.Set(ctx, user.ID, user, 0)) // 0 → 默认 TTL

	// 删除
	require.NoError(t, c.Del(ctx, user.ID))

	// 再次 Get 应 Miss
	_, err := c.Get(ctx, user.ID)
	assert.ErrorIs(t, err, cache.ErrCacheMiss)
}

// ──────────────────────────────────────────────
// TTL — key 过期后应 Miss
// ──────────────────────────────────────────────

func TestUserCache_TTL_Expiry(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	user := &domain.User{ID: 3, Name: "Charlie", Age: 20}
	require.NoError(t, c.Set(ctx, user.ID, user, 2*time.Second))

	// 命中
	_, err := c.Get(ctx, user.ID)
	require.NoError(t, err)

	// miniredis 快进时间
	mr.FastForward(3 * time.Second)

	// 过期后应 Miss
	_, err = c.Get(ctx, user.ID)
	assert.ErrorIs(t, err, cache.ErrCacheMiss, "TTL 到期后应返回 ErrCacheMiss")
}

// ──────────────────────────────────────────────
// 数据完整性 — Set 写入的 JSON 可被解析为 User
// ──────────────────────────────────────────────

func TestUserCache_DataIntegrity(t *testing.T) {
	_, mr := newTestCache(t)
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	c := cache.NewUserCache(rdb)

	user := &domain.User{ID: 10, Name: "Dave", Age: 40}
	require.NoError(t, c.Set(ctx, user.ID, user, time.Minute))

	raw, err := rdb.Get(ctx, "user:10").Bytes()
	require.NoError(t, err)

	var got domain.User
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, *user, got)
}
