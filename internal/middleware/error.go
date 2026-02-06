package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			requestID := c.GetString("request_id")

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
