package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestCacheGetRandomSatisfiedChannelRestrictedDisabledChannelRouting(t *testing.T) {
	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = true

	defer func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	}()

	priority10 := int64(10)
	priority5 := int64(5)
	weight1 := uint(100)

	// 插入专属渠道 380（已禁用）
	ch380 := &model.Channel{
		Id:       380,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-380",
		Status:   common.ChannelStatusManuallyDisabled, // 禁用
		Name:     "专属渠道380(禁用)",
		Group:    "Gemini组",
		Models:   "gemini-2.5-flash",
		Priority: &priority10,
		Weight:   &weight1,
	}
	ch380.SetOtherSettings(kitdto.ChannelOtherSettings{
		AllowedUserIds: []int{371, 563},
	})
	require.NoError(t, db.Create(ch380).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "Gemini组",
		Model:     "gemini-2.5-flash",
		ChannelId: 380,
		Enabled:   false,
		Priority:  &priority10,
		Weight:    weight1,
	}).Error)

	// 插入公共渠道 365（启用）
	ch365 := &model.Channel{
		Id:       365,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-365",
		Status:   common.ChannelStatusEnabled, // 启用
		Name:     "公共渠道365",
		Group:    "Gemini组",
		Models:   "gemini-2.5-flash,claude-3-5-sonnet",
		Priority: &priority5,
		Weight:   &weight1,
	}
	require.NoError(t, db.Create(ch365).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "Gemini组",
		Model:     "gemini-2.5-flash",
		ChannelId: 365,
		Enabled:   true,
		Priority:  &priority5,
		Weight:    weight1,
	}).Error)

	model.InitChannelCache()

	gin.SetMode(gin.TestMode)

	// 1. 专属用户 371 发起请求：即使专属渠道 380 被禁用，也必须走渠道 380
	w1 := httptest.NewRecorder()
	ctx1, _ := gin.CreateTestContext(w1)
	ctx1.Set("id", 371)
	retry1 := 0
	param1 := &RetryParam{
		Ctx:         ctx1,
		TokenGroup:  "Gemini组",
		ModelName:   "gemini-2.5-flash",
		RequestPath: "/v1/chat/completions",
		Retry:       &retry1,
	}
	selectedCh1, _, err1 := CacheGetRandomSatisfiedChannel(param1)
	require.NoError(t, err1)
	require.NotNil(t, selectedCh1)
	assert.Equal(t, 380, selectedCh1.Id, "专属用户即使专属渠道被禁用也必须路由到渠道 380")

	// 2. 专属用户 371 重试排除 380 之后，绝不能逃逸到公共渠道 365
	param1.MarkChannelTried(380)
	selectedCh1Retry, _, err1Retry := CacheGetRandomSatisfiedChannel(param1)
	assert.NoError(t, err1Retry)
	assert.Nil(t, selectedCh1Retry, "排除专属渠道后绝不能逃逸到公共渠道 365")

	// 3. 普通用户 999 发起请求：绝不能路由到渠道 380，只能路由到公共渠道 365
	w2 := httptest.NewRecorder()
	ctx2, _ := gin.CreateTestContext(w2)
	ctx2.Set("id", 999)
	retry2 := 0
	param2 := &RetryParam{
		Ctx:         ctx2,
		TokenGroup:  "Gemini组",
		ModelName:   "gemini-2.5-flash",
		RequestPath: "/v1/chat/completions",
		Retry:       &retry2,
	}
	selectedCh2, _, err2 := CacheGetRandomSatisfiedChannel(param2)
	require.NoError(t, err2)
	require.NotNil(t, selectedCh2)
	assert.Equal(t, 365, selectedCh2.Id, "普通用户绝对只能走公共渠道 365")
}
