package services

import (
	"context"
	"fmt"
	"sync"

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
	return vertexAuth.err
}

func GetVertexToken(ctx context.Context) (string, error) {
	if vertexAuth.ts == nil {
		return "", fmt.Errorf("Vertex ADC not initialized; call InitVertexAuth first")
	}
	tok, err := vertexAuth.ts.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get Vertex access token: %w", err)
	}
	return tok.AccessToken, nil
}
