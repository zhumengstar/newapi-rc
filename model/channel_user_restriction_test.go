package model

import (
	"testing"

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
