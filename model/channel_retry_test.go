package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestGetRandomSatisfiedChannelExcludingUsesBestRemainingPriority(t *testing.T) {
	previousMemoryCache := common.MemoryCacheEnabled
	channelSyncLock.Lock()
	previousGroups := group2model2channels
	previousChannels := channelsIDM
	priorityHigh := int64(100)
	priorityLow := int64(50)
	weightHigh := uint(100)
	weightLower := uint(50)
	group2model2channels = map[string]map[string][]int{
		"default": {"gpt-test": {1, 2, 3}},
	}
	channelsIDM = map[int]*Channel{
		1: {Id: 1, Priority: &priorityHigh, Weight: &weightHigh},
		2: {Id: 2, Priority: &priorityHigh, Weight: &weightLower},
		3: {Id: 3, Priority: &priorityLow, Weight: &weightHigh},
	}
	channelSyncLock.Unlock()
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCache
		channelSyncLock.Lock()
		group2model2channels = previousGroups
		channelsIDM = previousChannels
		channelSyncLock.Unlock()
	})

	peer, err := GetRandomSatisfiedChannelExcluding("default", "gpt-test", 0, nil, map[int]bool{1: true})
	require.NoError(t, err)
	require.NotNil(t, peer)
	require.Equal(t, 2, peer.Id, "same-priority peer must be tried before a lower priority")

	fallback, err := GetRandomSatisfiedChannelExcluding("default", "gpt-test", 0, nil, map[int]bool{1: true, 2: true})
	require.NoError(t, err)
	require.NotNil(t, fallback)
	require.Equal(t, 3, fallback.Id, "next priority must be used after the top priority is exhausted")
}

func TestGetRandomSatisfiedChannelExcludingUsesHighestRemainingWeight(t *testing.T) {
	previousMemoryCache := common.MemoryCacheEnabled
	channelSyncLock.Lock()
	previousGroups := group2model2channels
	previousChannels := channelsIDM
	priority := int64(100)
	weightLow := uint(10)
	weightHigh := uint(80)
	weightMiddle := uint(40)
	group2model2channels = map[string]map[string][]int{
		"default": {"gpt-test": {1, 2, 3}},
	}
	channelsIDM = map[int]*Channel{
		1: {Id: 1, Priority: &priority, Weight: &weightLow},
		2: {Id: 2, Priority: &priority, Weight: &weightHigh},
		3: {Id: 3, Priority: &priority, Weight: &weightMiddle},
	}
	channelSyncLock.Unlock()
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCache
		channelSyncLock.Lock()
		group2model2channels = previousGroups
		channelsIDM = previousChannels
		channelSyncLock.Unlock()
	})

	peer, err := GetRandomSatisfiedChannelExcluding("default", "gpt-test", 0, nil, map[int]bool{1: true})
	require.NoError(t, err)
	require.NotNil(t, peer)
	require.Equal(t, 2, peer.Id, "retry must use the highest remaining enabled weight")

	nextPeer, err := GetRandomSatisfiedChannelExcluding("default", "gpt-test", 0, nil, map[int]bool{1: true, 2: true})
	require.NoError(t, err)
	require.NotNil(t, nextPeer)
	require.Equal(t, 3, nextPeer.Id, "retry must continue in descending weight order")
}

func TestFilterChannelsByExcludedChannelIdsDoesNotMutateInput(t *testing.T) {
	original := []int{1, 2, 3}
	filtered := filterChannelsByExcludedChannelIds(original, map[int]bool{2: true})
	require.Equal(t, []int{1, 3}, filtered)
	require.Equal(t, []int{1, 2, 3}, original)
}

func TestGetRandomSatisfiedChannelUsesCostTierBeforeLegacyPriority(t *testing.T) {
	previousMemoryCache := common.MemoryCacheEnabled
	channelSyncLock.Lock()
	previousGroups := group2model2channels
	previousChannels := channelsIDM
	legacyPriority := int64(999)
	weight := uint(1)
	group2model2channels = map[string]map[string][]int{
		"default": {"gpt-test": {1, 2}},
	}
	channelsIDM = map[int]*Channel{
		// Cost tier is the actual routing layer even if legacy priority differs.
		1: {Id: 1, Priority: &legacyPriority, CostTier: 10, Weight: &weight},
		2: {Id: 2, Priority: &legacyPriority, CostTier: 20, Weight: &weight},
	}
	channelSyncLock.Unlock()
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCache
		channelSyncLock.Lock()
		group2model2channels = previousGroups
		channelsIDM = previousChannels
		channelSyncLock.Unlock()
	})

	selected, err := GetRandomSatisfiedChannelExcluding("default", "gpt-test", 0, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, 2, selected.Id)
}
