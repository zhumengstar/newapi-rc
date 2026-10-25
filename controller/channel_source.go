/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type GroupIncomeItem struct {
	Group       string  `json:"group"`
	TodayQuota  int64   `json:"today_quota"`
	TodayIncome float64 `json:"today_income"`
}

type UpstreamSourceDTO struct {
	BaseURL             string                     `json:"base_url"`
	DisplayBaseURL      string                     `json:"display_base_url"`
	WechatName          string                     `json:"wechat_name"`
	ChannelCount        int                        `json:"channel_count"`
	EnabledChannelCount int                        `json:"enabled_channel_count"`
	ChannelIds          []int                      `json:"channel_ids"`
	ChannelNames        []string                   `json:"channel_names"`
	Groups              []string                   `json:"groups"`
	ChannelRatio        float64                    `json:"channel_ratio"`
	CostTier            int                        `json:"cost_tier"`
	Models              []string                   `json:"models"`
	ModelPrices         map[string]float64         `json:"model_prices"`
	TodayQuota          int64                      `json:"today_quota"`
	TodayIncome         float64                    `json:"today_income"`
	GroupIncomes        map[string]GroupIncomeItem `json:"group_incomes"`
}

type ChannelIncomeItem struct {
	ChannelId   int     `json:"channel_id"`
	TodayQuota  int64   `json:"today_quota"`
	TodayIncome float64 `json:"today_income"`
}

type ChannelTodayIncomeResponse struct {
	Incomes             map[int]ChannelIncomeItem          `json:"incomes"`
	ChannelGroupIncomes map[int]map[string]GroupIncomeItem `json:"channel_group_incomes,omitempty"`
	TotalQuota          int64                              `json:"total_quota"`
	TotalIncome         float64                            `json:"total_income"`
	NextUpdateAt        int64                              `json:"next_update_at"`
	IntervalMinute      int                                `json:"interval_minutes"`
}

type UpdateUpstreamSourceRequest struct {
	BaseURL      string             `json:"base_url"`
	WechatName   *string            `json:"wechat_name"`
	ChannelRatio *float64           `json:"channel_ratio"`
	Group        *string            `json:"group"`
	CostTier     *int               `json:"cost_tier"`
	ModelPrices  map[string]float64 `json:"model_prices"`
}

func normalizeSourceURL(rawURL string) (cleanKey string, displayURL string) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "default", "官方直连 / 默认地址"
	}
	cleanKey = strings.TrimRight(strings.ToLower(trimmed), "/")
	return cleanKey, trimmed
}

