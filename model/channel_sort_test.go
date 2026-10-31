package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllChannelsSortsByUsedQuota(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	priorityLow := int64(10)
	priorityHigh := int64(30)
	priorityMiddle := int64(20)
	channels := []Channel{
		{Name: "quota-high-a", Key: "key-a", UsedQuota: 300, Priority: &priorityLow},
		{Name: "quota-low", Key: "key-b", UsedQuota: 100, Priority: &priorityHigh},
		{Name: "quota-high-b", Key: "key-c", UsedQuota: 300, Priority: &priorityMiddle},
	}
	require.NoError(t, DB.Create(&channels).Error)

	tests := []struct {
		name  string
		order string
		want  []string
	}{
		{
			name:  "ascending",
			order: "asc",
			want:  []string{"quota-low", "quota-high-a", "quota-high-b"},
		},
		{
			name:  "descending",
			order: "desc",
			want:  []string{"quota-high-a", "quota-high-b", "quota-low"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := GetAllChannels(
				0,
				10,
				false,
				false,
				NewChannelSortOptions("used_quota", test.order, false),
			)
			require.NoError(t, err)

			names := make([]string, len(got))
			for i, channel := range got {
				names[i] = channel.Name
			}
			assert.Equal(t, test.want, names)
		})
	}
}

func TestGetAllChannelsSortsByChannelRatio(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	lowRatio := 0.05
	highRatio := 1.5
	channels := []Channel{
		{Name: "ratio-unset", Key: "key-a"},
		{Name: "ratio-high", Key: "key-b", ChannelRatio: &highRatio},
		{Name: "ratio-low", Key: "key-c", ChannelRatio: &lowRatio},
	}
	require.NoError(t, DB.Create(&channels).Error)

	got, err := GetAllChannels(
		0,
		10,
		false,
		false,
		NewChannelSortOptions("channel_ratio", "desc", false),
	)
	require.NoError(t, err)

	names := make([]string, len(got))
	for i, channel := range got {
		names[i] = channel.Name
	}
	assert.Equal(t, []string{"ratio-high", "ratio-low", "ratio-unset"}, names)
}

func TestGetAllChannelsSortsByPriorityAndWeight(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	p10 := int64(10)
	p20 := int64(20)
	w50 := uint(50)
	w80 := uint(80)
	w30 := uint(30)

	channels := []Channel{
		{Name: "p20-w50", Key: "k1", Priority: &p20, Weight: &w50},
		{Name: "p20-w80", Key: "k2", Priority: &p20, Weight: &w80},
		{Name: "p10-w30", Key: "k3", Priority: &p10, Weight: &w30},
	}
	require.NoError(t, DB.Create(&channels).Error)

	// Test priority desc (should tie-break by weight desc)
	got, err := GetAllChannels(0, 10, false, false, NewChannelSortOptions("priority", "desc", false))
	require.NoError(t, err)
	names := make([]string, len(got))
	for i, ch := range got {
		names[i] = ch.Name
	}
	assert.Equal(t, []string{"p20-w80", "p20-w50", "p10-w30"}, names)

	// Test priority asc (should tie-break by weight asc)
	got, err = GetAllChannels(0, 10, false, false, NewChannelSortOptions("priority", "asc", false))
	require.NoError(t, err)
	names = make([]string, len(got))
	for i, ch := range got {
		names[i] = ch.Name
	}
	assert.Equal(t, []string{"p10-w30", "p20-w50", "p20-w80"}, names)

	// Test weight desc (should tie-break by priority desc)
	got, err = GetAllChannels(0, 10, false, false, NewChannelSortOptions("weight", "desc", false))
	require.NoError(t, err)
	names = make([]string, len(got))
	for i, ch := range got {
		names[i] = ch.Name
	}
	assert.Equal(t, []string{"p20-w80", "p20-w50", "p10-w30"}, names)

	// Test weight asc
	got, err = GetAllChannels(0, 10, false, false, NewChannelSortOptions("weight", "asc", false))
	require.NoError(t, err)
	names = make([]string, len(got))
	for i, ch := range got {
		names[i] = ch.Name
	}
	assert.Equal(t, []string{"p10-w30", "p20-w50", "p20-w80"}, names)
}

