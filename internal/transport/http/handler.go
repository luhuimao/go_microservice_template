package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luhuimao/microservice_mvp_demo/internal/service"
)

func RegisterRoutes(r *gin.Engine, svc service.UserService) {
	r.POST("/users", func(c *gin.Context) {
		var req struct {
			Name string
			Age  int
		}
		c.BindJSON(&req)
		err := svc.Create(req.Name, req.Age)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"status": "ok"})
	})

	r.GET("/users/:id", func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		user, err := svc.Get(uint(id))
		if err != nil {
			c.JSON(404, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, user)
	})
}
