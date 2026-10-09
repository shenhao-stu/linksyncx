package service

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// Diagnostics describe structure and consistency, never caller-provided values.
type gatewayDiagnostics struct {
	BodyBytes          int      `json:"body_bytes"`
	ValidJSON          bool     `json:"valid_json"`
	HeaderCount        int      `json:"header_count"`
	KnownHeaders       []string `json:"known_headers"`
	HasSystem          bool     `json:"has_system"`
	HasMetadataUserID  bool     `json:"has_metadata_user_id"`
	SessionConsistency string   `json:"session_consistency"`
}

func summarizeGatewayRequest(headers http.Header, body []byte) gatewayDiagnostics {
	result := gatewayDiagnostics{
		BodyBytes: len(body), ValidJSON: json.Valid(body), HeaderCount: len(headers),
		KnownHeaders: []string{}, SessionConsistency: "unavailable",
	}
	for _, name := range []string{
		"authorization", "x-api-key", "content-type", "accept", "accept-encoding",
		"user-agent", "x-app", "anthropic-version", "anthropic-beta",
		"x-stainless-lang", "x-stainless-package-version", "x-stainless-os", "x-stainless-arch",
		"x-stainless-runtime", "x-stainless-runtime-version", "x-stainless-retry-count",
		"x-stainless-timeout", "x-stainless-helper-method", "x-claude-code-session-id",
	} {
		if getHeaderRaw(headers, name) != "" {
			result.KnownHeaders = append(result.KnownHeaders, name)
		}
	}
	if !result.ValidJSON {
		return result
	}
	result.HasSystem = gjson.GetBytes(body, "system").Exists()
	userID := gjson.GetBytes(body, "metadata.user_id")
	result.HasMetadataUserID = userID.Exists()
	if userID.Type != gjson.String {
		return result
	}
	identity := ParseMetadataUserID(userID.String())
	var session string
	count := 0
	for name, values := range headers {
		if strings.EqualFold(name, "x-claude-code-session-id") {
			for _, value := range values {
				session = value
				count++
			}
		}
	}
	if count > 1 {
		result.SessionConsistency = "ambiguous"
		return result
	}
	if identity != nil && identity.SessionID != "" && session != "" {
		result.SessionConsistency = "mismatch"
		if session == identity.SessionID {
			result.SessionConsistency = "match"
		}
	}
	return result
}

func gatewayDiagnosticJSON(headers http.Header, body []byte) string {
	encoded, _ := json.Marshal(summarizeGatewayRequest(headers, body))
	return string(encoded)
}

func gatewayDiagnosticTag(tag string) string {
	switch tag {
	case "CLIENT_ORIGINAL", "UPSTREAM_FORWARD", "UPSTREAM_FORWARD_VERTEX_ANTHROPIC":
		return tag
	default:
		return "GATEWAY_REQUEST"
	}
}
