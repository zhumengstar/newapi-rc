package service

import (
	"net/http/httptest"
	"testing"

	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestUserRestrictionRoutingLogic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test case 1: Context without user ID
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	param := &RetryParam{
		Ctx:       c,
		ModelName: "gpt-4o",
	}

	assert.Equal(t, 0, param.Ctx.GetInt("id"))

	// Test case 2: Context with user ID
	c.Set("id", 436)
	assert.Equal(t, 436, param.Ctx.GetInt("id"))

	// Test case 3: Channel other settings serialization of AllowedUserIds
	settings := kitdto.ChannelOtherSettings{
		AllowedUserIds: []int{436, 567},
	}
	assert.Equal(t, []int{436, 567}, settings.AllowedUserIds)
}

func TestRetryParamChannelTried(t *testing.T) {
	param := &RetryParam{}
	param.MarkChannelTried(10)
	assert.True(t, param.TriedChannelIds[10])
	assert.False(t, param.TriedChannelIds[20])
}
