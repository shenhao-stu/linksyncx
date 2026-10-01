package routes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type unreadTelemetryBody struct{ reads int }

func (b *unreadTelemetryBody) Read([]byte) (int, error) {
	b.reads++
	return 0, io.EOF
}

func (*unreadTelemetryBody) Close() error { return nil }

func TestTelemetrySinksAcknowledgeWithoutReadingOrEchoingPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterCommonRoutes(router)
	for _, path := range []string{claude.EventLoggingPath, claude.EventLoggingV2Path} {
		t.Run(path, func(t *testing.T) {
			body := &unreadTelemetryBody{}
			request := httptest.NewRequest(http.MethodPost, path+"?token=private-marker", nil)
			request.Body = body
			request.Header.Set("Authorization", "Bearer private-marker")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Empty(t, recorder.Body.String())
			require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			require.Zero(t, body.reads)
		})
	}
	for _, path := range []string{"/api/event_logging/v3/batch", claude.EventLoggingV2Path + "/extra"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		require.Equal(t, http.StatusNotFound, recorder.Code)
	}
}
