package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestAllowChannelRPMUsesPerChannelWindow(t *testing.T) {
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	defer func() { common.RedisEnabled = originalRedisEnabled }()

	channelRPMMemory.Lock()
	channelRPMMemory.counts = make(map[string]channelRPMEntry)
	channelRPMMemory.Unlock()

	require.True(t, AllowChannelRPM(nil, 901, 2))
	require.True(t, AllowChannelRPM(nil, 901, 2))
	require.False(t, AllowChannelRPM(nil, 901, 2))
	require.True(t, AllowChannelRPM(nil, 902, 2), "limits are isolated by channel")
	require.True(t, AllowChannelRPM(nil, 901, 0), "zero disables the limit")
}
