package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luhuimao/microservice_mvp_demo/internal/cache"
	"github.com/luhuimao/microservice_mvp_demo/internal/domain"
	"github.com/luhuimao/microservice_mvp_demo/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ──────────────────────────────────────────────
// Mock: UserRepository
// ──────────────────────────────────────────────

type mockUserRepo struct {
	createErr error
	findUser  *domain.User
	findErr   error
	findCalls int
}

func (m *mockUserRepo) Create(user *domain.User) error { return m.createErr }
func (m *mockUserRepo) FindByID(id uint) (*domain.User, error) {
	m.findCalls++
	return m.findUser, m.findErr
}

// ──────────────────────────────────────────────
// Mock: UserCache
// ──────────────────────────────────────────────

type mockUserCache struct {
	getUser  *domain.User
	getErr   error
	setCalls int
	delCalls int
}

func (m *mockUserCache) Get(_ context.Context, _ uint) (*domain.User, error) {
	return m.getUser, m.getErr
}
func (m *mockUserCache) Set(_ context.Context, _ uint, _ *domain.User, _ time.Duration) error {
	m.setCalls++
	return nil
}
func (m *mockUserCache) Del(_ context.Context, _ uint) error {
	m.delCalls++
	return nil
}

// ──────────────────────────────────────────────
// Test: Get — 缓存命中（不查 DB）
// ──────────────────────────────────────────────

func TestUserService_Get_CacheHit(t *testing.T) {
	cachedUser := &domain.User{ID: 1, Name: "Alice", Age: 30}

	repo := &mockUserRepo{}
	mc := &mockUserCache{getUser: cachedUser, getErr: nil}
	svc := service.NewUserService(repo, mc)

	got, err := svc.Get(1)
	require.NoError(t, err)
	assert.Equal(t, cachedUser, got)

	// 缓存命中，DB 不应被调用
	assert.Equal(t, 0, repo.findCalls, "缓存命中时不应查询数据库")
	// 回填 Set 不应被调用
	assert.Equal(t, 0, mc.setCalls, "缓存命中时不应调用 Set")
}

// ──────────────────────────────────────────────
// Test: Get — 缓存未命中，查 DB 并回填
// ──────────────────────────────────────────────

func TestUserService_Get_CacheMiss_WriteThrough(t *testing.T) {
	dbUser := &domain.User{ID: 2, Name: "Bob", Age: 25}

	repo := &mockUserRepo{findUser: dbUser}
	mc := &mockUserCache{getErr: cache.ErrCacheMiss}
	svc := service.NewUserService(repo, mc)

	got, err := svc.Get(2)
	require.NoError(t, err)
	assert.Equal(t, dbUser, got)

	// DB 应被查询一次
	assert.Equal(t, 1, repo.findCalls, "缓存未命中时应查询数据库")
	// 应回填缓存
	assert.Equal(t, 1, mc.setCalls, "缓存未命中时应调用 Set 回填")
}

// ──────────────────────────────────────────────
// Test: Get — 缓存出错时降级查 DB（保证可用性）
// ──────────────────────────────────────────────

func TestUserService_Get_CacheError_Fallback(t *testing.T) {
	dbUser := &domain.User{ID: 3, Name: "Charlie", Age: 20}

	repo := &mockUserRepo{findUser: dbUser}
	mc := &mockUserCache{getErr: errors.New("redis connection refused")}
	svc := service.NewUserService(repo, mc)

	got, err := svc.Get(3)
	require.NoError(t, err, "缓存出错时应降级而非返回 error")
	assert.Equal(t, dbUser, got)
	assert.Equal(t, 1, repo.findCalls, "降级后应查询数据库")
}

// ──────────────────────────────────────────────
// Test: Get — DB 查询失败
// ──────────────────────────────────────────────

func TestUserService_Get_DBError(t *testing.T) {
	dbErr := errors.New("record not found")

	repo := &mockUserRepo{findErr: dbErr}
	mc := &mockUserCache{getErr: cache.ErrCacheMiss}
	svc := service.NewUserService(repo, mc)

	_, err := svc.Get(99)
	assert.ErrorIs(t, err, dbErr, "DB 出错时应向上传递错误")
	assert.Equal(t, 0, mc.setCalls, "DB 查询失败时不应写入缓存")
}

// ──────────────────────────────────────────────
// Test: Get — 无缓存（nil cache）退化为纯 DB 模式
// ──────────────────────────────────────────────

func TestUserService_Get_NilCache(t *testing.T) {
	dbUser := &domain.User{ID: 5, Name: "Eve", Age: 28}

	repo := &mockUserRepo{findUser: dbUser}
	svc := service.NewUserService(repo, nil) // nil cache = 无缓存模式

	got, err := svc.Get(5)
	require.NoError(t, err)
	assert.Equal(t, dbUser, got)
	assert.Equal(t, 1, repo.findCalls)
}

// ──────────────────────────────────────────────
// Test: Create — 成功创建用户
// ──────────────────────────────────────────────

func TestUserService_Create_Success(t *testing.T) {
	repo := &mockUserRepo{}
	svc := service.NewUserService(repo, nil)

	err := svc.Create("Frank", 35)
	require.NoError(t, err)
}

// ──────────────────────────────────────────────
// Test: Create — DB 写入失败
// ──────────────────────────────────────────────

func TestUserService_Create_DBError(t *testing.T) {
	dbErr := errors.New("duplicate entry")
	repo := &mockUserRepo{createErr: dbErr}
	svc := service.NewUserService(repo, nil)

	err := svc.Create("Frank", 35)
	assert.ErrorIs(t, err, dbErr)
}
