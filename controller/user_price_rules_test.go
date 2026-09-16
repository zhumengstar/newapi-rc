package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
)

func TestApplyUserModelPriceRules(t *testing.T) {
	basePricing := []model.Pricing{
		{
			ModelName:       "gpt-4o",
			QuotaType:       0,
			ModelRatio:      2.5,
			CompletionRatio: 2.0,
			EnableGroup:     []string{"default", "vip"},
		},
		{
			ModelName:       "dall-e-3",
			QuotaType:       1,
			ModelPrice:      0.04,
			EnableGroup:     []string{"default"},
		},
	}

	t.Run("empty rules return unchanged pricing", func(t *testing.T) {
		res := applyUserModelPriceRules(basePricing, dto.UserSetting{})
		assert.Equal(t, basePricing, res)
	})

	t.Run("apply per-call rules for group", func(t *testing.T) {
		setting := dto.UserSetting{
			UserModelPriceRules: []dto.UserModelPriceRule{
				{
					Group:  "default",
					Models: []string{"gpt-4o"},
					Price:  0.05,
				},
			},
		}
		res := applyUserModelPriceRules(basePricing, setting)
		assert.Len(t, res, 2)
		assert.Equal(t, 1, res[0].QuotaType)
		assert.Equal(t, 1.0, res[0].ModelRatio)
		assert.Equal(t, 1.0, res[0].CompletionRatio)
		assert.Equal(t, 0.05, res[0].UserGroupPrices["default"])

		// Original slice should not be modified
		assert.Equal(t, 0, basePricing[0].QuotaType)
		assert.Nil(t, basePricing[0].UserGroupPrices)
	})

	t.Run("rule for un-enabled group is ignored", func(t *testing.T) {
		setting := dto.UserSetting{
			UserModelPriceRules: []dto.UserModelPriceRule{
				{
					Group:  "svip", // dall-e-3 only enabled in default
					Models: []string{"dall-e-3"},
					Price:  0.01,
				},
			},
		}
		res := applyUserModelPriceRules(basePricing, setting)
		assert.Len(t, res, 2)
		assert.Nil(t, res[1].UserGroupPrices)
	})

	t.Run("legacy user model prices fallback", func(t *testing.T) {
		setting := dto.UserSetting{
			UserModelPrices: map[string]float64{
				"dall-e-3": 0.02,
			},
		}
		res := applyUserModelPriceRules(basePricing, setting)
		assert.Len(t, res, 2)
		assert.Equal(t, 0.02, res[1].UserGroupPrices["default"])
	})
}

func TestGetUserGroupRatioWithSetting(t *testing.T) {
	setting := dto.UserSetting{
		UserGroupRatios: map[string]float64{
			"custom-group": 0.35,
		},
	}

	ratio := service.GetUserGroupRatioWithSetting(setting, "default", "custom-group")
	assert.Equal(t, 0.35, ratio)
}

func TestFilterPricingByUsableGroups(t *testing.T) {
	basePricing := []model.Pricing{
		{
			ModelName:   "gemini-3-pro-image-preview-c",
			QuotaType:   1,
			ModelPrice:  0.07,
			EnableGroup: []string{"小香蕉对接组", "生成图片组", "大香蕉对接组"},
		},
		{
			ModelName:   "gemini-secret-model",
			QuotaType:   1,
			ModelPrice:  0.05,
			EnableGroup: []string{"小香蕉对接组"},
		},
		{
			ModelName:   "gemini-all-model",
			QuotaType:   0,
			ModelRatio:  1.0,
			EnableGroup: []string{"all", "小香蕉对接组"},
		},
	}

	t.Run("empty pricing returns empty", func(t *testing.T) {
		res := filterPricingByUsableGroups(nil, map[string]string{"生成图片组": ""})
		assert.Empty(t, res)
	})

	t.Run("empty usableGroup returns empty", func(t *testing.T) {
		res := filterPricingByUsableGroups(basePricing, nil)
		assert.Empty(t, res)
	})

	t.Run("strips unauthorized groups from EnableGroup and excludes inaccessible models", func(t *testing.T) {
		userUsable := map[string]string{
			"生成图片组":  "",
			"大香蕉对接组": "",
		}

		res := filterPricingByUsableGroups(basePricing, userUsable)
		// gemini-3-pro-image-preview-c and gemini-all-model should be retained, gemini-secret-model dropped
		assert.Len(t, res, 2)

		// Check first model: only allowed groups remain
		assert.Equal(t, "gemini-3-pro-image-preview-c", res[0].ModelName)
		assert.Equal(t, []string{"生成图片组", "大香蕉对接组"}, res[0].EnableGroup)
		assert.NotContains(t, res[0].EnableGroup, "小香蕉对接组")

		// Check all model: all is preserved, but unauthorized group is removed
		assert.Equal(t, "gemini-all-model", res[1].ModelName)
		assert.Equal(t, []string{"all"}, res[1].EnableGroup)
		assert.NotContains(t, res[1].EnableGroup, "小香蕉对接组")

		// Ensure original cache object was not mutated
		assert.Equal(t, []string{"小香蕉对接组", "生成图片组", "大香蕉对接组"}, basePricing[0].EnableGroup)
	})
}

func TestFilterActiveGroups(t *testing.T) {
	testGroups := []perfmetrics.GroupResult{
		{Group: "svip"},
		{Group: "vip"},
		{Group: "auto"},
	}

	t.Run("non-admin filters out groups not in usableGroup", func(t *testing.T) {
		usable := map[string]string{
			"vip": "",
		}
		res := filterActiveGroups(testGroups, usable, false)
		assert.Len(t, res, 2)
		assert.Equal(t, "vip", res[0].Group)
		assert.Equal(t, "auto", res[1].Group)
	})

	t.Run("admin sees all active groups regardless of usableGroup", func(t *testing.T) {
		usable := map[string]string{
			"vip": "",
		}
		res := filterActiveGroups(testGroups, usable, true)
		assert.Len(t, res, 3)
		assert.Equal(t, "svip", res[0].Group)
		assert.Equal(t, "vip", res[1].Group)
		assert.Equal(t, "auto", res[2].Group)
	})
}

func TestPublicGroupUsableInUserGroupRatios(t *testing.T) {
	// 验证用户可用分组判断逻辑：公共分组与专属分组均可通过校验
	userGroup := "default"
	// default 属于用户本身分组
	assert.True(t, service.GroupInUserUsableGroups(userGroup, "default"))

	// 模拟请求的自定义倍率映射，验证可用分组保留，不可用分组移除
	requestRatios := map[string]float64{
		"default":       0.8,
		"unusable-group": 1.5,
	}
	for group := range requestRatios {
		if !service.GroupInUserUsableGroups(userGroup, group) {
			delete(requestRatios, group)
		}
	}
	assert.Contains(t, requestRatios, "default")
	assert.NotContains(t, requestRatios, "unusable-group")
}


