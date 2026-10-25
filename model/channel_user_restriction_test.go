package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func TestUserRestrictionCache(t *testing.T) {
	channelSyncLock.Lock()
	oldRestricted := group2userRestricted
	oldAllowed := group2userAllowedChannels

	group2userRestricted = map[string]map[int]bool{
		"cc反重力按量": {
			436: true,
			567: true,
		},
	}
	group2userAllowedChannels = map[string]map[int]map[int]bool{
		"cc反重力按量": {
			436: {101: true, 102: true},
			567: {102: true},
		},
	}
	channelSyncLock.Unlock()

	defer func() {
		channelSyncLock.Lock()
		group2userRestricted = oldRestricted
		group2userAllowedChannels = oldAllowed
		channelSyncLock.Unlock()
	}()

	// 1. Configured user 436 is restricted
	assert.True(t, IsUserRestrictedInGroup("cc反重力按量", 436))
	allowed436 := GetUserAllowedChannelsInGroup("cc反重力按量", 436)
	assert.True(t, allowed436[101])
	assert.True(t, allowed436[102])
	assert.False(t, allowed436[103])

	// 2. Configured user 567 is restricted to 102 only
	assert.True(t, IsUserRestrictedInGroup("cc反重力按量", 567))
	allowed567 := GetUserAllowedChannelsInGroup("cc反重力按量", 567)
	assert.False(t, allowed567[101])
	assert.True(t, allowed567[102])

	// 3. Unconfigured user 999 is NOT restricted (all channels allowed)
	assert.False(t, IsUserRestrictedInGroup("cc反重力按量", 999))
	allowed999 := GetUserAllowedChannelsInGroup("cc反重力按量", 999)
	assert.Nil(t, allowed999)

	// 4. Other group: user 436 is not restricted in "default"
	assert.False(t, IsUserRestrictedInGroup("default", 436))

	// 5. Test Channel other settings with AllowedUserIds
	ch := &Channel{}
	ch.SetOtherSettings(kitdto.ChannelOtherSettings{
		AllowedUserIds: []int{436, 567},
	})
	assert.Equal(t, []int{436, 567}, ch.GetOtherSettings().AllowedUserIds)
}

func TestUserRestrictionDisabledChannelRoutingAndNoEscape(t *testing.T) {
	common.MemoryCacheEnabled = true

	channelSyncLock.Lock()
	oldRestricted := group2userRestricted
	oldAllowed := group2userAllowedChannels
	oldUserModel := group2userModel2channels
	oldModelChannels := group2model2channels
	oldChannelsIDM := channelsIDM

	ch380Priority := int64(10)
	ch380Weight := uint(1)
	ch380 := &Channel{
		Id:       380,
		Name:     "专属渠道380(已禁用)",
		Group:    "Gemini组",
		Models:   "gemini-2.5-flash",
		Status:   common.ChannelStatusManuallyDisabled, // 禁用状态
		Priority: &ch380Priority,
		Weight:   &ch380Weight,
	}
	ch380.SetOtherSettings(kitdto.ChannelOtherSettings{
		AllowedUserIds: []int{371, 563},
	})

	ch365Priority := int64(5)
	ch365Weight := uint(1)
	ch365 := &Channel{
		Id:       365,
		Name:     "公共渠道365(启用)",
		Group:    "Gemini组",
		Models:   "gemini-2.5-flash,claude-3-5-sonnet",
		Status:   common.ChannelStatusEnabled,
		Priority: &ch365Priority,
		Weight:   &ch365Weight,
	}

	channelsIDM = map[int]*Channel{
		380: ch380,
		365: ch365,
	}
	group2model2channels = map[string]map[string][]int{
		"Gemini组": {
			"gemini-2.5-flash":  {365},
			"claude-3-5-sonnet": {365},
		},
	}
	group2userRestricted = map[string]map[int]bool{
		"Gemini组": {
			371: true,
			563: true,
		},
	}
	group2userAllowedChannels = map[string]map[int]map[int]bool{
		"Gemini组": {
			371: {380: true},
			563: {380: true},
		},
	}
	group2userModel2channels = map[string]map[int]map[string][]int{
		"Gemini组": {
			371: {
				"gemini-2.5-flash": {380},
			},
			563: {
				"gemini-2.5-flash": {380},
			},
		},
	}
	channelSyncLock.Unlock()

	defer func() {
		channelSyncLock.Lock()
		group2userRestricted = oldRestricted
		group2userAllowedChannels = oldAllowed
		group2userModel2channels = oldUserModel
		group2model2channels = oldModelChannels
		channelsIDM = oldChannelsIDM
		channelSyncLock.Unlock()
	}()

	// 1. 验证辅助函数 IsUserAllowedChannelInGroup
	assert.True(t, IsUserAllowedChannelInGroup("Gemini组", 371, 380))
	assert.False(t, IsUserAllowedChannelInGroup("Gemini组", 371, 365))
	assert.False(t, IsUserAllowedChannelInGroup("Gemini组", 999, 380))

	// 2. 专属用户 371 请求已禁用的专属渠道 380：指定后，无论是否禁用，都只能走指定的渠道
	selectedCh, err := GetRandomSatisfiedChannelForUserExcluding("Gemini组", "gemini-2.5-flash", 371, 0, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, selectedCh)
	assert.Equal(t, 380, selectedCh.Id, "专属用户即使渠道被禁用也必须路由到渠道 380")

	// 3. 专属用户 371 排除渠道 380（如重试失败）时，严禁逃逸到公共渠道 365，必须返回 nil
	selectedExcluded, err := GetRandomSatisfiedChannelForUserExcluding("Gemini组", "gemini-2.5-flash", 371, 0, nil, map[int]bool{380: true})
	assert.NoError(t, err)
	assert.Nil(t, selectedExcluded, "排除专属渠道后绝不能 fallback 逃逸到公共渠道 365")

	// 4. 专属用户 371 请求未在专属渠道配置的模型（例如 claude-3-5-sonnet，虽公共渠道 365 包含），必须返回 nil 绝不逃逸
	selectedWrongModel, err := GetRandomSatisfiedChannelForUserExcluding("Gemini组", "claude-3-5-sonnet", 371, 0, nil, nil)
	assert.NoError(t, err)
	assert.Nil(t, selectedWrongModel, "专属渠道不支持的模型严禁穿透到公共渠道 365")

	// 5. 普通未受限用户 999 请求 gemini-2.5-flash，必须走公共渠道 365，绝不能打到专属渠道 380
	selectedPublic, err := GetRandomSatisfiedChannelForUserExcluding("Gemini组", "gemini-2.5-flash", 999, 0, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, selectedPublic)
	assert.Equal(t, 365, selectedPublic.Id, "普通用户只能访问公共渠道 365")

	// 6. 验证 IsChannelEnabledForUserGroupModel
	// 专属用户访问其专属渠道（即使被禁用）返回 true
	assert.True(t, IsChannelEnabledForUserGroupModel("Gemini组", "gemini-2.5-flash", 380, 371))
	// 专属用户访问公共渠道返回 false
	assert.False(t, IsChannelEnabledForUserGroupModel("Gemini组", "gemini-2.5-flash", 365, 371))
	// 普通用户访问专属渠道返回 false
	assert.False(t, IsChannelEnabledForUserGroupModel("Gemini组", "gemini-2.5-flash", 380, 999))
	// 普通用户访问公共渠道返回 true
	assert.True(t, IsChannelEnabledForUserGroupModel("Gemini组", "gemini-2.5-flash", 365, 999))
}
