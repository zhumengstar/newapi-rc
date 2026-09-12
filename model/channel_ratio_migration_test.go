package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestParseChannelNameRatio(t *testing.T) {
	tests := []struct {
		name        string
		wantRatio   float64
		wantName    string
		wantMatched bool
	}{
		{name: "0.03-Provider", wantRatio: 0.03, wantName: "Provider", wantMatched: true},
		{name: "0.03-倍率https://example.com", wantRatio: 0.03, wantName: "https://example.com", wantMatched: true},
		{name: " 1.5 - Provider  ", wantRatio: 1.5, wantName: "Provider", wantMatched: true},
		{name: "0.0-免费渠道", wantRatio: 0, wantName: "免费渠道", wantMatched: true},
		{name: "007-Provider", wantRatio: 0.07, wantName: "Provider", wantMatched: true},
		{name: "0065-Provider", wantRatio: 0.065, wantName: "Provider", wantMatched: true},
		{name: "012pro-Provider", wantRatio: 0.12, wantName: "pro-Provider", wantMatched: true},
		{name: "012pro-倍率https://example.com", wantRatio: 0.12, wantName: "pro-https://example.com", wantMatched: true},
		{name: "01-Provider", wantRatio: 0.1, wantName: "Provider", wantMatched: true},
		{name: "000-Provider", wantMatched: false},
		{name: "3-Provider", wantMatched: false},
		{name: "Provider-1.5", wantMatched: false},
		{name: "0.5-gpt-4.1", wantRatio: 0.5, wantName: "gpt-4.1", wantMatched: true},
		{name: "0.5-", wantMatched: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ratio, cleanedName, matched := ParseChannelNameRatio(test.name)
			assert.Equal(t, test.wantMatched, matched)
			if test.wantMatched {
				assert.InDelta(t, test.wantRatio, ratio, 1e-12)
				assert.Equal(t, test.wantName, cleanedName)
			}
		})
	}
}

func TestMigrateChannelNameRatiosIsIdempotent(t *testing.T) {
	t.Setenv("MIGRATE_CHANNEL_NAME_RATIO_COMPACT_PREFIXES", "007")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}))

	existingRatio := 0.9
	rows := []Channel{
		{Name: "0.03-Provider", Key: "key-1"},
		{Name: "007-Provider-ID", Key: "key-2"},
		{Name: "0.2-AlreadySet", Key: "key-3", ChannelRatio: &existingRatio},
	}
	require.NoError(t, db.Create(&rows).Error)

	result, err := MigrateChannelNameRatios(db)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Scanned)
	assert.Equal(t, 2, result.Migrated)

	var migrated Channel
	require.NoError(t, db.First(&migrated, "name = ?", "Provider").Error)
	require.NotNil(t, migrated.ChannelRatio)
	assert.InDelta(t, 0.03, *migrated.ChannelRatio, 1e-12)

	var compact Channel
	require.NoError(t, db.First(&compact, "name = ?", "Provider-ID").Error)
	require.NotNil(t, compact.ChannelRatio)
	assert.InDelta(t, 0.07, *compact.ChannelRatio, 1e-12)

	result, err = MigrateChannelNameRatios(db)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Scanned)
	assert.Equal(t, 0, result.Migrated)
	assert.Equal(t, 0, result.Cleaned)
}

func TestMigrateChannelNameRatiosCleansLegacyURLMarker(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}))

	ratio := 0.07
	rows := []Channel{
		{Name: "倍率https://example.com", Key: "key-1", ChannelRatio: &ratio},
		{Name: "pro-倍率https://example.com", Key: "key-2", ChannelRatio: &ratio},
		{Name: "倍率Provider", Key: "key-3", ChannelRatio: &ratio},
	}
	require.NoError(t, db.Create(&rows).Error)

	result, err := MigrateChannelNameRatios(db)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Scanned)
	assert.Equal(t, 0, result.Migrated)
	assert.Equal(t, 2, result.Cleaned)

	var firstCleaned Channel
	require.NoError(t, db.First(&firstCleaned, rows[0].Id).Error)
	assert.Equal(t, "https://example.com", firstCleaned.Name)

	var secondCleaned Channel
	require.NoError(t, db.First(&secondCleaned, rows[1].Id).Error)
	assert.Equal(t, "pro-https://example.com", secondCleaned.Name)

	var untouched Channel
	require.NoError(t, db.First(&untouched, rows[2].Id).Error)
	assert.Equal(t, "倍率Provider", untouched.Name)
}

