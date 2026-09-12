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
