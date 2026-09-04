package proxy

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const healthPath = "/healthz"

func healthHandler(store *managedTenantStore, logger *zap.SugaredLogger) gin.HandlerFunc {
	return func(request *gin.Context) {
		request.Header("Cache-Control", "no-store")
		probeContext, cancel := context.WithTimeout(request.Request.Context(), time.Second)
		defer cancel()
		if err := store.database.checkHealth(probeContext); err != nil {
			logger.Errorw("health probe failed", "error", err)
			request.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		request.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
