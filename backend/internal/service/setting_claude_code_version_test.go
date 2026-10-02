//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/stretchr/testify/require"
)

func TestGetClaudeCodeClientVersionPriority(t *testing.T) {
	for _, tt := range []struct {
		name    string
		manual  string
		synced  string
		enabled bool
		want    string
	}{
		{name: "手动值优先", manual: "2.1.287", synced: "2.1.288", want: "2.1.287"},
		{name: "归一化手动值", manual: " v2.1.287 ", synced: "2.1.288", want: "2.1.287"},
		{name: "跟随同步值", enabled: true, synced: "2.1.288", want: "2.1.288"},
		{name: "无效手动值回退", enabled: true, manual: "invalid", synced: "2.1.288", want: "2.1.288"},
		{name: "无效同步值回退", synced: "2.1.9", want: claude.CLIVersion()},
		{name: "未配置时回退", want: claude.CLIVersion()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &authSourceDefaultsRepoStub{values: map[string]string{
				SettingKeyClaudeCodeClientVersion:          tt.manual,
				SettingKeyClaudeCodeVersionAutoSyncEnabled: fmt.Sprint(tt.enabled),
				SettingKeyClaudeCodeClientVersionSynced:    tt.synced,
			}}
			svc := NewSettingService(repo, &config.Config{})
			require.Equal(t, tt.want, svc.GetClaudeCodeClientVersion(context.Background()))
		})
	}
}

func TestUpdateSettingsClaudeCodeVersionTakesEffectImmediately(t *testing.T) {
	ctx := context.Background()
	repo := &authSourceDefaultsRepoStub{values: map[string]string{
		SettingKeyClaudeCodeClientVersionSynced:    "2.1.288",
		SettingKeyClaudeCodeVersionAutoSyncEnabled: "true",
	}}
	svc := NewSettingService(repo, &config.Config{})
	resetGatewayForwardingSettingsCacheForTest(t)
	defer svc.refreshCachedSettings(&SystemSettings{})
	require.Equal(t, "2.1.288", svc.GetClaudeCodeClientVersion(ctx))

	settings := &SystemSettings{
		ClaudeCodeClientVersion:          "2.1.287",
		ClaudeCodeVersionAutoSyncEnabled: false,
	}
	require.NoError(t, svc.UpdateSettings(ctx, settings))
	require.Equal(t, "2.1.287", svc.GetClaudeCodeClientVersion(ctx), "保存手动版本后应立即生效")
	require.NotContains(t, repo.updates, SettingKeyClaudeCodeClientVersionSynced)
	require.Equal(t, "2.1.288", repo.values[SettingKeyClaudeCodeClientVersionSynced])

	// 开启自动同步后使用同步值，管理员自定义版本仍保存。
	settings.ClaudeCodeClientVersion = "2.1.287"
	settings.ClaudeCodeVersionAutoSyncEnabled = true
	require.NoError(t, svc.UpdateSettings(ctx, settings))
	require.Equal(t, "2.1.288", svc.GetClaudeCodeClientVersion(ctx))
	require.Equal(t, "true", repo.values[SettingKeyClaudeCodeVersionAutoSyncEnabled])
}
