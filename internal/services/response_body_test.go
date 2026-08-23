package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadBoundedResponseBody(t *testing.T) {
	t.Run("reads a response within the limit", func(t *testing.T) {
		body, err := readBoundedResponseBody(strings.NewReader("response"), 8)

		require.NoError(t, err)
		assert.Equal(t, "response", string(body))
	})

	t.Run("returns only the bounded prefix when oversized", func(t *testing.T) {
		body, err := readBoundedResponseBody(strings.NewReader("oversized"), 4)

		require.Error(t, err)
		assert.Equal(t, "over", string(body))
	})
}
