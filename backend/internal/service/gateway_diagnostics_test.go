package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayDiagnosticsExcludeCallerValuesWithoutMutatingRequest(t *testing.T) {
	const marker = "private-caller-marker"
	session := "11111111-2222-4333-8444-555555555555"
	payload := map[string]any{
		"system": marker, "messages": []any{map[string]any{"content": marker}},
		"metadata": map[string]any{"user_id": FormatMetadataUserID(marker, marker, session, "2.1.280")},
	}
	for i := range 250 {
		payload[fmt.Sprintf("%s-%d", marker, i)] = marker
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "https://"+marker+":"+marker+"@example.invalid/"+marker+"?token="+marker, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header = http.Header{
		"Authorization": {"Bearer " + marker}, "Cookie": {marker}, "Proxy-Authorization": {marker},
		"User-Agent": {marker}, "X-Api-Key": {marker}, marker: {marker},
		"x-claude-code-session-id": {session},
	}
	headersBefore, bodyBefore := req.Header.Clone(), bytes.Clone(body)
	line := buildClaudeMimicDebugLine(req, body, &Account{ID: 42, Name: marker}, marker, false)
	require.NotContains(t, line, marker)
	require.NotContains(t, line, session)
	require.Contains(t, line, "account_id=42")
	require.Contains(t, line, "token_type=other")
	require.Contains(t, line, `"session_consistency":"match"`)
	require.Equal(t, headersBefore, req.Header)
	require.Equal(t, bodyBefore, body)

	req.Header.Set("x-claude-code-session-id", "different-session")
	require.Equal(t, "ambiguous", summarizeGatewayRequest(req.Header, body).SessionConsistency)
	setHeaderRaw(req.Header, "x-claude-code-session-id", "different-session")
	require.Equal(t, "mismatch", summarizeGatewayRequest(req.Header, body).SessionConsistency)
	req.Header["x-claude-code-session-id"] = []string{session, session}
	require.Equal(t, "ambiguous", summarizeGatewayRequest(req.Header, body).SessionConsistency)
	for _, malformed := range [][]byte{nil, []byte(marker), []byte(`{"system":"` + marker)} {
		line := buildClaudeMimicDebugLine(req, malformed, nil, "oauth", true)
		require.NotContains(t, line, marker)
		require.Contains(t, line, `"valid_json":false`)
		require.Contains(t, line, `"session_consistency":"unavailable"`)
	}
	require.Empty(t, buildClaudeMimicDebugLine(nil, body, nil, "oauth", false))
}

func TestGatewayDebugFileRestrictsPermissionsAndNeverWritesRawSnapshots(t *testing.T) {
	const marker = "private-snapshot-marker"
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nested", "gateway.log")
			if existing {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
				require.NoError(t, os.WriteFile(path, nil, 0644))
				require.NoError(t, os.Chmod(path, 0644))
			}
			service := &GatewayService{}
			service.initDebugGatewayBodyFile(path)
			file := service.debugGatewayBodyFile.Load()
			require.NotNil(t, file)
			t.Cleanup(func() { require.NoError(t, file.Close()) })
			service.debugLogGatewaySnapshot(marker, http.Header{"Cookie": {marker}}, []byte(marker))
			service.debugLogGatewaySnapshot("CLIENT_ORIGINAL", http.Header{"User-Agent": {marker}}, []byte(`{"system":"`+marker+`"}`))
			contents, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NotContains(t, string(contents), marker)
			require.Contains(t, string(contents), "GATEWAY_REQUEST")
			require.Contains(t, string(contents), "CLIENT_ORIGINAL")
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0600), info.Mode().Perm())
			if !existing {
				info, err := os.Stat(filepath.Dir(path))
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0700), info.Mode().Perm())
			}
		})
	}
}
