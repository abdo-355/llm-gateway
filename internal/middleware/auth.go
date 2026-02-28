package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/gin-gonic/gin"
)

func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			metrics.AuthRejectionsTotal.WithLabelValues("missing_auth").Inc()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"code":    "MISSING_AUTH",
					"message": "Authorization header is required",
				},
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			metrics.AuthRejectionsTotal.WithLabelValues("invalid_format").Inc()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"code":    "INVALID_TOKEN_FORMAT",
					"message": "Authorization header must be 'Bearer <token>'",
				},
			})
			return
		}

		token := parts[1]
		expected := config.GetEnv().GatewayAPIKey
		if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			metrics.AuthRejectionsTotal.WithLabelValues("invalid_token").Inc()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"code":    "INVALID_TOKEN",
					"message": "Invalid API key",
				},
			})
			return
		}

		c.Next()
	}
}
