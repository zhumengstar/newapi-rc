package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestChannelBalanceHandlerScheduleConfiguration(t *testing.T) {
	handler := channelBalanceHandler{}

	t.Run("can be disabled explicitly", func(t *testing.T) {
		t.Setenv("CHANNEL_BALANCE_REFRESH_ENABLED", "false")
		assert.False(t, handler.Enabled())
	})

	t.Run("uses configured interval", func(t *testing.T) {
		t.Setenv("CHANNEL_BALANCE_REFRESH_INTERVAL_MINUTES", "30")
		t.Setenv("CHANNEL_UPDATE_FREQUENCY", "5")
		assert.Equal(t, 30*time.Minute, handler.Interval())
	})

	t.Run("supports legacy interval", func(t *testing.T) {
		t.Setenv("CHANNEL_BALANCE_REFRESH_INTERVAL_MINUTES", "")
		t.Setenv("CHANNEL_UPDATE_FREQUENCY", "15")
		assert.Equal(t, 15*time.Minute, handler.Interval())
	})
}

func TestChannelRecoveryHandlerRequiresExplicitEnablement(t *testing.T) {
	handler := channelRecoveryHandler{}
	t.Setenv("CHANNEL_RECOVERY_TASK_ENABLED", "true")
	// AutomaticEnableChannelEnabled is a process-wide safety switch. The task
	// must remain dormant when it is not enabled, even if the env flag is set.
	old := common.AutomaticEnableChannelEnabled
	common.AutomaticEnableChannelEnabled = false
	t.Cleanup(func() { common.AutomaticEnableChannelEnabled = old })
	assert.False(t, handler.Enabled())
	assert.Equal(t, 1800*time.Second, handler.Interval())
}

func TestPriorityNormalizeHandlerUsesConfiguredInterval(t *testing.T) {
	handler := priorityNormalizeHandler{}
	t.Setenv("CHANNEL_PRIORITY_NORMALIZER_ENABLED", "true")
	t.Setenv("CHANNEL_PRIORITY_NORMALIZER_INTERVAL_SECONDS", "3600")
	assert.True(t, handler.Enabled())
	assert.Equal(t, time.Hour, handler.Interval())
}

func TestChannelHealthHandlerRequiresExplicitEnablement(t *testing.T) {
	handler := channelHealthHandler{}
	t.Setenv("CHANNEL_HEALTH_CHECK_ENABLED", "true")
	assert.True(t, handler.Enabled())
	t.Setenv("CHANNEL_HEALTH_CHECK_INTERVAL_SECONDS", "120")
	assert.Equal(t, 120*time.Second, handler.Interval())
}
