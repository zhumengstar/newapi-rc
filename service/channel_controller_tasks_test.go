package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredAdaptiveGroupsDeduplicatesAndTrims(t *testing.T) {
	t.Setenv("CHANNEL_ADAPTIVE_GROUPS", " GPT对接组, ,GPTPro-对接池,GPT对接组 ")
	assert.Equal(t, []string{"GPT对接组", "GPTPro-对接池"}, configuredAdaptiveGroups())
}

func TestConfiguredAdaptiveGroupsFallsBackToLegacyVariable(t *testing.T) {
	t.Setenv("CHANNEL_ADAPTIVE_GROUPS", "")
	t.Setenv("CHANNEL_WEIGHT_CONTROLLER_GROUPS", "图片组")
	assert.Equal(t, []string{"图片组"}, configuredAdaptiveGroups())
}

func TestNormalizedChannelGroupsSupportsSharedChannels(t *testing.T) {
	assert.True(t, adaptiveGroupConfigured("GPT对接组, GPTPro-对接池", []string{"GPTPro-对接池"}))
}

func TestRunChannelRecoveryRequiresRunner(t *testing.T) {
	summary, err := RunChannelRecoveryOnce(context.Background(), nil)
	assert.Error(t, err)
	assert.Equal(t, 0, summary.Attempted)
	assert.Equal(t, 0, summary.Recovered)
}

func TestRunChannelRecoveryKeepsSuccessfulProbesPending(t *testing.T) {
	summary, err := RunChannelRecoveryOnce(context.Background(), func(context.Context) (int, int, int, error) {
		return 3, 1, 1, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, summary.Attempted)
	assert.Equal(t, 1, summary.Recovered)
	assert.Equal(t, 1, summary.Pending)
	assert.Equal(t, 1, summary.Failed)
}
