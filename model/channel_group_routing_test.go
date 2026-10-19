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
	assert.Equal(t, uint(1), *storedChannels[0].Weight)
	assert.Equal(t, uint(2), *storedChannels[1].Weight)
	assert.Equal(t, uint(1), *storedChannels[2].Weight)

	var abilities []Ability
	require.NoError(t, DB.Order("channel_id ASC, "+commonGroupCol+" ASC").Find(&abilities).Error)
	require.Len(t, abilities, 4)
	for _, ability := range abilities {
		require.NotNil(t, ability.Priority)
		if ability.Group == "alpha" {
			assert.Equal(t, int64(9), *ability.Priority)
		} else {
			assert.Equal(t, int64(19), *ability.Priority)
		}
	}
}

func TestChannelWeightEditDoesNotAffectOtherChannels(t *testing.T) {
	setupChannelStatusTest(t)

	w1, w2, w3 := uint(99), uint(53), uint(42)
	ch1 := Channel{Name: "ch-1", Key: "k1", Group: "gemini-group", Weight: &w1, Status: common.ChannelStatusEnabled, Models: "gpt-4o"}
	ch2 := Channel{Name: "ch-2", Key: "k2", Group: "gemini-group", Weight: &w2, Status: common.ChannelStatusEnabled, Models: "gpt-4o"}
	ch3 := Channel{Name: "ch-3", Key: "k3", Group: "gemini-group", Weight: &w3, Status: common.ChannelStatusEnabled, Models: "gpt-4o"}

	require.NoError(t, DB.Create(&ch1).Error)
	require.NoError(t, ch1.AddAbilities(nil))
	require.NoError(t, DB.Create(&ch2).Error)
	require.NoError(t, ch2.AddAbilities(nil))
	require.NoError(t, DB.Create(&ch3).Error)
	require.NoError(t, ch3.AddAbilities(nil))

	// Now simulate editing ch1 weight to 150
	newW1 := uint(150)
	ch1Edit := ch1
	ch1Edit.Weight = &newW1
	require.NoError(t, ch1Edit.Update())

	// Verify ch1 is updated to 150
	var dbCh1 Channel
	require.NoError(t, DB.First(&dbCh1, ch1.Id).Error)
	assert.Equal(t, uint(150), *dbCh1.Weight)

	// Verify ch2 and ch3 weights remain COMPLETELY UNCHANGED
	var dbCh2 Channel
	require.NoError(t, DB.First(&dbCh2, ch2.Id).Error)
	assert.Equal(t, uint(53), *dbCh2.Weight, "ch2 weight must NOT be auto-adjusted when ch1 is edited")

	var dbCh3Channel Channel
	require.NoError(t, DB.First(&dbCh3Channel, ch3.Id).Error)
	assert.Equal(t, uint(42), *dbCh3Channel.Weight, "ch3 weight must NOT be auto-adjusted when ch1 is edited")

	// Verify abilities for ch2 and ch3 also remain unchanged
	var ab2, ab3 Ability
	require.NoError(t, DB.Where("channel_id = ?", ch2.Id).First(&ab2).Error)
	assert.Equal(t, uint(53), ab2.Weight)

	require.NoError(t, DB.Where("channel_id = ?", ch3.Id).First(&ab3).Error)
	assert.Equal(t, uint(42), ab3.Weight)

	var ab1 Ability
	require.NoError(t, DB.Where("channel_id = ?", ch1.Id).First(&ab1).Error)
	assert.Equal(t, uint(150), ab1.Weight)
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

func TestGetGroupBasePriority(t *testing.T) {
	setupChannelStatusTest(t)

	priority41 := int64(41)
	priority85 := int64(85)
	channels := []Channel{
		{Name: "ch-gpt", Key: "k1", Group: "gpt-group", Priority: &priority41, Status: common.ChannelStatusEnabled},
		{Name: "ch-multi", Key: "k2", Group: "foo,bar", Priority: &priority85, Status: common.ChannelStatusEnabled},
	}
	for index := range channels {
		require.NoError(t, DB.Create(&channels[index]).Error)
	}

	// Case 1: Ability table match
	require.NoError(t, DB.Create(&Ability{
		Group:     "ability-group",
		Model:     "gpt",
		ChannelId: channels[0].Id,
		Priority:  &priority41,
		Enabled:   true,
	}).Error)
	prio, err := GetGroupBasePriority("ability-group")
	require.NoError(t, err)
	require.NotNil(t, prio)
	assert.Equal(t, int64(41), *prio)

	// Case 2: Channel table match (no ability entry)
	prio, err = GetGroupBasePriority("gpt-group")
	require.NoError(t, err)
	require.NotNil(t, prio)
	assert.Equal(t, int64(41), *prio)

	// Case 3: Comma-separated group matching in channel table
	prio, err = GetGroupBasePriority("bar")
	require.NoError(t, err)
	require.NotNil(t, prio)
	assert.Equal(t, int64(85), *prio)

	// Case 4: Completely new group (does not exist in either table) - must NOT fail with SQL syntax error
	prio, err = GetGroupBasePriority("aswb专用分组外接-蒸馏")
	require.NoError(t, err)
	assert.Nil(t, prio)

	// Case 5: Empty group
	prio, err = GetGroupBasePriority("")
	require.NoError(t, err)
	assert.Nil(t, prio)
}
