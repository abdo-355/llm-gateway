package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Setenv("GATEWAY_API_KEY", "test-api-key-that-is-at-least-32-characters-long")
	os.Exit(m.Run())
}

func TestInitGCPCredentials_NoEnvVar(t *testing.T) {
	os.Unsetenv("GCP_SA_KEY_JSON")
	os.Unsetenv("GCP_CREDENTIALS_PATH")
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")

	err := InitGCPCredentials()

	assert.NoError(t, err)
	assert.Equal(t, "", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
}

func TestInitGCPCredentials_ValidJSON(t *testing.T) {
	credPath := "/tmp/test-gcp-sa-" + t.Name() + ".json"
	os.Setenv("GCP_SA_KEY_JSON", `{"type":"service_account","project_id":"test-project"}`)
	os.Setenv("GCP_CREDENTIALS_PATH", credPath)
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")

	defer os.Unsetenv("GCP_SA_KEY_JSON")
	defer os.Unsetenv("GCP_CREDENTIALS_PATH")
	os.Remove(credPath)

	err := InitGCPCredentials()

	require.NoError(t, err)
	assert.Equal(t, credPath, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))

	content, err := os.ReadFile(credPath)
	require.NoError(t, err)
	assert.Equal(t, `{"type":"service_account","project_id":"test-project"}`, string(content))

	info, err := os.Stat(credPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	os.Remove(credPath)
}

func TestInitGCPCredentials_InvalidJSON(t *testing.T) {
	os.Setenv("GCP_SA_KEY_JSON", `{"invalid": json}`)
	os.Unsetenv("GCP_CREDENTIALS_PATH")
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")

	defer os.Unsetenv("GCP_SA_KEY_JSON")

	err := InitGCPCredentials()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid JSON")
}

func TestInitGCPCredentials_CustomPath(t *testing.T) {
	credPath := "/tmp/test-creds-custom.json"
	os.Setenv("GCP_SA_KEY_JSON", `{}`)
	os.Setenv("GCP_CREDENTIALS_PATH", credPath)

	defer os.Unsetenv("GCP_SA_KEY_JSON")
	defer os.Unsetenv("GCP_CREDENTIALS_PATH")
	os.Remove(credPath)

	err := InitGCPCredentials()

	require.NoError(t, err)
	_, err = os.Stat(credPath)
	assert.NoError(t, err)

	os.Remove(credPath)
}

func TestInitGCPCredentials_DefaultPath(t *testing.T) {
	origVal := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	os.Setenv("GCP_SA_KEY_JSON", `{}`)
	os.Setenv("GCP_CREDENTIALS_PATH", "/tmp/test-gcp-default-creds.json")
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")

	defer os.Unsetenv("GCP_SA_KEY_JSON")
	defer os.Unsetenv("GCP_CREDENTIALS_PATH")
	os.Remove("/tmp/test-gcp-default-creds.json")

	err := InitGCPCredentials()

	require.NoError(t, err)
	assert.Equal(t, "/tmp/test-gcp-default-creds.json", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))

	os.Remove("/tmp/test-gcp-default-creds.json")

	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", origVal)
}

func TestGetGCPCredentialsPath(t *testing.T) {
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
	assert.Equal(t, "", GetGCPCredentialsPath())

	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/custom/path.json")
	defer os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
	assert.Equal(t, "/custom/path.json", GetGCPCredentialsPath())
}

func TestIsVertexAuthConfigured(t *testing.T) {
	tests := []struct {
		name        string
		projectEnv string
		credEnv   string
		expected  bool
	}{
		{"no_project", "", "", false},
		{"no_creds", "project-id", "", false},
		{"has_cred_file", "project-id", "/path/to/file.json", true},
		{"has_sa_key_json", "project-id", "GCP_SA_KEY_JSON_SET", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("GOOGLE_CLOUD_PROJECT")
			os.Unsetenv("GOOGLE_VERTEX_PROJECT_ID")
			os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
			os.Unsetenv("GCP_SA_KEY_JSON")

			if tt.credEnv == "GCP_SA_KEY_JSON_SET" {
				os.Setenv("GCP_SA_KEY_JSON", `{}`)
			} else if tt.credEnv != "" {
				os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", tt.credEnv)
			}

			if tt.projectEnv != "" {
				os.Setenv("GOOGLE_CLOUD_PROJECT", tt.projectEnv)
			}

			result := IsVertexAuthConfigured()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInitGCPCredentials_MissingDir(t *testing.T) {
	os.Setenv("GCP_SA_KEY_JSON", `{}`)
	os.Setenv("GCP_CREDENTIALS_PATH", "/nonexistent/nested/dir/creds.json")
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")

	defer os.Unsetenv("GCP_SA_KEY_JSON")
	defer os.Unsetenv("GCP_CREDENTIALS_PATH")
	os.RemoveAll("/nonexistent")

	err := InitGCPCredentials()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create")
}