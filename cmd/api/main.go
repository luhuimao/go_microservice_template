package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/luhuimao/microservice_mvp_demo/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/luhuimao/microservice_mvp_demo/internal/cache"
	pkgcache "github.com/luhuimao/microservice_mvp_demo/internal/pkg/cache"
	"github.com/luhuimao/microservice_mvp_demo/internal/pkg/database"
	"github.com/luhuimao/microservice_mvp_demo/internal/pkg/logger"
	"github.com/luhuimao/microservice_mvp_demo/internal/repository"
	"github.com/luhuimao/microservice_mvp_demo/internal/service"
	httpTransport "github.com/luhuimao/microservice_mvp_demo/internal/transport/http"
)

func main() {
	cfg := config.Load()
	log := logger.New()

	// 数据库
	db := database.NewMySQL(cfg)

	// Redis 缓存
	rdb := pkgcache.NewRedisClient(cfg)
	userCache := cache.NewUserCache(rdb)

	// 依赖注入
	userRepo := repository.NewUserRepository(db)
	userService := service.NewUserService(userRepo, userCache)

	r := gin.Default()
	httpTransport.RegisterRoutes(r, userService)

	srv := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: r,
	}

	go func() {
		log.Info("server starting on port " + cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err.Error())
		}
	}()

	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)

	log.Info("server stopped")
}
