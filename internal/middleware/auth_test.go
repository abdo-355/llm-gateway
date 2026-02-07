package middleware

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

const testAPIKey = "test-api-key-that-is-at-least-32-characters-long"

func TestMain(m *testing.M) {
	os.Setenv("GATEWAY_API_KEY", testAPIKey)
	os.Setenv("GROQ_API_KEY", "test-groq-key")
	os.Setenv("CEREBRAS_API_KEY", "test-cerebras-key")
	os.Setenv("MISTRAL_API_KEY", "test-mistral-key")
	os.Exit(m.Run())
}

func TestAuth_HealthBypass(t *testing.T) {
	r := gin.New()
	r.Use(Auth())
	called := false
	r.GET("/health", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	w := performRequest(r, "GET", "/health", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)
}

func TestAuth_MissingAuthHeader(t *testing.T) {
	r := gin.New()
	r.Use(Auth())
	r.GET("/api/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := performRequest(r, "GET", "/api/test", nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "MISSING_AUTH", errObj["code"])
}

func TestAuth_InvalidTokenFormat(t *testing.T) {
	r := gin.New()
	r.Use(Auth())
	r.GET("/api/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := performRequest(r, "GET", "/api/test", map[string]string{
		"Authorization": "Basic some-token",
	})

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_TOKEN_FORMAT", errObj["code"])
}

func TestAuth_WrongToken(t *testing.T) {
	r := gin.New()
	r.Use(Auth())
	r.GET("/api/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := performRequest(r, "GET", "/api/test", map[string]string{
		"Authorization": "Bearer wrong-token",
	})

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_TOKEN", errObj["code"])
}

func TestAuth_ValidToken(t *testing.T) {
	r := gin.New()
	r.Use(Auth())
	called := false
	r.GET("/api/test", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	w := performRequest(r, "GET", "/api/test", map[string]string{
		"Authorization": "Bearer " + testAPIKey,
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called)
}
