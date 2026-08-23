package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestDeadlineAddsEndToEndDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestDeadline(time.Second))
	r.GET("/", func(c *gin.Context) {
		deadline, ok := c.Request.Context().Deadline()
		require.True(t, ok)
		require.WithinDuration(t, time.Now().Add(time.Second), deadline, 100*time.Millisecond)
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusNoContent, w.Code)
}
