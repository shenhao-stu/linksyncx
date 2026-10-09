package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type clientVersionRepo struct {
	SettingRepository
	mu     sync.Mutex
	values map[string]string
	err    error
	writes int
}

func (r *clientVersionRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string)
	for _, key := range keys {
		out[key] = r.values[key]
	}
	return out, r.err
}
func (r *clientVersionRepo) SetMultiple(_ context.Context, values map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	for key, value := range values {
		r.values[key] = value
	}
	r.writes++
	return nil
}
func versionTestService() (*SettingService, *clientVersionRepo) {
	repo := &clientVersionRepo{values: make(map[string]string)}
	return NewSettingService(repo, nil), repo
}
func defaultClientVersionChoices() []ClientVersionChoice {
	choices := make([]ClientVersionChoice, 0, 3)
	for _, spec := range clientVersionSpecs() {
		choices = append(choices, ClientVersionChoice{spec.id, spec.builtin, false})
	}
	return choices
}

func TestClientVersionResolutionAndDefaults(t *testing.T) {
	for _, spec := range clientVersionSpecs() {
		t.Run(spec.id, func(t *testing.T) {
			view := resolveClientVersion(spec, nil)
			require.False(t, view.AutoSync)
			require.Equal(t, spec.builtin, view.CustomVersion)
			values := map[string]string{spec.versionKey: spec.builtin, spec.syncedKey: "9.0.0"}
			require.Equal(t, spec.builtin, resolveClientVersion(spec, values).EffectiveVersion)
			values[spec.autoKey] = "true"
			require.Equal(t, "9.0.0", resolveClientVersion(spec, values).EffectiveVersion)
			values[spec.syncedKey] = "broken\r\nX-Injected: yes"
			require.Equal(t, spec.builtin, resolveClientVersion(spec, values).EffectiveVersion)
		})
	}
}

func TestClientVersionEditsAreAtomicAndInvalidateRuntime(t *testing.T) {
	svc, repo := versionTestService()
	ctx := context.Background()
	choices := defaultClientVersionChoices()
	require.Equal(t, xai.CLIClientVersion, svc.clientVersion(ctx, "grok_cli"))
	choices[0].CustomVersion = "9.0.0"
	require.NoError(t, svc.UpdateClientVersions(ctx, choices))
	require.Equal(t, "9.0.0", svc.clientVersion(ctx, "grok_cli"))
	for _, bad := range []string{"", "1", "1.0.0\r\nfoo:bar", "http://localhost", "999999999999999999999999999.1.2"} {
		copyChoices := append([]ClientVersionChoice(nil), choices...)
		copyChoices[1].CustomVersion = bad
		require.ErrorIs(t, svc.UpdateClientVersions(ctx, copyChoices), ErrInvalidClientVersions)
	}
	require.Equal(t, 1, repo.writes)
	choices[2].ID = choices[0].ID
	require.ErrorIs(t, svc.UpdateClientVersions(ctx, choices), ErrInvalidClientVersions)
	require.ErrorIs(t, svc.UpdateClientVersions(ctx, choices[:1]), ErrInvalidClientVersions)
}

func TestClientVersionFailedRefreshPreservesCachedVersion(t *testing.T) {
	svc, repo := versionTestService()
	spec := clientVersionSpecs()[0]
	repo.values[spec.versionKey] = "9.0.0"
	require.Equal(t, "9.0.0", svc.clientVersion(context.Background(), spec.id))
	snapshot := svc.clientVersionCache.Load()
	svc.clientVersionCache.Store(&cachedClientVersions{snapshot.versions, time.Time{}})
	repo.err = errors.New("DB unavailable")
	require.Equal(t, "9.0.0", svc.clientVersion(context.Background(), spec.id))
	_, err := svc.GetClientVersions(context.Background())
	require.Error(t, err)
}