func GetChannelSources(c *gin.Context) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	globalModelPrices := ratio_setting.GetModelPriceMap()
	incomeDetail, _, _ := model.GetCachedChannelTodayIncomeDetail(false)

	sourceMap := make(map[string]*UpstreamSourceDTO)
	groupSets := make(map[string]map[string]struct{})
	modelSets := make(map[string]map[string]struct{})

	for _, ch := range channels {
		if ch == nil {
			continue
		}
		rawURL := ch.GetBaseURL()
		cleanKey, displayURL := normalizeSourceURL(rawURL)

		src, exists := sourceMap[cleanKey]
		if !exists {
			src = &UpstreamSourceDTO{
				BaseURL:        cleanKey,
				DisplayBaseURL: displayURL,
				ChannelIds:     make([]int, 0),
				ChannelNames:   make([]string, 0),
				ModelPrices:    make(map[string]float64),
			}
			sourceMap[cleanKey] = src
			groupSets[cleanKey] = make(map[string]struct{})
			modelSets[cleanKey] = make(map[string]struct{})
		}

		src.ChannelCount++
		if ch.Status == 1 {
			src.EnabledChannelCount++
		}
		src.ChannelIds = append(src.ChannelIds, ch.Id)
		src.ChannelNames = append(src.ChannelNames, ch.Name)

		if incomeDetail != nil && incomeDetail.ChannelIncomes != nil {
			src.TodayQuota += incomeDetail.ChannelIncomes[ch.Id]
		}

		if src.WechatName == "" && ch.Contact != "" {
			src.WechatName = ch.Contact
		}
		if src.ChannelRatio == 0 && ch.ChannelRatio != nil {
			src.ChannelRatio = *ch.ChannelRatio
		}
		if src.CostTier == 0 && ch.CostTier != 0 {
			src.CostTier = ch.CostTier
		}

		// 收集分组
		for _, g := range strings.Split(ch.Group, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				groupSets[cleanKey][g] = struct{}{}
			}
		}

		// 收集模型
		for _, m := range strings.Split(ch.Models, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				modelSets[cleanKey][m] = struct{}{}
			}
		}

		// 解析 channel settings 中的 model_prices
		if ch.OtherSettings != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(ch.OtherSettings), &parsed); err == nil {
				if mp, ok := parsed["model_prices"].(map[string]any); ok {
					for mName, priceVal := range mp {
						if pFloat, ok := priceVal.(float64); ok && pFloat >= 0 {
							src.ModelPrices[mName] = pFloat
						}
					}
				}
			}
		}
	}

	result := make([]*UpstreamSourceDTO, 0, len(sourceMap))
	for cleanKey, src := range sourceMap {
		// 分组排序
		groups := make([]string, 0, len(groupSets[cleanKey]))
		for g := range groupSets[cleanKey] {
			groups = append(groups, g)
		}
		sort.Strings(groups)
		src.Groups = groups

		// 模型排序
		models := make([]string, 0, len(modelSets[cleanKey]))
		for m := range modelSets[cleanKey] {
			models = append(models, m)
		}
		sort.Strings(models)
		src.Models = models

		// 补充全局模型定价中的单次价格
		for _, m := range models {
			if _, set := src.ModelPrices[m]; !set {
				if gp, ok := globalModelPrices[m]; ok && gp >= 0 {
					src.ModelPrices[m] = gp
				}
			}
		}

		if src.TodayQuota > 0 {
			src.TodayIncome = float64(src.TodayQuota) / common.QuotaPerUnit
		}

		src.GroupIncomes = make(map[string]GroupIncomeItem)
		for _, g := range groups {
			src.GroupIncomes[g] = GroupIncomeItem{
				Group:       g,
				TodayQuota:  0,
				TodayIncome: 0,
			}
		}
		if incomeDetail != nil && incomeDetail.ChannelGroupIncomes != nil {
			for _, chId := range src.ChannelIds {
				if gMap, ok := incomeDetail.ChannelGroupIncomes[chId]; ok {
					for g, q := range gMap {
						item := src.GroupIncomes[g]
						item.Group = g
						item.TodayQuota += q
						item.TodayIncome = float64(item.TodayQuota) / common.QuotaPerUnit
						src.GroupIncomes[g] = item
					}
				}
			}
		}

		result = append(result, src)
	}

	// 按包含的渠道数降序排列
	sort.Slice(result, func(i, j int) bool {
		return result[i].ChannelCount > result[j].ChannelCount
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

func UpdateChannelSource(c *gin.Context) {
	var req UpdateUpstreamSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	cleanKey, _ := normalizeSourceURL(req.BaseURL)

	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var matchedChannelIds []int
	var matchedChannels []*model.Channel
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		chCleanKey, _ := normalizeSourceURL(ch.GetBaseURL())
		if chCleanKey == cleanKey {
			matchedChannelIds = append(matchedChannelIds, ch.Id)
			matchedChannels = append(matchedChannels, ch)
		}
	}

	if len(matchedChannelIds) == 0 {
		common.ApiError(c, fmt.Errorf("未找到地址为 %s 的关联渠道", req.BaseURL))
		return
	}

	updates := make(map[string]any)
	auditData := map[string]any{
		"base_url":    req.BaseURL,
		"channel_ids": matchedChannelIds,
	}

	if req.WechatName != nil {
		trimmedContact := strings.TrimSpace(*req.WechatName)
		updates["contact"] = trimmedContact
		auditData["wechat_name"] = trimmedContact
	}

	if req.ChannelRatio != nil {
		if *req.ChannelRatio < 0 {
			common.ApiError(c, fmt.Errorf("渠道倍率不能为负数"))
			return
		}
		updates["channel_ratio"] = *req.ChannelRatio
		auditData["channel_ratio"] = *req.ChannelRatio
	}

	if req.Group != nil {
		cleanGroup := strings.TrimSpace(*req.Group)
		if cleanGroup != "" {
			updates["group"] = cleanGroup
			auditData["group"] = cleanGroup
		}
	}

	if req.CostTier != nil {
		updates["cost_tier"] = *req.CostTier
		auditData["cost_tier"] = *req.CostTier
	}

	// 1. 更新渠道基础字段
	if len(updates) > 0 {
		err := model.DB.Model(&model.Channel{}).Where("id IN ?", matchedChannelIds).Updates(updates).Error
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}

	// 2. 如果配置了模型单请求价格
	if len(req.ModelPrices) > 0 {
		auditData["model_prices"] = req.ModelPrices

		// 同步到全局 ModelPrice 选项
		for mName, price := range req.ModelPrices {
			if price >= 0 {
				ratio_setting.SetModelPrice(mName, price)
			}
		}
		_ = model.UpdateOption("ModelPrice", ratio_setting.ModelPrice2JSONString())

		// 在匹配渠道的 other_settings 中记录 model_prices
		for _, ch := range matchedChannels {
			settingsMap := make(map[string]any)
			if ch.OtherSettings != "" {
				_ = json.Unmarshal([]byte(ch.OtherSettings), &settingsMap)
			}
			existingPrices, _ := settingsMap["model_prices"].(map[string]any)
			if existingPrices == nil {
				existingPrices = make(map[string]any)
			}
			for mName, price := range req.ModelPrices {
				existingPrices[mName] = price
			}
			settingsMap["model_prices"] = existingPrices
			newSettingsBytes, _ := json.Marshal(settingsMap)
			_ = model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("settings", string(newSettingsBytes)).Error
		}
	}

	// 3. 同步能力 abilities（若修改了分组）
	if req.Group != nil {
		for _, ch := range matchedChannels {
			ch.Group = strings.TrimSpace(*req.Group)
			_ = ch.UpdateAbilities(nil)
		}
	}

	// 4. 刷新缓存与记录审计
	model.InitChannelCache()
	recordManageAudit(c, "channel_source.update", auditData)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "来源成本与联系人已成功更新并同步至对应渠道",
		"data": gin.H{
			"updated_count": len(matchedChannelIds),
			"base_url":      req.BaseURL,
		},
	})
}

