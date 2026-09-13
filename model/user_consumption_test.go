package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func setupUserConsumptionTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&User{}, &Log{}, &UserConsumptionDailyStat{}))
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)
	require.NoError(t, DB.Exec("DELETE FROM user_consumption_daily_stats").Error)

	userConsumptionStatsCacheLock.Lock()
	cachedUserConsumptionStats = nil
	userConsumptionStatsCacheLock.Unlock()

	userTodayQuotaCacheLock.Lock()
	userTodayQuotaCache = userTodayQuotaCacheEntry{}
	userTodayQuotaCacheLock.Unlock()
}

func TestGetUserConsumptionStatsAndSorting(t *testing.T) {
	setupUserConsumptionTest(t)

	now := time.Now().Unix()
	todayStart := shanghaiTodayStartUnix()

	// 1. Create Users:
	// Admin user (role 10)
	admin := &User{
		Username:  "admin",
		AffCode:   "aff_admin",
		Role:      common.RoleAdminUser,
		Status:    common.UserStatusEnabled,
		Quota:     1000000,
		UsedQuota: 50000,
	}
	require.NoError(t, DB.Create(admin).Error)

	// Normal user 1 (high consumption today)
	user1 := &User{
		Username:  "alice",
		AffCode:   "aff_alice",
		Role:      common.RoleCommonUser,
		Status:    common.UserStatusEnabled,
		Quota:     200000,
		UsedQuota: 800000,
	}
	require.NoError(t, DB.Create(user1).Error)

	// Normal user 2 (low consumption today, high lifetime)
	user2 := &User{
		Username:  "bob",
		AffCode:   "aff_bob",
		Role:      common.RoleCommonUser,
		Status:    common.UserStatusEnabled,
		Quota:     500000,
		UsedQuota: 1200000,
	}
	require.NoError(t, DB.Create(user2).Error)

	// Normal user 3 (zero consumption today)
	user3 := &User{
		Username:  "charlie",
		AffCode:   "aff_charlie",
		Role:      common.RoleCommonUser,
		Status:    common.UserStatusEnabled,
		Quota:     100000,
		UsedQuota: 0,
	}
	require.NoError(t, DB.Create(user3).Error)

	// 2. Create Logs:
	// Admin consumption today: 50000 (should be excluded from stats)
	require.NoError(t, DB.Create(&Log{
		UserId:    admin.Id,
		Type:      LogTypeConsume,
		Quota:     50000,
		CreatedAt: todayStart + 10,
	}).Error)

	// Alice consumption today: 300000
	require.NoError(t, DB.Create(&Log{
		UserId:    user1.Id,
		Type:      LogTypeConsume,
		Quota:     300000,
		CreatedAt: todayStart + 20,
	}).Error)

	// Bob consumption today: 100000
	require.NoError(t, DB.Create(&Log{
		UserId:    user2.Id,
		Type:      LogTypeConsume,
		Quota:     100000,
		CreatedAt: todayStart + 30,
	}).Error)

	// Test GetUserConsumptionStats
	stats, err := GetUserConsumptionStats(7)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// TodayQuota should only include non-admins: 300000 + 100000 = 400000
	require.Equal(t, int64(400000), stats.TodayQuota)

	// TotalQuota should only include non-admins: alice(800000) + bob(1200000) + charlie(0) = 2000000
	require.Equal(t, int64(2000000), stats.TotalQuota)

	// BalanceQuota should only include enabled non-admins: alice(200000) + bob(500000) + charlie(100000) = 800000
	require.Equal(t, int64(800000), stats.BalanceQuota)

	// Test GetAllUsers sorted by today_consumed_quota desc
	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	users, total, err := GetAllUsers(pageInfo, UserSortOptions{SortBy: "today_consumed_quota", SortOrder: "desc"})
	require.NoError(t, err)
	require.Equal(t, int64(4), total)
	require.Len(t, users, 4)

	// Check order: Alice (300000) > Bob (100000) > remaining (Charlie, Admin with 0)
	require.Equal(t, user1.Id, users[0].Id)
	require.Equal(t, int64(300000), users[0].TodayConsumedQuota)
	require.Equal(t, user2.Id, users[1].Id)
	require.Equal(t, int64(100000), users[1].TodayConsumedQuota)

	// Test GetAllUsers sorted by total_consumed_quota desc
	users, total, err = GetAllUsers(pageInfo, UserSortOptions{SortBy: "total_consumed_quota", SortOrder: "desc"})
	require.NoError(t, err)
	require.Equal(t, int64(4), total)
	// Bob (1200000) > Alice (800000) > Admin (50000) > Charlie (0)
	require.Equal(t, user2.Id, users[0].Id)
	require.Equal(t, int64(1200000), users[0].TotalConsumedQuota)
	require.Equal(t, user1.Id, users[1].Id)
	require.Equal(t, int64(800000), users[1].TotalConsumedQuota)

	// Test SearchUsers with today_consumed_quota
	searchedUsers, searchTotal, err := SearchUsers("alice", "", nil, nil, 0, 10, UserSortOptions{SortBy: "today_consumed_quota", SortOrder: "desc"})
	require.NoError(t, err)
	require.Equal(t, int64(1), searchTotal)
	require.Len(t, searchedUsers, 1)
	require.Equal(t, user1.Id, searchedUsers[0].Id)
	require.Equal(t, int64(300000), searchedUsers[0].TodayConsumedQuota)
	_ = now
}
