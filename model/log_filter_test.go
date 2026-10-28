package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestShouldFilterErrorLog(t *testing.T) {
	channelSyncLock.Lock()
	oldChannelsIDM := channelsIDM
	channelsIDM = map[int]*Channel{
		100: {
			Id:     100,
			Status: common.ChannelStatusManuallyDisabled, // 已禁用
		},
		200: {
			Id:     200,
			Status: common.ChannelStatusEnabled, // 启用
		},
	}
	channelSyncLock.Unlock()

	defer func() {
		channelSyncLock.Lock()
		channelsIDM = oldChannelsIDM
		channelSyncLock.Unlock()
	}()

	// 1. 已禁用渠道的报错必须被过滤
	assert.True(t, shouldFilterErrorLog(100, "status_code=500, some upstream failure"))

	// 2. 403 权限控制/分组禁用必须被过滤
	assert.True(t, shouldFilterErrorLog(0, "status_code=403, 无权访问 Gemini组 分组"))
	assert.True(t, shouldFilterErrorLog(200, "status_code=403, 无权访问 Gemini4按次组 分组"))
	assert.True(t, shouldFilterErrorLog(0, "status_code=403, user has been banned"))

	// 3. 503 / 400 无可用渠道、无可用账号、渠道已禁用必须被过滤
	assert.True(t, shouldFilterErrorLog(200, "status_code=503, no available Gemini account"))
	assert.True(t, shouldFilterErrorLog(0, "status_code=503, 当前分组无可用渠道"))
	assert.True(t, shouldFilterErrorLog(0, "status_code=400, 渠道已禁用"))
	assert.True(t, shouldFilterErrorLog(0, "status_code=503, origin_task_channel_disabled"))

	// 4. 正常非预期系统故障（如 500 数据库错误、502 网关故障、正常渠道 500）不被过滤
	assert.False(t, shouldFilterErrorLog(200, "status_code=500, internal server error occurred"))
	assert.False(t, shouldFilterErrorLog(0, "status_code=500, database dead lock"))
}
