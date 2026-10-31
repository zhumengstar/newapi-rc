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

	// 3. 普通用户 999 发起请求：绝不能路由到已禁用的专属渠道 380，只能路由到公共渠道 365
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
	assert.Equal(t, 365, selectedCh2.Id, "普通用户走公共渠道 365")

	// 4. 核心安全验证：启用的渠道即使配置了指定专属用户，普通用户依然可以正常访问（确保普通大客户不发生断流）
	priority20 := int64(20)
	ch428 := &model.Channel{
		Id:       428,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-428",
		Status:   common.ChannelStatusEnabled, // 启用状态
		Name:     "渠道428(启用的专属渠道)",
		Group:    "cc反重力按量",
		Models:   "claude-opus-4-6",
		Priority: &priority20,
		Weight:   &weight1,
	}
	ch428.SetOtherSettings(kitdto.ChannelOtherSettings{
		AllowedUserIds: []int{382}, // 指定了客户 382
	})
	require.NoError(t, db.Create(ch428).Error)
	model.InitChannelCache()

	// 普通客户 435 请求该分组该模型，能够正常命中启用的渠道 428，绝不会报错无可用渠道
	w3 := httptest.NewRecorder()
	ctx3, _ := gin.CreateTestContext(w3)
	ctx3.Set("id", 435)
	retry3 := 0
	param3 := &RetryParam{
		Ctx:         ctx3,
		TokenGroup:  "cc反重力按量",
		ModelName:   "claude-opus-4-6",
		RequestPath: "/v1/messages",
		Retry:       &retry3,
	}
	selectedCh3, _, err3 := CacheGetRandomSatisfiedChannel(param3)
	require.NoError(t, err3)
	require.NotNil(t, selectedCh3)
	assert.Equal(t, 428, selectedCh3.Id, "普通客户在无其他公共渠道时依然能正常访问启用的渠道，绝不断流")

	// 受限客户 382 请求时，也正常且只能走渠道 428
	w4 := httptest.NewRecorder()
	ctx4, _ := gin.CreateTestContext(w4)
	ctx4.Set("id", 382)
	retry4 := 0
	param4 := &RetryParam{
		Ctx:         ctx4,
		TokenGroup:  "cc反重力按量",
		ModelName:   "claude-opus-4-6",
		RequestPath: "/v1/messages",
		Retry:       &retry4,
	}
	selectedCh4, _, err4 := CacheGetRandomSatisfiedChannel(param4)
	require.NoError(t, err4)
	require.NotNil(t, selectedCh4)
	assert.Equal(t, 428, selectedCh4.Id, "专属客户只能走其指定的渠道 428")
}

func TestSamePriorityMultiChannelAndMultiPassRetries(t *testing.T) {
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

	prioHigh := int64(777)
	prioLow := int64(666)
	w20 := uint(20)
	w10 := uint(10)

	// Priority 777 has two channels: 501 (weight 20) and 502 (weight 10)
	// Priority 666 has one channel: 503 (weight 10)
	for _, ch := range []*model.Channel{
		{Id: 501, Type: constant.ChannelTypeOpenAI, Key: "k1", Status: common.ChannelStatusEnabled, Name: "ch-501", Group: "test-group", Models: "gemini-4-argon", Priority: &prioHigh, Weight: &w20},
		{Id: 502, Type: constant.ChannelTypeOpenAI, Key: "k2", Status: common.ChannelStatusEnabled, Name: "ch-502", Group: "test-group", Models: "gemini-4-argon", Priority: &prioHigh, Weight: &w10},
		{Id: 503, Type: constant.ChannelTypeOpenAI, Key: "k3", Status: common.ChannelStatusEnabled, Name: "ch-503", Group: "test-group", Models: "gemini-4-argon", Priority: &prioLow, Weight: &w10},
	} {
		require.NoError(t, db.Create(ch).Error)
		require.NoError(t, db.Create(&model.Ability{
			Group: "test-group", Model: "gemini-4-argon", ChannelId: ch.Id, Enabled: true, Priority: ch.Priority, Weight: *ch.Weight,
		}).Error)
	}
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	retry := 0
	param := &RetryParam{
		Ctx:            ctx,
		TokenGroup:     "test-group",
		ModelName:      "gemini-4-argon",
		RequestPath:    "/v1/chat/completions",
		Retry:          &retry,
		AllowMultiPass: true,
	}

	var sequence []int
	for attempt := 0; attempt <= 8; attempt++ {
		param.Retry = &attempt
		ch, _, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, ch, "attempt %d must not return nil", attempt)
		sequence = append(sequence, ch.Id)
		param.MarkChannelTried(ch.Id)
	}

	// First 4 attempts must stay in Priority 777 alternating between 501 and 502 (2 full rounds):
	assert.Contains(t, []int{501, 502}, sequence[0])
	assert.Contains(t, []int{501, 502}, sequence[1])
	assert.NotEqual(t, sequence[0], sequence[1], "must switch to the other channel in the same priority")
	assert.Equal(t, sequence[0], sequence[2], "round 2 in same priority must alternate")
	assert.Equal(t, sequence[1], sequence[3], "round 2 in same priority must complete second channel")
	// 5th attempt (index 4) drops to Priority 666 (channel 503):
	assert.Equal(t, 503, sequence[4], "after 2 rounds in priority 777, must fall through to priority 666")
	// 6th..9th attempts (indices 5..8) start Cycle 2 back at Priority 777:
	assert.Contains(t, []int{501, 502}, sequence[5])
	assert.Contains(t, []int{501, 502}, sequence[6])
	assert.NotEqual(t, sequence[5], sequence[6])
}
