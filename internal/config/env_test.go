package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDotEnv(t *testing.T) {
	t.Setenv("EXISTING_KEY", "keep-me")

	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	err := os.WriteFile(envPath, []byte("# comment\nEMPTY=\nFOO=bar\nSPACED = value with spaces\nQUOTED=\"quoted value\"\nSINGLE='single value'\nWITH_EQUALS=a=b=c\nEXISTING_KEY=override-me\n"), 0o600)
	require.NoError(t, err)

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(tempDir))

	require.NoError(t, LoadDotEnv())

	assert.Equal(t, "bar", os.Getenv("FOO"))
	assert.Equal(t, "", os.Getenv("EMPTY"))
	assert.Equal(t, "value with spaces", os.Getenv("SPACED"))
	assert.Equal(t, "quoted value", os.Getenv("QUOTED"))
	assert.Equal(t, "single value", os.Getenv("SINGLE"))
	assert.Equal(t, "a=b=c", os.Getenv("WITH_EQUALS"))
	assert.Equal(t, "keep-me", os.Getenv("EXISTING_KEY"))
}

func TestLoadDotEnvMissingFileIsIgnored(t *testing.T) {
	tempDir := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(tempDir))

	require.NoError(t, LoadDotEnv())
}

func TestLoadDotEnvInvalidLineFails(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	err := os.WriteFile(envPath, []byte("NOT_AN_ASSIGNMENT\n"), 0o600)
	require.NoError(t, err)

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(tempDir))

	err = LoadDotEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse .env line 1")
}