func TestClientVersionSyncRequiresExplicitOptIn(t *testing.T) {
	for _, flag := range []string{"", "false", "TRUE", "garbage"} {
		svc, repo := versionTestService()
		for _, spec := range clientVersionSpecs() {
			repo.values[spec.autoKey] = flag
		}
		worker := NewClientVersionSyncService(svc, time.Hour)
		worker.fetch = func(context.Context, clientVersionSpec) (string, error) {
			t.Fatal("outbound fetch without opt-in")
			return "", nil
		}
		worker.runOnce()
		repo.err = errors.New("DB down")
		worker.runOnce()
		require.Zero(t, repo.writes)
		worker.Stop()
	}
}

func TestClientVersionSyncAllClientsPreservesPinsAndThrottles(t *testing.T) {
	svc, repo := versionTestService()
	choices := defaultClientVersionChoices()
	for i := range choices {
		choices[i].AutoSync = true
	}
	require.NoError(t, svc.UpdateClientVersions(context.Background(), choices))
	worker := NewClientVersionSyncService(svc, time.Hour)
	defer worker.Stop()
	calls := 0
	worker.fetch = func(context.Context, clientVersionSpec) (string, error) { calls++; return "9.0.0", nil }
	worker.runOnce()
	worker.runOnce()
	require.Equal(t, 3, calls)
	for _, spec := range clientVersionSpecs() {
		require.Equal(t, spec.builtin, repo.values[spec.versionKey])
		require.Equal(t, "9.0.0", svc.clientVersion(context.Background(), spec.id))
	}
	// Persisted check times also throttle an immediate process restart.
	other := NewClientVersionSyncService(svc, time.Hour)
	defer other.Stop()
	other.fetch = worker.fetch
	other.runOnce()
	require.Equal(t, 3, calls)
}

func TestClientVersionSyncRetainsGoodReleaseOnFailures(t *testing.T) {
	for _, candidate := range []string{"", "1.0.0", "9.0.1-alpha.1", "garbage", "8.0.0", "9.0.0"} {
		svc, repo := versionTestService()
		spec := clientVersionSpecs()[0]
		repo.values[spec.autoKey], repo.values[spec.syncedKey] = "true", "9.0.0"
		worker := NewClientVersionSyncService(svc, time.Hour)
		worker.fetch = func(context.Context, clientVersionSpec) (string, error) { return candidate, nil }
		worker.syncClient(context.Background(), spec)
		require.Equal(t, "9.0.0", repo.values[spec.syncedKey])
		require.NotEmpty(t, repo.values[spec.statusKey()])
		worker.Stop()
	}
	svc, repo := versionTestService()
	spec := clientVersionSpecs()[1]
	repo.values[spec.autoKey], repo.values[spec.syncedKey] = "true", "9.0.0"
	worker := NewClientVersionSyncService(svc, time.Hour)
	defer worker.Stop()
	worker.fetch = func(context.Context, clientVersionSpec) (string, error) { return "", errors.New("private HTTP body") }
	worker.runOnce()
	require.Equal(t, "9.0.0", repo.values[spec.syncedKey])
	require.NotContains(t, repo.values[spec.statusKey()], "private")
}

func TestClientVersionDisableWhileFetchingDiscardsResponse(t *testing.T) {
	svc, repo := versionTestService()
	choices := defaultClientVersionChoices()
	choices[0].AutoSync = true
	require.NoError(t, svc.UpdateClientVersions(context.Background(), choices))
	worker := NewClientVersionSyncService(svc, time.Hour)
	defer worker.Stop()
	entered, finish := make(chan struct{}), make(chan struct{})
	worker.fetch = func(context.Context, clientVersionSpec) (string, error) {
		close(entered)
		<-finish
		return "9.0.0", nil
	}
	done := make(chan struct{})
	go func() { defer close(done); worker.runOnce() }()
	<-entered
	choices[0].AutoSync = false
	require.NoError(t, svc.UpdateClientVersions(context.Background(), choices))
	close(finish)
	<-done
	require.Empty(t, repo.values[clientVersionSpecs()[0].syncedKey])
}

