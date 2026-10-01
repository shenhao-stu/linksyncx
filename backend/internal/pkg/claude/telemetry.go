package claude

import "net/http"

const (
	EventLoggingPath   = "/api/event_logging/batch"
	EventLoggingV2Path = "/api/event_logging/v2/batch"
)

// IsDiscardedTelemetryRequest identifies only the local, non-forwarding sinks.
func IsDiscardedTelemetryRequest(method, path string) bool {
	return method == http.MethodPost && (path == EventLoggingPath || path == EventLoggingV2Path)
}