func TestGetAllChannelsSortsByGroup(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	p10 := int64(10)
	p20 := int64(20)

	channels := []Channel{
		{Name: "group-b-p10", Key: "k1", Group: "beta", Priority: &p10},
		{Name: "group-a", Key: "k2", Group: "alpha", Priority: &p10},
		{Name: "group-b-p20", Key: "k3", Group: "beta", Priority: &p20},
	}
	require.NoError(t, DB.Create(&channels).Error)

	// Ascending by group, with tie-break priority desc
	got, err := GetAllChannels(0, 10, false, false, NewChannelSortOptions("group", "asc", false))
	require.NoError(t, err)
	names := make([]string, len(got))
	for i, ch := range got {
		names[i] = ch.Name
	}
	assert.Equal(t, []string{"group-a", "group-b-p20", "group-b-p10"}, names)

	// Descending by group, with tie-break priority desc
	got, err = GetAllChannels(0, 10, false, false, NewChannelSortOptions("group", "desc", false))
	require.NoError(t, err)
	names = make([]string, len(got))
	for i, ch := range got {
		names[i] = ch.Name
	}
	assert.Equal(t, []string{"group-b-p20", "group-b-p10", "group-a"}, names)
}

func TestGetChannelApiAddresses(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	u1 := "https://api.example.com"
	u2 := "https://api.svip.com"
	u3 := "https://api.other.com"

	channels := []Channel{
		{Name: "ch1", Key: "k1", Group: "group_a", BaseURL: &u1, UsedQuota: 1000},
		{Name: "ch2", Key: "k2", Group: "group_a", BaseURL: &u1, UsedQuota: 2500},
		{Name: "ch3", Key: "k3", Group: "group_b", BaseURL: &u2, UsedQuota: 500},
		{Name: "ch4", Key: "k4", Group: "group_a", BaseURL: &u3, UsedQuota: 100},
	}
	require.NoError(t, DB.Create(&channels).Error)

	// All groups - should be sorted by total used_quota desc
	allAddresses, err := GetChannelApiAddresses("")
	require.NoError(t, err)
	require.Len(t, allAddresses, 3)
	assert.Equal(t, "https://api.example.com", allAddresses[0].BaseURL)
	assert.Equal(t, int64(3500), allAddresses[0].UsedQuota)
	assert.Equal(t, 2, allAddresses[0].ChannelCount)
	assert.Equal(t, "https://api.svip.com", allAddresses[1].BaseURL)
	assert.Equal(t, int64(500), allAddresses[1].UsedQuota)
	assert.Equal(t, "https://api.other.com", allAddresses[2].BaseURL)
	assert.Equal(t, int64(100), allAddresses[2].UsedQuota)

	// Filter by group "group_a"
	vipAddresses, err := GetChannelApiAddresses("group_a")
	require.NoError(t, err)
	require.Len(t, vipAddresses, 2)
	assert.Equal(t, "https://api.example.com", vipAddresses[0].BaseURL)
	assert.Equal(t, int64(3500), vipAddresses[0].UsedQuota)
	assert.Equal(t, "https://api.other.com", vipAddresses[1].BaseURL)
	assert.Equal(t, int64(100), vipAddresses[1].UsedQuota)

	// Large time range (e.g. 30 days) - should not hang and return full addresses
	largeRangeAddrs, err := GetChannelApiAddresses("", "", int64(1790870400), int64(1793462399))
	require.NoError(t, err)
	require.Len(t, largeRangeAddrs, 3)
	assert.Equal(t, "https://api.example.com", largeRangeAddrs[0].BaseURL)
	assert.Equal(t, int64(3500), largeRangeAddrs[0].UsedQuota)
}
