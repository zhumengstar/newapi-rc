package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
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
