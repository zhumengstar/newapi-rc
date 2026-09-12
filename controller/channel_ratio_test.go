package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateChannelRatio(t *testing.T) {
	tests := []struct {
		name    string
		ratio   float64
		wantErr string
	}{
		{name: "zero is preserved", ratio: 0},
		{name: "decimal ratio", ratio: 0.065},
		{name: "negative ratio", ratio: -0.01, wantErr: "channel ratio"},
		{name: "ratio above limit", ratio: maxChannelRatio + 1, wantErr: "channel ratio"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{ChannelRatio: &test.ratio}
			err := validateChannel(channel, false)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestUpdateChannelRatioOnlyDoesNotRebuildAbilities(t *testing.T) {
	setupTaskPluginBindChannelTest(t)
	weight := uint(10)
	channel := model.Channel{
		Type:   1,
		Status: common.ChannelStatusEnabled,
		Name:   "ratio-only",
		Key:    "key",
		Models: "model-a",
		Group:  "default",
		Weight: &weight,
	}
	require.NoError(t, channel.Insert())
	require.NoError(t, model.DB.Model(&model.Ability{}).
		Where("channel_id = ?", channel.Id).
		Update("weight", 777).Error)

	callbackName := "test:channel_ratio_update_avoids_ability_rebuild"
	deleteAttempts := 0
	callbackRegistered := true
	require.NoError(t, model.DB.Callback().Delete().Before("gorm:delete").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "abilities" {
			deleteAttempts++
			tx.AddError(errors.New("channel ratio update must not rebuild abilities"))
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Delete().Remove(callbackName)
		}
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 1)
	context.Set("role", common.RoleRootUser)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/channel/",
		strings.NewReader(fmt.Sprintf(`{"id":%d,"channel_ratio":0.125}`, channel.Id)),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Zero(t, deleteAttempts)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	require.NotNil(t, stored.ChannelRatio)
	assert.InDelta(t, 0.125, *stored.ChannelRatio, 1e-12)

	var ability model.Ability
	require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
	assert.Equal(t, uint(777), ability.Weight)

	require.NoError(t, model.DB.Callback().Delete().Remove(callbackName))
	callbackRegistered = false
}
