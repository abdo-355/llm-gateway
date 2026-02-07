package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/abdo-355/llm-gateway/internal/logger"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type VertexTokenSource struct {
	ts   oauth2.TokenSource
	once sync.Once
	err  error
}

var vertexAuth = &VertexTokenSource{}

func InitVertexAuth(ctx context.Context) error {
	vertexAuth.once.Do(func() {
		ts, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			vertexAuth.err = fmt.Errorf("failed to create Vertex token source: %w", err)
			return
		}
		vertexAuth.ts = oauth2.ReuseTokenSource(nil, ts)
	})

	if vertexAuth.err != nil {
		logger.Warn().
			Str("type", "auth").
			Str("event", "vertex.adc_init_failed").
			Err(vertexAuth.err).
			Msg("Vertex ADC initialization failed; Vertex provider will be unavailable")
	} else {
		logger.Info().
			Str("type", "auth").
			Str("event", "vertex.adc_init_success").
			Msg("Vertex ADC initialized successfully")
	}

	return vertexAuth.err
}

func GetVertexToken(ctx context.Context) (string, error) {
	if vertexAuth.ts == nil {
		return "", fmt.Errorf("Vertex ADC not initialized; call InitVertexAuth first")
	}
	tok, err := vertexAuth.ts.Token()
	if err != nil {
		logger.Error().
			Str("type", "auth").
			Str("event", "vertex.token_refresh_failed").
			Err(err).
			Msg("Failed to refresh Vertex access token")
		return "", fmt.Errorf("failed to get Vertex access token: %w", err)
	}
	logger.Debug().
		Str("type", "auth").
		Str("event", "vertex.token_acquired").
		Time("expiry", tok.Expiry).
		Msg("Vertex access token acquired")
	return tok.AccessToken, nil
}