func TestClientVersionMetadataValidation(t *testing.T) {
	for _, tc := range []struct {
		id, body string
		want     string
		fail     bool
	}{
		{"grok_cli", "1.0.46\n", "1.0.46", false},
		{"claude_cli", `{"tag_name":"v2.1.287"}`, "2.1.287", false},
		{"claude_cli", `{"tag_name":"v2.1.287","prerelease":true}`, "", true},
		{"claude_sdk", `{"name":"@anthropic-ai/sdk","version":"0.131.0"}`, "0.131.0", false},
		{"claude_sdk", `{"name":"@anthropic-ai/vertex-sdk","version":"0.1.0"}`, "", true},
		{"claude_sdk", strings.Repeat("x", clientVersionMetadataLimit+1), "", true},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Empty(t, r.Header.Get("Authorization"))
			_, _ = io.WriteString(w, tc.body)
		}))
		spec := clientVersionSpec{id: tc.id, source: server.URL}
		got, err := fetchClientVersion(context.Background(), server.Client(), spec)
		require.Equal(t, tc.want, got)
		require.Equal(t, tc.fail, err != nil)
		server.Close()
	}
	worker := NewClientVersionSyncService(nil, time.Hour)
	defer worker.Stop()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/private", http.StatusFound)
	}))
	defer server.Close()
	_, err := worker.fetch(context.Background(), clientVersionSpec{id: "grok_cli", source: server.URL})
	require.ErrorContains(t, err, "302")
}

func TestConfiguredSDKHeadersPreserveClientIdentity(t *testing.T) {
	claude.SetSDKVersionResolver(func() string { return "9.0.0" })
	t.Cleanup(func() { claude.SetSDKVersionResolver(nil) })
	require.Equal(t, "9.0.0", claude.DefaultHeaders()["X-Stainless-Package-Version"])
	svc := &IdentityService{}
	for _, provided := range []string{"", "0.100.0"} {
		h := http.Header{}
		if provided != "" {
			h.Set("X-Stainless-Package-Version", provided)
		}
		fp := svc.createFingerprintFromHeaders(h)
		// A persisted generated default follows a later configuration change.
		encoded, err := json.Marshal(fp)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(encoded, fp))
		claude.SetSDKVersionResolver(func() string { return "9.1.0" })
		req, _ := http.NewRequest(http.MethodPost, "https://example.test", nil)
		svc.ApplyFingerprint(req, fp)
		want := provided
		if want == "" {
			want = "9.1.0"
		}
		require.Equal(t, want, req.Header.Get("X-Stainless-Package-Version"))
	}
	legacy := &Fingerprint{StainlessPackageVersion: "0.99.0"}
	req, _ := http.NewRequest(http.MethodPost, "https://example.test", nil)
	svc.ApplyFingerprint(req, legacy)
	require.Equal(t, "0.99.0", req.Header.Get("X-Stainless-Package-Version"))
}

func TestClientVersionWorkerStopCancelsFetch(t *testing.T) {
	svc, repo := versionTestService()
	repo.values[clientVersionSpecs()[0].autoKey] = "true"
	worker := NewClientVersionSyncService(svc, time.Hour)
	entered := make(chan struct{})
	worker.fetch = func(ctx context.Context, _ clientVersionSpec) (string, error) {
		close(entered)
		<-ctx.Done()
		return "", ctx.Err()
	}
	worker.Start()
	worker.Start()
	<-entered
	worker.Stop()
	worker.Stop()
	require.Zero(t, repo.writes)
}

func TestClientVersionOldPinFallsBackWithoutWritesOrSync(t *testing.T) {
	svc, repo := versionTestService()
	spec := clientVersionSpecs()[1]
	repo.values[spec.versionKey] = "2.1.287"
	repo.values[spec.autoKey] = "false"
	view := resolveClientVersion(spec, repo.values)
	require.False(t, view.AutoSync)
	require.Equal(t, spec.builtin, view.EffectiveVersion)
	require.Equal(t, spec.builtin, svc.clientVersion(t.Context(), spec.id))
	require.Equal(t, "2.1.287", repo.values[spec.versionKey])
	require.Zero(t, repo.writes)
}
