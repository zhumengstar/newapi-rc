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

func TestBuildEffectiveRetryExcludedChannelIdsNeverRepeatsTriedChannelAndFollowsPriorityAndWeight(t *testing.T) {
	previousMemoryCache := common.MemoryCacheEnabled
	channelSyncLock.Lock()
	previousGroups := group2model2channels
	previousChannels := channelsIDM
	prio199 := int64(199)
	prio180 := int64(180)
	w100 := uint(100)
	w60 := uint(60)
	w20 := uint(20)
	w90 := uint(90)
	w30 := uint(30)
	group2model2channels = map[string]map[string][]int{
		"Gemini组": {
			"gemini-4-argon": {438},
			"gemini-3-flash": {10, 11, 12, 20, 21},
		},
	}
	channelsIDM = map[int]*Channel{
		438: {Id: 438, Status: common.ChannelStatusEnabled, Priority: &prio199, Weight: &w100},
		10:  {Id: 10, Status: common.ChannelStatusEnabled, Priority: &prio199, Weight: &w60},
		11:  {Id: 11, Status: common.ChannelStatusEnabled, Priority: &prio199, Weight: &w100},
		12:  {Id: 12, Status: common.ChannelStatusEnabled, Priority: &prio199, Weight: &w20},
		20:  {Id: 20, Status: common.ChannelStatusEnabled, Priority: &prio180, Weight: &w30},
		21:  {Id: 21, Status: common.ChannelStatusEnabled, Priority: &prio180, Weight: &w90},
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

	// 1. Single-channel scenario (#438): once #438 is tried once, retry must NOT select #438 again.
	excludedSingle := BuildEffectiveRetryExcludedChannelIds("Gemini组", "gemini-4-argon", 0, nil, nil, map[int]int{438: 1}, 438, true, true)
	require.True(t, excludedSingle[438], "tried channel #438 must be excluded on retry")
	chSingle, err := GetRandomSatisfiedChannelForUserExcluding("Gemini组", "gemini-4-argon", 0, 0, nil, excludedSingle)
	require.NoError(t, err)
	require.Nil(t, chSingle, "single-channel model must return nil on retry instead of repeating #438")

	// 2. Multi-channel scenario: suppose initial request picked #10 (prio 199, weight 60) and failed.
	// Remaining untried channels must be selected strictly by (priority DESC, weight DESC):
	// 11 (199, w100) -> 12 (199, w20) -> 21 (180, w90) -> 20 (180, w30) -> nil
	triedCounts := map[int]int{10: 1}
	lastTried := 10
	expectedOrder := []int{11, 12, 21, 20}
	for _, expectedId := range expectedOrder {
		excluded := BuildEffectiveRetryExcludedChannelIds("Gemini组", "gemini-3-flash", 0, nil, nil, triedCounts, lastTried, true, true)
		ch, err := GetRandomSatisfiedChannelForUserExcluding("Gemini组", "gemini-3-flash", 0, 0, nil, excluded)
		require.NoError(t, err)
		require.NotNil(t, ch)
		require.Equal(t, expectedId, ch.Id)
		triedCounts[ch.Id]++
		lastTried = ch.Id
	}

	// After all 5 channels have been tried once, the next retry must return nil.
	excludedAll := BuildEffectiveRetryExcludedChannelIds("Gemini组", "gemini-3-flash", 0, nil, nil, triedCounts, lastTried, true, true)
	chAfterAll, err := GetRandomSatisfiedChannelForUserExcluding("Gemini组", "gemini-3-flash", 0, 0, nil, excludedAll)
	require.NoError(t, err)
	require.Nil(t, chAfterAll, "must stop retrying once all candidate channels have been tried once")
}

