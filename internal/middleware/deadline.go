package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

const DefaultRequestTimeout = 5 * time.Minute

func RequestDeadline(timeout time.Duration) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
