package model

import (
	"context"
	"database/sql"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelStatusTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	memoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = memoryCacheEnabled
	})
}

func TestUpdateChannelStatusPersistsMultiKeyState(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "multi-key-status",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         2,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 1,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)

	changed := UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "provider rejected key")
	require.True(t, changed)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, "provider rejected key", stored.ChannelInfo.MultiKeyDisabledReason[0])
	assert.NotZero(t, stored.ChannelInfo.MultiKeyDisabledTime[0])
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
}

func TestUpdateChannelStatusClearsRecoveryProbeState(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{Name: "recovery-state", Key: "test-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, DB.Create(&channel).Error)
	_, err := RecordChannelRecoveryProbe(context.Background(), channel.Id, true)
	require.NoError(t, err)

	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, "test"))

	var count int64
	require.NoError(t, DB.Model(&ChannelRecoveryState{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSaveStatusStateFromSingleKeySnapshotPreservesUnownedColumns(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:        "single-key-status",
		Key:         "original-key",
		Status:      common.ChannelStatusEnabled,
		Models:      "original-model",
		Group:       "default",
		UsedQuota:   100,
		ChannelInfo: ChannelInfo{},
	}
	require.NoError(t, DB.Create(&channel).Error)

	stale, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)

	concurrentChannelInfo := ChannelInfo{
		IsMultiKey:           true,
		MultiKeySize:         2,
		MultiKeyMode:         constant.MultiKeyModePolling,
		MultiKeyPollingIndex: 1,
	}
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"key":          "rotated-key",
		"used_quota":   gorm.Expr("used_quota + ?", 250),
		"models":       "concurrent-model",
		"channel_info": concurrentChannelInfo,
	}).Error)

	stale.Status = common.ChannelStatusManuallyDisabled
	stale.SetOtherInfo(map[string]any{
		"status_reason": "manual operation",
		"status_time":   int64(1234),
	})
	require.NoError(t, stale.saveStatusState())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Equal(t, "rotated-key", stored.Key)
	assert.Equal(t, int64(350), stored.UsedQuota)
	assert.Equal(t, "concurrent-model", stored.Models)
	assert.Equal(t, concurrentChannelInfo, stored.ChannelInfo)

	otherInfo := stored.GetOtherInfo()
	assert.Equal(t, "manual operation", otherInfo["status_reason"])
	assert.Equal(t, float64(1234), otherInfo["status_time"])
}

func TestUpdateAbilitiesPreservesAdaptiveWeights(t *testing.T) {
	setupChannelStatusTest(t)

	weight := uint(10)
	channel := Channel{
		Name:            "adaptive-routing",
		Key:             "key",
		Status:          common.ChannelStatusEnabled,
		Models:          "model-a",
		Group:           "adaptive-group",
		Weight:          &weight,
		AdaptiveEnabled: true,
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	require.NoError(t, DB.Model(&Ability{}).
		Where("channel_id = ? and model = ?", channel.Id, "model-a").
		Update("weight", 777).Error)

	// Upstream model refreshes may pass a reduced channel projection. The
	// persisted adaptive flag must still preserve the existing model weight.
	channel.AdaptiveEnabled = false
	channel.Models = "model-a,model-b"
	require.NoError(t, channel.UpdateAbilities(nil))

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Order("model").Find(&abilities).Error)
	require.Len(t, abilities, 2)
	assert.Equal(t, uint(777), abilities[0].Weight)
	assert.Equal(t, uint(10), abilities[1].Weight)
}

func TestUpdateAbilitiesTreatsLegacyNullAdaptiveEnabledAsDisabled(t *testing.T) {
	setupChannelStatusTest(t)

	weight := uint(10)
	channel := Channel{
		Name:   "legacy-adaptive-routing",
		Key:    "key",
		Status: common.ChannelStatusEnabled,
		Models: "model-a",
		Group:  "adaptive-group",
		Weight: &weight,
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, DB.Exec("UPDATE channels SET adaptive_enabled = NULL WHERE id = ?", channel.Id).Error)

	require.NoError(t, channel.UpdateAbilities(nil))

	var persisted sql.NullBool
	require.NoError(t, DB.Model(&Channel{}).
		Select("adaptive_enabled").
		Where("id = ?", channel.Id).
		Scan(&persisted).Error)
	assert.False(t, persisted.Valid)
}

func TestNormalizeChannelAdaptiveEnabledBackfillsLegacyNull(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{Name: "legacy-adaptive-default", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, DB.Exec("UPDATE channels SET adaptive_enabled = NULL WHERE id = ?", channel.Id).Error)

	require.NoError(t, normalizeChannelAdaptiveEnabled(DB))

	var persisted sql.NullBool
	require.NoError(t, DB.Model(&Channel{}).
		Select("adaptive_enabled").
		Where("id = ?", channel.Id).
		Scan(&persisted).Error)
	assert.True(t, persisted.Valid)
	assert.False(t, persisted.Bool)
}
