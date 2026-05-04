package middleware

import (
	"net/http"

	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			requestID := requestid.Get(c)

			logger.Error().
				Str("type", "middleware").
				Str("event", "middleware.error").
				Str("request_id", requestID).
				Err(err.Err).
				Msg("Unhandled middleware error")

			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"type":       "internal_error",
					"code":       "INTERNAL_ERROR",
					"message":    err.Error(),
					"request_id": requestID,
				},
			})
		}
	}
}
