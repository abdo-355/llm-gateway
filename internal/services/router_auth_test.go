package services

import (
	"net/http"
	"testing"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/stretchr/testify/assert"
)

func TestIsAuthProviderError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		message    string
		wantAuth   bool
	}{
		{
			name:       "401 unauthorized is auth error",
			statusCode: http.StatusUnauthorized,
			message:    "HTTP error 401: Unauthorized",
			wantAuth:   true,
		},
		{
			name:       "403 forbidden is auth error",
			statusCode: http.StatusForbidden,
			message:    "HTTP error 403: Forbidden",
			wantAuth:   true,
		},
		{
			name:       "401 model not supported is not auth error",
			statusCode: http.StatusUnauthorized,
			message:    "HTTP error 401: Model hy3-free is not supported",
			wantAuth:   false,
		},
		{
			name:       "401 model not found is not auth error",
			statusCode: http.StatusUnauthorized,
			message:    "HTTP error 401: Model not found",
			wantAuth:   false,
		},
		{
			name:       "404 not found is not auth error",
			statusCode: http.StatusNotFound,
			message:    "HTTP error 404: Not Found",
			wantAuth:   false,
		},
		{
			name:       "500 internal server error is not auth error",
			statusCode: http.StatusInternalServerError,
			message:    "HTTP error 500",
			wantAuth:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &errors.ProviderError{
				StatusCode: tt.statusCode,
				Message:    tt.message,
			}
			assert.Equal(t, tt.wantAuth, isAuthProviderError(err))
		})
	}
}
