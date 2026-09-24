package router

import (
	"commercial-diving-decompression-control/backend/internal/handler"
	"github.com/gin-gonic/gin"
)

func RegisterDecompressionAssessmentRoutes(api *gin.RouterGroup, h *handler.DecompressionAssessmentHandler, write, review gin.HandlerFunc) {
	api.POST("/plans/:id/assessments/run", write, h.Run)
	// A single archived check is read through a top-level resource so it never
	// shares a path position with the /assessments/:id wildcard. Read access is
	// available to every authenticated role; creating a check requires write.
	api.GET("/sensitivity-checks/:id", h.GetSensitivityCheck)
	assessments := api.Group("/assessments")
	assessments.GET("", h.List)
	assessments.GET("/:id", h.Get)
	assessments.GET("/:id/compare", h.Compare)
	assessments.POST("/:id/submit", write, h.Submit)
	assessments.POST("/:id/approve", review, h.Approve)
	assessments.GET("/:id/sensitivity-checks", h.ListSensitivityChecks)
	assessments.POST("/:id/sensitivity-checks", write, h.RunSensitivityCheck)
}
