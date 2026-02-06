package middleware

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRateLimit_HealthBypass(t *testing.T) {
	client, _ := newTestRedis(t)
	rl := &RateLimiter{
		redis:       client,
		maxRequests: 1,
		windowMs:    60000,
	}

	r := gin.New()
	r.Use(rl.RateLimit())
	called := false
	r.GET("/health", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	w := performRequest(r, "GET", "/health", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)
}

func TestRateLimit_UnderLimit(t *testing.T) {
	client, _ := newTestRedis(t)
	rl := &RateLimiter{
		redis:       client,
		maxRequests: 5,
		windowMs:    60000,
	}

	r := gin.New()
	r.Use(rl.RateLimit())
	r.GET("/api/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := performRequest(r, "GET", "/api/test", nil)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimit_AtLimit(t *testing.T) {
	client, _ := newTestRedis(t)
	rl := &RateLimiter{
		redis:       client,
		maxRequests: 3,
		windowMs:    60000,
	}

	r := gin.New()
	r.Use(rl.RateLimit())
	r.GET("/api/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 0; i < 3; i++ {
		w := performRequest(r, "GET", "/api/test", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		time.Sleep(2 * time.Millisecond)
	}

	w := performRequest(r, "GET", "/api/test", nil)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}
