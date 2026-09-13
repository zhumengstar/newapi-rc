package controller

import (
	"maps"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func applyUserModelPriceRules(pricing []model.Pricing, userSetting dto.UserSetting) []model.Pricing {
	if len(pricing) == 0 || (len(userSetting.UserModelPriceRules) == 0 && len(userSetting.UserModelPrices) == 0) {
		return pricing
	}

	result := append([]model.Pricing(nil), pricing...)
	indexByModel := make(map[string]int, len(result))
	for i := range result {
		indexByModel[result[i].ModelName] = i
	}
	applyPrice := func(modelName, group string, price float64) {
		index, ok := indexByModel[modelName]
		if !ok || price < 0 {
			return
		}
		item := &result[index]
		if group != "" && !common.StringsContains(item.EnableGroup, group) && !common.StringsContains(item.EnableGroup, "all") {
			return
		}
		if item.UserGroupPrices == nil {
			item.UserGroupPrices = make(map[string]float64)
		}
		item.UserGroupPrices[group] = price
		item.QuotaType = 1
		item.ModelRatio = 1
		item.CompletionRatio = 1
	}

	for _, rule := range userSetting.UserModelPriceRules {
		for _, modelName := range rule.Models {
			applyPrice(modelName, rule.Group, rule.Price)
		}
	}
	for modelName, price := range userSetting.UserModelPrices {
		if _, hasRules := indexByModel[modelName]; !hasRules {
			continue
		}
		for _, group := range result[indexByModel[modelName]].EnableGroup {
			applyPrice(modelName, group, price)
		}
	}
	return result
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	maps.Copy(groupRatio, ratio_setting.GetGroupRatioCopy())
	var group string
	userSetting := dto.UserSetting{}
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			userSetting = user.GetSetting()
			for g := range groupRatio {
				groupRatio[g] = service.GetUserGroupRatioWithSetting(userSetting, group, g)
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	pricing = applyUserModelPriceRules(pricing, userSetting)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}
