package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

const clientVersionSyncInterval = time.Hour
const clientVersionMetadataLimit = 64 << 10

type clientVersionFetcher func(context.Context, clientVersionSpec) (string, error)

type ClientVersionSyncService struct {
	settings  *SettingService
	fetch     clientVersionFetcher
	interval  time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	wg        sync.WaitGroup
}

func NewClientVersionSyncService(settings *SettingService, interval time.Duration) *ClientVersionSyncService {
	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &ClientVersionSyncService{settings: settings, interval: interval, ctx: ctx, cancel: cancel,
		fetch: func(ctx context.Context, spec clientVersionSpec) (string, error) {
			return fetchClientVersion(ctx, client, spec)
		},
	}
}

func (s *ClientVersionSyncService) Start() {
	if s == nil || s.settings == nil || s.settings.settingRepo == nil || s.interval <= 0 {
		return
	}
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			s.runOnce()
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
					s.runOnce()
				}
			}
		}()
	})
}

func (s *ClientVersionSyncService) Stop() {
	if s != nil {
		s.cancel()
		s.wg.Wait()
	}
}

func (s *ClientVersionSyncService) runOnce() {
	for _, spec := range clientVersionSpecs() {
		if s.ctx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
		s.syncClient(ctx, spec)
		cancel()
	}
}

func (s *ClientVersionSyncService) syncClient(ctx context.Context, spec clientVersionSpec) {
	values, err := s.settings.settingRepo.GetMultiple(ctx, []string{spec.autoKey, spec.statusKey()})
	// Missing, malformed or unreadable opt-ins never authorize outbound requests.
	if err != nil || values[spec.autoKey] != "true" {
		return
	}
	var last ClientVersionStatus
	_ = json.Unmarshal([]byte(values[spec.statusKey()]), &last)
	if !last.CheckedAt.IsZero() && time.Since(last.CheckedAt) < s.interval {
		return
	}
	latest, fetchErr := s.fetch(ctx, spec)
	if ctx.Err() != nil {
		return
	}
	status := ClientVersionStatus{CheckedAt: time.Now().UTC()}
	if fetchErr != nil {
		status.Error = "fetch_failed"
	}
	latest = spec.normalize(latest)
	if fetchErr == nil && (latest == "" || semver.Prerelease("v"+latest) != "") {
		status.Error = "invalid_release"
	}

	s.settings.clientVersionMu.Lock()
	defer s.settings.clientVersionMu.Unlock()
	// Re-read after fetching: disabling sync while a request is in flight must
	// discard that response. Sync writes never modify the administrator's pin.
	current, err := s.settings.settingRepo.GetMultiple(ctx, []string{spec.autoKey, spec.syncedKey})
	if err != nil || current[spec.autoKey] != "true" {
		return
	}
	updates := make(map[string]string, 2)
	if status.Error == "" {
		old := spec.normalize(current[spec.syncedKey])
		if newerClientVersion(latest, old) {
			updates[spec.syncedKey] = latest
		}
		if old != "" && semver.Compare("v"+latest, "v"+old) < 0 {
			status.Error = "older_release"
		}
	}
	encoded, _ := json.Marshal(status)
	updates[spec.statusKey()] = string(encoded)
	if err := s.settings.settingRepo.SetMultiple(ctx, updates); err != nil {
		slog.Warn("client_version_sync_persist_failed", "client", spec.id)
		return
	}
	s.settings.InvalidateClaudeCodeClientVersionCache()
}

func fetchClientVersion(ctx context.Context, client *http.Client, spec clientVersionSpec) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.source, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "linksyncx-version-sync")
	res, err := client.Do(req)
	if err != nil {
		return "", errors.New("version metadata unavailable")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, clientVersionMetadataLimit+1))
	if err != nil || len(body) > clientVersionMetadataLimit {
		return "", errors.New("invalid metadata size")
	}
	switch spec.id {
	case "grok_cli":
		return strings.TrimSpace(string(body)), nil
	case "claude_cli":
		var release struct {
			TagName    string `json:"tag_name"`
			Draft      bool   `json:"draft"`
			Prerelease bool   `json:"prerelease"`
		}
		if err := json.Unmarshal(body, &release); err != nil {
			return "", err
		}
		if release.Draft || release.Prerelease || !strings.HasPrefix(release.TagName, "v") {
			return "", errors.New("not a stable CLI release")
		}
		return strings.TrimPrefix(release.TagName, "v"), nil
	case "claude_sdk":
		var pkg struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(body, &pkg); err != nil {
			return "", err
		}
		if pkg.Name != "@anthropic-ai/sdk" {
			return "", errors.New("unexpected SDK package")
		}
		return pkg.Version, nil
	default:
		return "", errors.New("unknown client")
	}
}