func TestMigrateChannelNameRatiosRequiresCompactPrefixAllowlist(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}))
	require.NoError(t, db.Create(&Channel{Name: "007-Provider", Key: "key-1"}).Error)

	result, err := MigrateChannelNameRatios(db)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Scanned)
	assert.Equal(t, 0, result.Migrated)

	var channel Channel
	require.NoError(t, db.First(&channel).Error)
	assert.Equal(t, "007-Provider", channel.Name)
	assert.Nil(t, channel.ChannelRatio)
}

func TestMigrateChannelNameRatioUsesMatchingGroupRatio(t *testing.T) {
	t.Setenv("MIGRATE_CHANNEL_NAME_RATIO_INTEGER_PREFIXES", "3")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}))
	require.NoError(t, db.Create(&Option{
		Key:   "GroupRatio",
		Value: `{"aswb专用分组外接":3}`,
	}).Error)
	rows := []Channel{
		{Name: "3-aswb-Provider", Group: "aswb专用分组外接", Key: "key-1"},
		{Name: "4-aswb-Provider", Group: "aswb专用分组外接", Key: "key-2"},
		{Name: "3-other-Provider", Group: "other", Key: "key-3"},
	}
	require.NoError(t, db.Create(&rows).Error)

	result, err := MigrateChannelNameRatios(db)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Scanned)
	assert.Equal(t, 1, result.Migrated)

	var migrated Channel
	require.NoError(t, db.First(&migrated, "name = ?", "aswb-Provider").Error)
	require.NotNil(t, migrated.ChannelRatio)
	assert.InDelta(t, 3, *migrated.ChannelRatio, 1e-12)

	var unmatched Channel
	require.NoError(t, db.First(&unmatched, "name = ?", "4-aswb-Provider").Error)
	assert.Nil(t, unmatched.ChannelRatio)
}

func TestMigrateChannelNameRatioDoesNotInferIntegerPrefixWithoutAllowlist(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}))
	require.NoError(t, db.Create(&Option{
		Key:   "GroupRatio",
		Value: `{"aswb专用分组外接":3}`,
	}).Error)
	require.NoError(t, db.Create(&Channel{
		Name:  "3-aswb-Provider",
		Group: "aswb专用分组外接",
		Key:   "key-1",
	}).Error)

	result, err := MigrateChannelNameRatios(db)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Scanned)
	assert.Equal(t, 0, result.Migrated)

	var channel Channel
	require.NoError(t, db.First(&channel).Error)
	assert.Equal(t, "3-aswb-Provider", channel.Name)
	assert.Nil(t, channel.ChannelRatio)
}

func TestMigrateChannelNameRatiosIfRequestedRunsOnlyOnce(t *testing.T) {
	t.Setenv("MIGRATE_CHANNEL_NAME_RATIOS", "true")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Option{}))
	require.NoError(t, db.Create(&Channel{Name: "0.03-First", Key: "key-1"}).Error)

	require.NoError(t, MigrateChannelNameRatiosIfRequested(db))
	var marker Option
	require.NoError(t, db.Where(&Option{Key: channelNameRatioMigrationOptionKey}).First(&marker).Error)
	assert.Equal(t, channelNameRatioMigrationOptionValue, marker.Value)

	lateChannel := Channel{Name: "0.2-Later", Key: "key-2"}
	require.NoError(t, db.Create(&lateChannel).Error)
	require.NoError(t, MigrateChannelNameRatiosIfRequested(db))

	var stored Channel
	require.NoError(t, db.First(&stored, lateChannel.Id).Error)
	assert.Equal(t, "0.2-Later", stored.Name)
	assert.Nil(t, stored.ChannelRatio)
}
