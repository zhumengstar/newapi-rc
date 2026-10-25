package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelExclusiveUpdateRequest(t *testing.T) {
	rawJson := `{"group":"default,vip","allowed_user_ids":[101, 202, 101],"status":1}`
	var req ChannelExclusiveUpdateRequest
	err := json.Unmarshal([]byte(rawJson), &req)
	require.NoError(t, err)

	require.NotNil(t, req.Group)
	require.Equal(t, "default,vip", *req.Group)

	require.NotNil(t, req.AllowedUserIds)
	require.Equal(t, []int{101, 202, 101}, *req.AllowedUserIds)

	require.NotNil(t, req.Status)
	require.Equal(t, 1, *req.Status)

	// Partial updates: only update allowed_user_ids to empty (clear)
	clearJson := `{"allowed_user_ids":[]}`
	var reqClear ChannelExclusiveUpdateRequest
	err = json.Unmarshal([]byte(clearJson), &reqClear)
	require.NoError(t, err)

	require.Nil(t, reqClear.Group)
	require.Nil(t, reqClear.Status)
	require.NotNil(t, reqClear.AllowedUserIds)
	require.Len(t, *reqClear.AllowedUserIds, 0)
}
