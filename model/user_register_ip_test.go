package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func setupUserRegisterIPTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&User{}))
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
}

func TestUserRegisterIPLimit(t *testing.T) {
	setupUserRegisterIPTest(t)

	oldLimit := common.MaxUsersPerIP
	common.MaxUsersPerIP = 2
	t.Cleanup(func() {
		common.MaxUsersPerIP = oldLimit
	})

	testIP := "198.51.100.1"

	// 1. Initial check should be available
	require.NoError(t, CheckRegisterIPAvailable(testIP))
	count, err := CountUsersByRegisterIP(testIP)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)

	// 2. Insert first user
	user1 := &User{
		Username:   "user_ip_1",
		Password:   "password123",
		RegisterIP: testIP,
		Role:       common.RoleCommonUser,
		Status:     common.UserStatusEnabled,
	}
	require.NoError(t, user1.Insert(0))

	count, err = CountUsersByRegisterIP(testIP)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.NoError(t, CheckRegisterIPAvailable(testIP))

	// 3. Insert second user
	user2 := &User{
		Username:   "user_ip_2",
		Password:   "password123",
		RegisterIP: testIP,
		Role:       common.RoleCommonUser,
		Status:     common.UserStatusEnabled,
	}
	require.NoError(t, user2.Insert(0))

	count, err = CountUsersByRegisterIP(testIP)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)

	// 4. Pre-check should now report limit reached
	err = CheckRegisterIPAvailable(testIP)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrIPRegisterLimitReached))

	// 5. Attempting to insert a 3rd user with same IP must fail
	user3 := &User{
		Username:   "user_ip_3",
		Password:   "password123",
		RegisterIP: testIP,
		Role:       common.RoleCommonUser,
		Status:     common.UserStatusEnabled,
	}
	err = user3.Insert(0)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrIPRegisterLimitReached))

	// 6. A different IP should still succeed
	differentIP := "198.51.100.2"
	require.NoError(t, CheckRegisterIPAvailable(differentIP))
	userOther := &User{
		Username:   "user_ip_other",
		Password:   "password123",
		RegisterIP: differentIP,
		Role:       common.RoleCommonUser,
		Status:     common.UserStatusEnabled,
	}
	require.NoError(t, userOther.Insert(0))

	// 7. Soft delete user1, 3rd registration for testIP must still be blocked
	require.NoError(t, DB.Delete(user1).Error)
	err = user3.Insert(0)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrIPRegisterLimitReached))
}
