package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const defaultGCPCredentialsPath = "/app/secrets/gcp-sa.json"

func InitGCPCredentials() error {
	keyJSON := os.Getenv("GCP_SA_KEY_JSON")
	if keyJSON == "" {
		return nil
	}

	if !json.Valid([]byte(keyJSON)) {
		return fmt.Errorf("GCP_SA_KEY_JSON is not valid JSON")
	}

	credPath := os.Getenv("GCP_CREDENTIALS_PATH")
	if credPath == "" {
		credPath = defaultGCPCredentialsPath
	}

	dir := filepath.Dir(credPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory %s: %w", dir, err)
	}

	if err := os.WriteFile(credPath, []byte(keyJSON), 0600); err != nil {
		return fmt.Errorf("failed to write credentials file %s: %w", credPath, err)
	}

	if err := os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credPath); err != nil {
		return fmt.Errorf("failed to set GOOGLE_APPLICATION_CREDENTIALS: %w", err)
	}

	return nil
}

func GetGCPCredentialsPath() string {
	return os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
}

func IsVertexAuthConfigured() bool {
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		project = os.Getenv("GOOGLE_VERTEX_PROJECT_ID")
	}
	hasCreds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" || os.Getenv("GCP_SA_KEY_JSON") != ""
	return project != "" && hasCreds
}