package middleware

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCORS_RegularRequest(t *testing.T) {
	r := gin.New()
	r.Use(CORS())
	called := false
	r.GET("/api/test", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	w := performRequest(r, "GET", "/api/test", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Authorization, Content-Type", w.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORS_OptionsRequest(t *testing.T) {
	r := gin.New()
	r.Use(CORS())
	called := false
	r.OPTIONS("/api/test", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	w := performRequest(r, "OPTIONS", "/api/test", nil)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.False(t, called)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Authorization, Content-Type", w.Header().Get("Access-Control-Allow-Headers"))
}
