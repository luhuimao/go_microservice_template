package service

import (
	"context"
	"errors"

	"github.com/luhuimao/microservice_mvp_demo/internal/cache"
	"github.com/luhuimao/microservice_mvp_demo/internal/domain"
	"github.com/luhuimao/microservice_mvp_demo/internal/repository"
)

type UserService interface {
	Create(name string, age int) error
	Get(id uint) (*domain.User, error)
}

type userService struct {
	repo  repository.UserRepository
	cache cache.UserCache
}

// NewUserService 创建 UserService。cache 参数可为 nil（退化为无缓存模式）。
func NewUserService(r repository.UserRepository, c cache.UserCache) UserService {
	return &userService{repo: r, cache: c}
}

func (s *userService) Create(name string, age int) error {
	return s.repo.Create(&domain.User{
		Name: name,
		Age:  age,
	})
}

// Get 实现 Cache-Aside 旁路缓存模式：
//  1. 先查 Redis 缓存；命中直接返回
//  2. 未命中查 MySQL；查到后回填缓存
func (s *userService) Get(id uint) (*domain.User, error) {
	ctx := context.Background()

	// 1. 查缓存
	if s.cache != nil {
		user, err := s.cache.Get(ctx, id)
		if err == nil {
			return user, nil // 缓存命中
		}
		if !errors.Is(err, cache.ErrCacheMiss) {
			// 缓存出错，降级继续查 DB（不返回错误，保证可用性）
			_ = err
		}
	}

	// 2. 查数据库
	user, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}

	// 3. 回填缓存（失败不影响主流程）
	if s.cache != nil {
		_ = s.cache.Set(ctx, id, user, 0)
	}

	return user, nil
}
