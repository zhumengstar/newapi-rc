package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChannelsForAddExpandsGroupsAndKeys(t *testing.T) {
	weight := uint(12)
	source := &model.Channel{
		Name:   "shared-channel",
		Key:    "original-key",
		Models: "gpt-test",
		Group:  " group-a,group-b,group-a ",
		Weight: &weight,
	}

	channels := buildChannelsForAdd(source, []string{"key-one", "key-two"}, true)

	if assert.Len(t, channels, 4) {
		assert.Equal(t, []string{"key-one", "key-one", "key-two", "key-two"}, []string{
			channels[0].Key,
			channels[1].Key,
			channels[2].Key,
			channels[3].Key,
		})
		assert.Equal(t, []string{"group-a", "group-b", "group-a", "group-b"}, []string{
			channels[0].Group,
			channels[1].Group,
			channels[2].Group,
			channels[3].Group,
		})
		assert.Equal(t, "shared-channel key-one", channels[0].Name)
		assert.Equal(t, "shared-channel key-two", channels[2].Name)
		assert.Equal(t, uint(12), *channels[0].Weight)
	}
	assert.Equal(t, " group-a,group-b,group-a ", source.Group)
}

func TestBuildChannelsForAddKeepsSingleGroupAndSkipsEmptyKeys(t *testing.T) {
	source := &model.Channel{Name: "channel", Group: "default"}

	channels := buildChannelsForAdd(source, []string{"", "key"}, false)

	if assert.Len(t, channels, 1) {
		assert.Equal(t, "default", channels[0].Group)
		assert.Equal(t, "key", channels[0].Key)
		assert.Equal(t, "channel", channels[0].Name)
	}
}

func TestAddChannelCreatesIndependentChannelsForGroups(t *testing.T) {
	setupTaskPluginBindChannelTest(t)

	body := fmt.Sprintf(
		`{"mode":"single","channel":{"type":%d,"name":"multi-group-channel","key":"channel-key","models":"gpt-test","group":"group-a,group-b","status":%d}}`,
		constant.ChannelTypeOpenAI,
		common.ChannelStatusEnabled,
	)
	recorder := postAddChannel(t, 1, common.RoleRootUser, body)
	require.Contains(t, recorder.Body.String(), `"success":true`)

	var channels []model.Channel
	require.NoError(t, model.DB.Where("name = ?", "multi-group-channel").Order("id ASC").Find(&channels).Error)
	if assert.Len(t, channels, 2) {
		assert.Equal(t, []string{"group-a", "group-b"}, []string{channels[0].Group, channels[1].Group})
		assert.NotEqual(t, channels[0].Id, channels[1].Id)
		assert.Equal(t, channels[0].Key, channels[1].Key)
	}

	var abilities []model.Ability
	require.NoError(t, model.DB.Where("model = ?", "gpt-test").Order("channel_id ASC").Find(&abilities).Error)
	if assert.Len(t, abilities, 2) {
		assert.Equal(t, []string{"group-a", "group-b"}, []string{abilities[0].Group, abilities[1].Group})
		assert.Equal(t, abilities[0].ChannelId, channels[0].Id)
		assert.Equal(t, abilities[1].ChannelId, channels[1].Id)
	}
}
