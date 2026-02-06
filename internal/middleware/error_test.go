package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestErrorHandler_NoErrors(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/api/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := performRequest(r, "GET", "/api/test", nil)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
}

func TestErrorHandler_WithGinError(t *testing.T) {
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/api/test", func(c *gin.Context) {
		_ = c.Error(errors.New("something went wrong"))
	})

	w := performRequest(r, "GET", "/api/test", nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	assert.NoError(t, err)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "internal_error", errObj["type"])
	assert.Equal(t, "INTERNAL_ERROR", errObj["code"])
	assert.Equal(t, "something went wrong", errObj["message"])
}
