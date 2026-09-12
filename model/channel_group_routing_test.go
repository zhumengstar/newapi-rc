package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateChannelGroupAdaptiveEnabledMatchesExactGroup(t *testing.T) {
	setupChannelStatusTest(t)

	channels := []Channel{
		{Name: "alpha", Key: "key-1", Group: "alpha", Status: common.ChannelStatusEnabled},
		{Name: "alpha-beta", Key: "key-2", Group: "alpha,beta", Status: common.ChannelStatusEnabled},
		{Name: "alphabet", Key: "key-3", Group: "alphabet", Status: common.ChannelStatusEnabled},
	}
	for index := range channels {
		require.NoError(t, DB.Create(&channels[index]).Error)
	}

	updated, err := UpdateChannelGroupAdaptiveEnabled("alpha", true)
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated)

	var stored []Channel
	require.NoError(t, DB.Order("id ASC").Find(&stored).Error)
	require.Len(t, stored, 3)
	assert.True(t, stored[0].AdaptiveEnabled)
	assert.True(t, stored[1].AdaptiveEnabled)
	assert.False(t, stored[2].AdaptiveEnabled)
}

func TestChannelGroupsAreTrimmedAndDeduplicated(t *testing.T) {
	channel := Channel{Group: " GPT对接组, GPTPro-对接池, GPT对接组 ,,"}

	assert.Equal(t, []string{"GPT对接组", "GPTPro-对接池"}, channel.GetGroups())
}

func TestApplyChannelGroupPriorityOrderUpdatesChannelAndAbilityPriorities(t *testing.T) {
	setupChannelStatusTest(t)

	alphaWeight, mixedWeight, betaWeight := uint(1), uint(2), uint(1)
	channels := []Channel{
		{Name: "alpha", Key: "key-1", Group: "alpha", Weight: &alphaWeight, Status: common.ChannelStatusEnabled},
		{Name: "mixed", Key: "key-2", Group: "alpha,beta", Weight: &mixedWeight, Status: common.ChannelStatusEnabled},
		{Name: "beta", Key: "key-3", Group: "beta", Weight: &betaWeight, Status: common.ChannelStatusEnabled},
	}
	for index := range channels {
		require.NoError(t, DB.Create(&channels[index]).Error)
	}
	require.NoError(t, DB.Create(&[]Ability{
		{Group: "alpha", Model: "gpt", ChannelId: channels[0].Id, Enabled: true},
		{Group: "alpha", Model: "gpt", ChannelId: channels[1].Id, Enabled: true},
		{Group: "beta", Model: "gpt", ChannelId: channels[1].Id, Enabled: true},
		{Group: "beta", Model: "gpt", ChannelId: channels[2].Id, Enabled: true},
	}).Error)

	updated, err := ApplyChannelGroupPriorityOrder([]string{"alpha", "beta"})
	require.NoError(t, err)
	assert.Equal(t, int64(3), updated)

	var storedChannels []Channel
	require.NoError(t, DB.Order("id ASC").Find(&storedChannels).Error)
	require.Len(t, storedChannels, 3)
	assert.Equal(t, int64(9), storedChannels[0].GetPriority())
	assert.Equal(t, int64(9), storedChannels[1].GetPriority())
	assert.Equal(t, int64(19), storedChannels[2].GetPriority())
	assert.Equal(t, uint(100), *storedChannels[0].Weight)
	assert.Equal(t, uint(200), *storedChannels[1].Weight)
	assert.Equal(t, uint(100), *storedChannels[2].Weight)

	var abilities []Ability
	require.NoError(t, DB.Order("channel_id ASC, "+commonGroupCol+" ASC").Find(&abilities).Error)
	require.Len(t, abilities, 4)
	for _, ability := range abilities {
		require.NotNil(t, ability.Priority)
		if ability.Group == "alpha" {
			assert.Equal(t, int64(9), *ability.Priority)
			if ability.ChannelId == channels[0].Id {
				assert.Equal(t, uint(100), ability.Weight)
			} else {
				assert.Equal(t, uint(200), ability.Weight)
			}
		} else {
			assert.Equal(t, int64(19), *ability.Priority)
			if ability.ChannelId == channels[1].Id {
				assert.Equal(t, uint(200), ability.Weight)
			} else {
				assert.Equal(t, uint(100), ability.Weight)
			}
		}
	}
}

func TestChannelGroupOrderSortUsesPriorityThenWeight(t *testing.T) {
	setupChannelStatusTest(t)
	priorityLow, priorityHigh := int64(10), int64(20)
	weightLow, weightHigh := uint(1), uint(9)
	channels := []Channel{
		{Name: "alpha-low", Key: "key-1", Group: "alpha", Priority: &priorityLow, Weight: &weightLow, Status: common.ChannelStatusEnabled},
		{Name: "alpha-high", Key: "key-2", Group: "alpha", Priority: &priorityLow, Weight: &weightHigh, Status: common.ChannelStatusEnabled},
		{Name: "beta", Key: "key-3", Group: "beta", Priority: &priorityHigh, Weight: &weightLow, Status: common.ChannelStatusEnabled},
	}
	for index := range channels {
		require.NoError(t, DB.Create(&channels[index]).Error)
	}

	var ordered []Channel
	options := NewChannelSortOptions("", "", false, "alpha,beta")
	require.NoError(t, options.Apply(DB.Model(&Channel{})).Find(&ordered).Error)
	require.Len(t, ordered, 3)
	assert.Equal(t, "alpha-high", ordered[0].Name)
	assert.Equal(t, "alpha-low", ordered[1].Name)
	assert.Equal(t, "beta", ordered[2].Name)
}
