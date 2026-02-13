package database

import (
	"github.com/luhuimao/microservice_mvp_demo/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func NewMySQL(cfg *config.Config) *gorm.DB {
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	return db
}
