package router

import (
	"GopherAI/controller/file"

	"github.com/gin-gonic/gin"
)

func DocumentRouter(r *gin.RouterGroup) {
	r.GET("", file.ListDocuments)
	r.GET("/:id", file.GetDocument)
}
