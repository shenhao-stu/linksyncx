package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"golang.org/x/mod/semver"
)

// The catalog is the only mapping between a client, its persisted settings and
// its official release source. Runtime selection and the admin view share it.
type clientVersionSpec struct {
	id, versionKey, autoKey, syncedKey, builtin, minimum, source string
	fallback                                                     func() string
	valid                                                        func(string) bool
}

func clientVersionSpecs() []clientVersionSpec {
	return []clientVersionSpec{
		{id: "grok_cli", versionKey: "grok_cli_client_version", autoKey: "grok_cli_version_auto_sync_enabled",
			syncedKey: "grok_cli_client_version_synced", builtin: xai.CLIClientVersion, minimum: xai.CLIStableVersion,
			source: "https://x.ai/cli/stable", fallback: xai.EnvironmentCLIVersion, valid: xai.IsSupportedCLIVersion},
		{id: "claude_cli", versionKey: SettingKeyClaudeCodeClientVersion, autoKey: SettingKeyClaudeCodeVersionAutoSyncEnabled,
			syncedKey: SettingKeyClaudeCodeClientVersionSynced, builtin: claude.CLICurrentVersion, minimum: claude.CLICurrentVersion,
			source: "https://api.github.com/repos/anthropics/claude-code/releases/latest", fallback: claude.CLIVersion, valid: claude.IsSupportedCLIVersion},
		{id: "claude_sdk", versionKey: "claude_sdk_client_version", autoKey: "claude_sdk_version_auto_sync_enabled",
			syncedKey: "claude_sdk_client_version_synced", builtin: claude.SDKTSVersion, minimum: claude.SDKTSVersion,
			source: "https://registry.npmjs.org/@anthropic-ai/sdk/latest", fallback: func() string { return claude.SDKTSVersion }, valid: claude.IsSupportedSDKVersion},
	}
}

func (s clientVersionSpec) normalize(raw string) string {
	v := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	if len(v) > 64 {
		return ""
	}
	core, _, _ := strings.Cut(v, "-")
	for _, component := range strings.Split(core, ".") {
		if _, err := strconv.ParseUint(component, 10, 32); err != nil {
			return ""
		}
	}
	if !s.valid(v) {
		return ""
	}
	return v
}

func (s clientVersionSpec) statusKey() string { return s.syncedKey + "_status" }

func clientVersionKeys() []string {
	keys := make([]string, 0, 12)
	for _, spec := range clientVersionSpecs() {
		keys = append(keys, spec.versionKey, spec.autoKey, spec.syncedKey, spec.statusKey())
	}
	return keys
}

type ClientVersionChoice struct {
	ID            string `json:"id"`
	CustomVersion string `json:"custom_version"`
	AutoSync      bool   `json:"auto_sync"`
}

type ClientVersionStatus struct {
	CheckedAt time.Time `json:"checked_at,omitzero"`
	Error     string    `json:"error,omitempty"`
}

type ClientVersionView struct {
	ClientVersionChoice
	BuiltinVersion   string `json:"builtin_version"`
	MinimumVersion   string `json:"minimum_version"`
	EffectiveVersion string `json:"effective_version"`
	EffectiveSource  string `json:"effective_source"`
	SyncedVersion    string `json:"synced_version"`
	OfficialSource   string `json:"official_source"`
	ClientVersionStatus
}

func resolveClientVersion(spec clientVersionSpec, values map[string]string) ClientVersionView {
	custom := spec.normalize(values[spec.versionKey])
	source := "custom"
	if custom == "" {
		custom, source = spec.fallback(), "builtin"
		if custom != spec.builtin {
			source = "environment"
		}
	}
	synced := spec.normalize(values[spec.syncedKey])
	enabled := values[spec.autoKey] == "true"
	effective := custom
	if enabled && synced != "" {
		effective, source = synced, "official"
	}
	var status ClientVersionStatus
	_ = json.Unmarshal([]byte(values[spec.statusKey()]), &status)
	return ClientVersionView{
		ClientVersionChoice: ClientVersionChoice{spec.id, custom, enabled},
		BuiltinVersion:      spec.builtin, MinimumVersion: spec.minimum, EffectiveVersion: effective, EffectiveSource: source,
		SyncedVersion: synced, OfficialSource: spec.source, ClientVersionStatus: status,
	}
}

func (s *SettingService) GetClientVersions(ctx context.Context) ([]ClientVersionView, error) {
	values, err := s.settingRepo.GetMultiple(ctx, clientVersionKeys())
	if err != nil {
		return nil, err
	}
	views := make([]ClientVersionView, 0, 3)
	for _, spec := range clientVersionSpecs() {
		views = append(views, resolveClientVersion(spec, values))
	}
	return views, nil
}

var ErrInvalidClientVersions = errors.New("invalid client versions")

// All three choices are validated before a single atomic settings write. Only
// configuration keys are written; a stale admin page cannot erase sync results.
func (s *SettingService) UpdateClientVersions(ctx context.Context, choices []ClientVersionChoice) error {
	specs := clientVersionSpecs()
	if len(choices) != len(specs) {
		return fmt.Errorf("%w: all three clients are required", ErrInvalidClientVersions)
	}
	updates := make(map[string]string, 6)
	seen := make(map[string]bool, 3)
	for _, choice := range choices {
		found := false
		for _, spec := range specs {
			if choice.ID != spec.id {
				continue
			}
			if seen[spec.id] {
				return fmt.Errorf("%w: duplicate %s", ErrInvalidClientVersions, spec.id)
			}
			version := spec.normalize(choice.CustomVersion)
			if version == "" {
				return fmt.Errorf("%w: unsupported version for %s", ErrInvalidClientVersions, spec.id)
			}
			seen[spec.id], found = true, true
			updates[spec.versionKey] = version
			updates[spec.autoKey] = "false"
			if choice.AutoSync {
				updates[spec.autoKey] = "true"
			}
		}
		if !found {
			return fmt.Errorf("%w: unknown client", ErrInvalidClientVersions)
		}
	}
	s.clientVersionMu.Lock()
	defer s.clientVersionMu.Unlock()
	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return err
	}
	s.InvalidateClaudeCodeClientVersionCache()
	return nil
}

type cachedClientVersions struct {
	versions  map[string]string
	expiresAt time.Time
}

// A failed refresh keeps the last good snapshot. Refreshes and configuration
// writes share the mutex, preventing an older DB read from undoing a saved edit.
func (s *SettingService) clientVersion(ctx context.Context, id string) string {
	fallback := ""
	for _, spec := range clientVersionSpecs() {
		if spec.id == id {
			fallback = spec.fallback()
		}
	}
	if s == nil || s.settingRepo == nil {
		return fallback
	}
	if cached := s.clientVersionCache.Load(); cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.versions[id]
	}
	s.clientVersionMu.Lock()
	defer s.clientVersionMu.Unlock()
	cached := s.clientVersionCache.Load()
	if cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.versions[id]
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	values, err := s.settingRepo.GetMultiple(dbCtx, clientVersionKeys())
	versions := make(map[string]string, 3)
	for _, spec := range clientVersionSpecs() {
		versions[spec.id] = resolveClientVersion(spec, values).EffectiveVersion
	}
	ttl := time.Minute
	if err != nil {
		ttl = 5 * time.Second
		if cached != nil {
			versions = cached.versions
		}
	}
	s.clientVersionCache.Store(&cachedClientVersions{versions, time.Now().Add(ttl)})
	return versions[id]
}

func newerClientVersion(candidate, current string) bool {
	return current == "" || semver.Compare("v"+candidate, "v"+current) > 0
}
