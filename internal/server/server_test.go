package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPprofServerUsesDedicatedLocalListenerAndMux(t *testing.T) {
	s := newPprofServer()
	assert.Equal(t, "127.0.0.1:6060", s.Addr)

	index := httptest.NewRecorder()
	s.Handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	assert.Equal(t, http.StatusOK, index.Code)

	unrelated := httptest.NewRecorder()
	s.Handler.ServeHTTP(unrelated, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Equal(t, http.StatusNotFound, unrelated.Code)
}
