package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClientVersionsHandlerRoundtrip(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{"unrelated": "keep"})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/client-versions", nil)
	h.GetClientVersions(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"auto_sync":false`)
	choices := []service.ClientVersionChoice{
		{ID: "grok_cli", CustomVersion: xai.CLIClientVersion},
		{ID: "claude_cli", CustomVersion: claude.CLICurrentVersion},
		{ID: "claude_sdk", CustomVersion: claude.SDKTSVersion},
	}
	for _, valid := range []bool{true, false} {
		if !valid {
			choices[2].CustomVersion = "invalid\r\nInjected:value"
		}
		body, err := json.Marshal(choices)
		require.NoError(t, err)
		rec = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/client-versions", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		h.UpdateClientVersions(c)
		if valid {
			require.Equal(t, http.StatusOK, rec.Code)
		} else {
			require.Equal(t, http.StatusBadRequest, rec.Code)
		}
	}
	require.Equal(t, "keep", repo.values["unrelated"])
	require.Equal(t, claude.SDKTSVersion, repo.values["claude_sdk_client_version"])
}
