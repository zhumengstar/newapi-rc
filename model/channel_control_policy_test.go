package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelControlPolicyNormalize(t *testing.T) {
	policy := ChannelControlPolicy{Group: " GPTPro ", ProbeMode: "IMAGE"}
	require.NoError(t, policy.Normalize())
	assert.Equal(t, "GPTPro", policy.Group)
	assert.Equal(t, ChannelProbeModeImage, policy.ProbeMode)
	assert.Equal(t, defaultChannelRecoverySuccesses, policy.RecoverySuccessesRequired)

	invalid := ChannelControlPolicy{Group: "a,b"}
	assert.Error(t, invalid.Normalize())
}

func TestRecordChannelRecoveryProbeRequiresConsecutiveSuccesses(t *testing.T) {
	truncateTables(t)
	ctx := context.Background()

	first, err := RecordChannelRecoveryProbe(ctx, 991, true)
	require.NoError(t, err)
	assert.Equal(t, 1, first.SuccessStreak)

	second, err := RecordChannelRecoveryProbe(ctx, 991, true)
	require.NoError(t, err)
	assert.Equal(t, 2, second.SuccessStreak)

	failed, err := RecordChannelRecoveryProbe(ctx, 991, false)
	require.NoError(t, err)
	assert.Zero(t, failed.SuccessStreak)
	assert.NotZero(t, failed.LastFailureAt)

	third, err := RecordChannelRecoveryProbe(ctx, 991, true)
	require.NoError(t, err)
	assert.Equal(t, 1, third.SuccessStreak)

	require.NoError(t, ClearChannelRecoveryState(ctx, 991))
	var count int64
	require.NoError(t, DB.Model(&ChannelRecoveryState{}).Where("channel_id = ?", 991).Count(&count).Error)
	assert.Zero(t, count)
}
