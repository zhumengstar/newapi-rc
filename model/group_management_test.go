package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateGroupGlobalRenameAndRatio(t *testing.T) {
	setupChannelStatusTest(t)
	require.NoError(t, DB.AutoMigrate(&Option{}, &Token{}, &User{}))

	// 1. Setup channel and abilities
	ch := Channel{
		Name:   "gemini-ch",
		Key:    "test-key",
		Group:  "old-group,other-group",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(&ch).Error)

	ab := Ability{
		Group:     "old-group",
		Model:     "gemini-test",
		ChannelId: ch.Id,
		Enabled:   true,
	}
	require.NoError(t, DB.Create(&ab).Error)

	// 2. Setup user with setting
	u := User{
		Username: "test-user-grp",
		Group:    "old-group,vip",
		Setting:  `{"user_group_ratios":{"old-group":1.5},"user_model_price_rules":[{"group":"old-group","models":["m1"],"price":0.1}]}`,
	}
	require.NoError(t, DB.Create(&u).Error)

	// 3. Setup token
	tok := Token{
		UserId:     u.Id,
		Name:       "tok-1",
		Key:        "tok-key-1",
		Group:      "old-group",
		AutoGroups: `["old-group","default"]`,
	}
	require.NoError(t, DB.Create(&tok).Error)

	// 4. Setup ratio_setting
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"old-group":1.2}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","old-group":"Old Desc"}`))

	// Execute UpdateGroupGlobal
	err := UpdateGroupGlobal("old-group", "new-renamed-group", 0.85, "New Desc")
	require.NoError(t, err)

	// Verify Channel
	var updatedCh Channel
	require.NoError(t, DB.First(&updatedCh, ch.Id).Error)
	assert.Equal(t, "new-renamed-group,other-group", updatedCh.Group)

	// Verify Ability
	var updatedAb Ability
	require.NoError(t, DB.Where("channel_id = ?", ch.Id).First(&updatedAb).Error)
	assert.Equal(t, "new-renamed-group", updatedAb.Group)

	// Verify User
	var updatedU User
	require.NoError(t, DB.First(&updatedU, u.Id).Error)
	assert.Equal(t, "new-renamed-group,vip", updatedU.Group)
	assert.Contains(t, updatedU.Setting, `"new-renamed-group":1.5`)
	assert.NotContains(t, updatedU.Setting, `"old-group"`)
	assert.Contains(t, updatedU.Setting, `"group":"new-renamed-group"`)

	// Verify Token
	var updatedTok Token
	require.NoError(t, DB.First(&updatedTok, tok.Id).Error)
	assert.Equal(t, "new-renamed-group", updatedTok.Group)
	assert.Contains(t, updatedTok.AutoGroups, `"new-renamed-group"`)
	assert.NotContains(t, updatedTok.AutoGroups, `"old-group"`)

	// Verify GroupRatio
	assert.False(t, ratio_setting.ContainsGroupRatio("old-group"))
	assert.True(t, ratio_setting.ContainsGroupRatio("new-renamed-group"))
	assert.Equal(t, 0.85, ratio_setting.GetGroupRatio("new-renamed-group"))

	// Verify UserUsableGroups
	uug := setting.GetUserUsableGroupsCopy()
	assert.Equal(t, "New Desc", uug["new-renamed-group"])
	_, hasOld := uug["old-group"]
	assert.False(t, hasOld)
}

func TestUpdateGroupGlobalRatioOnly(t *testing.T) {
	setupChannelStatusTest(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"group-a":1.0}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"group-a":"Desc A"}`))

	err := UpdateGroupGlobal("group-a", "group-a", 0.45, "Updated Desc A")
	require.NoError(t, err)

	assert.Equal(t, 0.45, ratio_setting.GetGroupRatio("group-a"))
	uug := setting.GetUserUsableGroupsCopy()
	assert.Equal(t, "Updated Desc A", uug["group-a"])
}

func TestUpdateGroupGlobalValidation(t *testing.T) {
	setupChannelStatusTest(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))

	// Empty name
	assert.Error(t, UpdateGroupGlobal("", "new", 1, ""))
	assert.Error(t, UpdateGroupGlobal("old", "", 1, ""))

	// Auto group
	assert.Error(t, UpdateGroupGlobal("auto", "new", 1, ""))
	assert.Error(t, UpdateGroupGlobal("old", "auto", 1, ""))

	// Negative ratio
	assert.Error(t, UpdateGroupGlobal("old", "new", -0.1, ""))

	// Duplicate target name
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"existing-grp":1.0}`))
	err := UpdateGroupGlobal("old", "existing-grp", 1.0, "")
	assert.ErrorContains(t, err, "已存在")
}