// GetChannelTodayIncome 获取所有渠道当天的消耗额度与美元收入（30分钟自动刷新缓存）
func GetChannelTodayIncome(c *gin.Context) {
	force, _ := strconv.ParseBool(c.DefaultQuery("force", "false"))
	detail, exp, err := model.GetCachedChannelTodayIncomeDetail(force)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	incomes := make(map[int]ChannelIncomeItem, len(detail.ChannelIncomes))
	channelGroupIncomes := make(map[int]map[string]GroupIncomeItem, len(detail.ChannelGroupIncomes))
	var totalQuota int64
	for chId, quota := range detail.ChannelIncomes {
		totalQuota += quota
		incomes[chId] = ChannelIncomeItem{
			ChannelId:   chId,
			TodayQuota:  quota,
			TodayIncome: float64(quota) / common.QuotaPerUnit,
		}
	}

	for chId, gMap := range detail.ChannelGroupIncomes {
		channelGroupIncomes[chId] = make(map[string]GroupIncomeItem, len(gMap))
		for g, q := range gMap {
			channelGroupIncomes[chId][g] = GroupIncomeItem{
				Group:       g,
				TodayQuota:  q,
				TodayIncome: float64(q) / common.QuotaPerUnit,
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": ChannelTodayIncomeResponse{
			Incomes:             incomes,
			ChannelGroupIncomes: channelGroupIncomes,
			TotalQuota:          totalQuota,
			TotalIncome:         float64(totalQuota) / common.QuotaPerUnit,
			NextUpdateAt:        exp.Unix(),
			IntervalMinute:      30,
		},
	})
}
