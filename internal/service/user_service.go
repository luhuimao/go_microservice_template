package service

import (
	"github.com/luhuimao/microservice_mvp_demo/internal/domain"
	"github.com/luhuimao/microservice_mvp_demo/internal/repository"
)

type UserService interface {
	Create(name string, age int) error
	Get(id uint) (*domain.User, error)
}

type userService struct {
	repo repository.UserRepository
}

func NewUserService(r repository.UserRepository) UserService {
	return &userService{repo: r}
}

func (s *userService) Create(name string, age int) error {
	return s.repo.Create(&domain.User{
		Name: name,
		Age:  age,
	})
}

func (s *userService) Get(id uint) (*domain.User, error) {
	return s.repo.FindByID(id)
}
