package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDiscardedTelemetryDoesNotCreateIdentifyingAccessLogs(t *testing.T) {
	sink := initMiddlewareTestLogger(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestLogger(), Logger())
	for _, path := range []string{claude.EventLoggingPath, claude.EventLoggingV2Path} {
		router.POST(path, func(c *gin.Context) { c.Status(http.StatusOK) })
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader("private-body"))
		request.Header.Set("X-Request-ID", "private-correlation-id")
		request.Header.Set("Authorization", "Bearer private-token")
		request.RemoteAddr = "192.0.2.25:443"
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.Empty(t, recorder.Header().Get("X-Request-ID"))
		require.Empty(t, sink.list())
	}
	// Adjacent paths and different methods retain ordinary request diagnostics.
	for _, target := range [][2]string{{http.MethodPost, claude.EventLoggingV2Path + "/extra"}, {http.MethodGet, claude.EventLoggingPath}} {
		before := len(sink.list())
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(target[0], target[1], nil))
		require.NotEmpty(t, recorder.Header().Get("X-Request-ID"))
		require.Greater(t, len(sink.list()), before)
	}
}
