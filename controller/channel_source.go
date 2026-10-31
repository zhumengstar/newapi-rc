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

type SourceModelCostRule struct {
	Group       string   `json:"group"`                  // "*" 表示该地址下所有分组通用，或指定具体分组名
	Models      []string `json:"models"`                 // 模型名称列表
	Mode        string   `json:"mode"`                   // "per_call" (按次 $/次) 或 "per_token" (按量 $/M tokens)
	Price       float64  `json:"price,omitempty"`        // mode == "per_call" 时的单次成本 ($/次)
	InputPrice  float64  `json:"input_price,omitempty"`  // mode == "per_token" 时的输入成本 ($/1M tokens)
	OutputPrice float64  `json:"output_price,omitempty"` // mode == "per_token" 时的输出成本 ($/1M tokens)
}

type SourceCostProfile struct {
	BaseURL          string                `json:"base_url"`
	WechatName       string                `json:"wechat_name,omitempty"`
	DefaultCostRatio float64               `json:"default_cost_ratio,omitempty"`
	CostTier         int                   `json:"cost_tier,omitempty"`
	GroupCostRatios  map[string]float64    `json:"group_cost_ratios,omitempty"`
	ModelCostRules   []SourceModelCostRule `json:"model_cost_rules,omitempty"`
	ModelPrices      map[string]float64    `json:"model_prices,omitempty"`
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
	GroupChannelCounts  map[string]int             `json:"group_channel_counts"`
	GroupModels         map[string][]string        `json:"group_models"`
	GroupCustomerRatios map[string]float64         `json:"group_customer_ratios"`
	ChannelRatio        float64                    `json:"channel_ratio"`
	DefaultCostRatio    float64                    `json:"default_cost_ratio"`
	GroupCostRatios     map[string]float64         `json:"group_cost_ratios"`
	ModelCostRules      []SourceModelCostRule      `json:"model_cost_rules"`
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
	BaseURL                 string                 `json:"base_url"`
	WechatName              *string                `json:"wechat_name"`
	ChannelRatio            *float64               `json:"channel_ratio"`
	DefaultCostRatio        *float64               `json:"default_cost_ratio"`
	GroupCostRatios         map[string]float64     `json:"group_cost_ratios"`
	ModelCostRules          *[]SourceModelCostRule `json:"model_cost_rules"`
	Group                   *string                `json:"group"`
	ApplyGroupToAllChannels bool                   `json:"apply_group_to_all_channels"`
	CostTier                *int                   `json:"cost_tier"`
	ModelPrices             map[string]float64     `json:"model_prices"`
}

const upstreamSourceCostOptionKey = "UpstreamSourceCostConfig"

func loadUpstreamSourceCostProfiles() map[string]SourceCostProfile {
	profiles := make(map[string]SourceCostProfile)
	if model.DB == nil {
		return profiles
	}
	var opt model.Option
	if err := model.DB.Where("key = ?", upstreamSourceCostOptionKey).First(&opt).Error; err == nil && opt.Value != "" {
		_ = json.Unmarshal([]byte(opt.Value), &profiles)
	}
	return profiles
}

func saveUpstreamSourceCostProfiles(profiles map[string]SourceCostProfile) {
	if model.DB == nil {
		return
	}
	if b, err := json.Marshal(profiles); err == nil {
		_ = model.UpdateOption(upstreamSourceCostOptionKey, string(b))
	}
}

func normalizeSourceModelCostRules(rules []SourceModelCostRule) []SourceModelCostRule {
	out := make([]SourceModelCostRule, 0, len(rules))
	for _, r := range rules {
		grp := strings.TrimSpace(r.Group)
		if grp == "" {
			grp = "*"
		}
		mode := strings.TrimSpace(r.Mode)
		if mode != "per_token" {
			mode = "per_call"
		}
		seen := make(map[string]struct{}, len(r.Models))
		models := make([]string, 0, len(r.Models))
		for _, m := range r.Models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			if _, ok := seen[m]; !ok {
				seen[m] = struct{}{}
				models = append(models, m)
			}
		}
		if len(models) == 0 {
			continue
		}
		sort.Strings(models)
		cleanRule := SourceModelCostRule{
			Group:  grp,
			Models: models,
			Mode:   mode,
		}
		if mode == "per_token" {
			if r.InputPrice < 0 && r.OutputPrice < 0 {
				continue
			}
			if r.InputPrice >= 0 {
				cleanRule.InputPrice = r.InputPrice
			}
			if r.OutputPrice >= 0 {
				cleanRule.OutputPrice = r.OutputPrice
			}
		} else {
			if r.Price < 0 {
				continue
			}
			cleanRule.Price = r.Price
		}
		out = append(out, cleanRule)
	}
	return out
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

	customerGroupRatios := ratio_setting.GetGroupRatioCopy()
	savedProfiles := loadUpstreamSourceCostProfiles()
	incomeDetail, _, _ := model.GetCachedChannelTodayIncomeDetail(false)

	sourceMap := make(map[string]*UpstreamSourceDTO)
	groupSets := make(map[string]map[string]struct{})
	modelSets := make(map[string]map[string]struct{})
	groupModelSets := make(map[string]map[string]map[string]struct{})

	for _, ch := range channels {
		if ch == nil {
			continue
		}
		rawURL := ch.GetBaseURL()
		cleanKey, displayURL := normalizeSourceURL(rawURL)

		src, exists := sourceMap[cleanKey]
		if !exists {
			src = &UpstreamSourceDTO{
				BaseURL:             cleanKey,
				DisplayBaseURL:      displayURL,
				ChannelIds:          make([]int, 0),
				ChannelNames:        make([]string, 0),
				GroupChannelCounts:  make(map[string]int),
				GroupModels:         make(map[string][]string),
				GroupCustomerRatios: make(map[string]float64),
				GroupCostRatios:     make(map[string]float64),
				ModelCostRules:      make([]SourceModelCostRule, 0),
				ModelPrices:         make(map[string]float64),
			}
			sourceMap[cleanKey] = src
			groupSets[cleanKey] = make(map[string]struct{})
			modelSets[cleanKey] = make(map[string]struct{})
			groupModelSets[cleanKey] = make(map[string]map[string]struct{})
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
		if src.ChannelRatio == 0 && ch.ChannelRatio != nil && *ch.ChannelRatio > 0 {
			src.ChannelRatio = *ch.ChannelRatio
			src.DefaultCostRatio = *ch.ChannelRatio
		}
		if src.CostTier == 0 && ch.CostTier != 0 {
			src.CostTier = ch.CostTier
		}

		// 收集渠道分组与模型
		chGroups := make([]string, 0)
		for _, g := range strings.Split(ch.Group, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				groupSets[cleanKey][g] = struct{}{}
				src.GroupChannelCounts[g]++
				chGroups = append(chGroups, g)
				if groupModelSets[cleanKey][g] == nil {
					groupModelSets[cleanKey][g] = make(map[string]struct{})
				}
			}
		}

		for _, m := range strings.Split(ch.Models, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				modelSets[cleanKey][m] = struct{}{}
				for _, g := range chGroups {
					groupModelSets[cleanKey][g][m] = struct{}{}
				}
			}
		}

		// 若单分组渠道本身设置了 channel_ratio，作为该分组成本倍率的初始回退值
		if len(chGroups) == 1 && ch.ChannelRatio != nil && *ch.ChannelRatio > 0 {
			if _, exists := src.GroupCostRatios[chGroups[0]]; !exists {
				src.GroupCostRatios[chGroups[0]] = *ch.ChannelRatio
			}
		}

		// 解析 channel settings 中的 source_cost 或旧版 model_prices
		if ch.OtherSettings != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(ch.OtherSettings), &parsed); err == nil {
				if scRaw, ok := parsed["source_cost"]; ok {
					if scBytes, err := json.Marshal(scRaw); err == nil {
						var sc SourceCostProfile
						if json.Unmarshal(scBytes, &sc) == nil {
							for g, r := range sc.GroupCostRatios {
								if r >= 0 {
									src.GroupCostRatios[g] = r
								}
							}
							if len(src.ModelCostRules) == 0 && len(sc.ModelCostRules) > 0 {
								src.ModelCostRules = sc.ModelCostRules
							}
						}
					}
				}
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
		// 合并 Option 中持久化的 SourceCostProfile（最高优先级）
		if saved, ok := savedProfiles[cleanKey]; ok {
			if saved.WechatName != "" {
				src.WechatName = saved.WechatName
			}
			if saved.DefaultCostRatio > 0 {
				src.DefaultCostRatio = saved.DefaultCostRatio
				src.ChannelRatio = saved.DefaultCostRatio
			}
			if saved.CostTier > 0 {
				src.CostTier = saved.CostTier
			}
			for g, r := range saved.GroupCostRatios {
				if r >= 0 {
					src.GroupCostRatios[g] = r
				}
			}
			if len(saved.ModelCostRules) > 0 {
				src.ModelCostRules = saved.ModelCostRules
			}
			for m, p := range saved.ModelPrices {
				if p >= 0 {
					src.ModelPrices[m] = p
				}
			}
		}

		// 如果还没有结构化 ModelCostRules 但有旧版 ModelPrices，自动转换为 per_call 规则
		if len(src.ModelCostRules) == 0 && len(src.ModelPrices) > 0 {
			byPrice := make(map[float64][]string)
			for m, p := range src.ModelPrices {
				if p >= 0 {
					byPrice[p] = append(byPrice[p], m)
				}
			}
			for p, ms := range byPrice {
				sort.Strings(ms)
				src.ModelCostRules = append(src.ModelCostRules, SourceModelCostRule{
					Group:  "*",
					Models: ms,
					Mode:   "per_call",
					Price:  p,
				})
			}
			sort.Slice(src.ModelCostRules, func(i, j int) bool {
				return src.ModelCostRules[i].Price < src.ModelCostRules[j].Price
			})
		}

		// 同步 per_call 规则到 ModelPrices 供兼容展示
		if len(src.ModelCostRules) > 0 {
			cleanMap := make(map[string]float64)
			for _, rule := range src.ModelCostRules {
				if rule.Mode == "per_call" && rule.Price >= 0 {
					for _, m := range rule.Models {
						cleanMap[m] = rule.Price
					}
				}
			}
			src.ModelPrices = cleanMap
		}

		// 分组排序与分组模型映射
		groups := make([]string, 0, len(groupSets[cleanKey]))
		for g := range groupSets[cleanKey] {
			groups = append(groups, g)
		}
		sort.Strings(groups)
		src.Groups = groups

		for _, g := range groups {
			if cr, ok := customerGroupRatios[g]; ok {
				src.GroupCustomerRatios[g] = cr
			} else {
				src.GroupCustomerRatios[g] = 1.0
			}
			gModels := make([]string, 0, len(groupModelSets[cleanKey][g]))
			for m := range groupModelSets[cleanKey][g] {
				gModels = append(gModels, m)
			}
			sort.Strings(gModels)
			src.GroupModels[g] = gModels
		}

		// 模型排序
		models := make([]string, 0, len(modelSets[cleanKey]))
		for m := range modelSets[cleanKey] {
			models = append(models, m)
		}
		sort.Strings(models)
		src.Models = models

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

	// 按今日收入降序，其次按包含的渠道数降序排列
	sort.Slice(result, func(i, j int) bool {
		if result[i].TodayQuota != result[j].TodayQuota {
			return result[i].TodayQuota > result[j].TodayQuota
		}
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

	savedProfiles := loadUpstreamSourceCostProfiles()
	profile := savedProfiles[cleanKey]
	profile.BaseURL = cleanKey
	if profile.GroupCostRatios == nil {
		profile.GroupCostRatios = make(map[string]float64)
	}

	updates := make(map[string]any)
	auditData := map[string]any{
		"base_url":    req.BaseURL,
		"channel_ids": matchedChannelIds,
	}

	if req.WechatName != nil {
		trimmedContact := strings.TrimSpace(*req.WechatName)
		updates["contact"] = trimmedContact
		profile.WechatName = trimmedContact
		auditData["wechat_name"] = trimmedContact
	}

	effectiveDefaultRatio := req.DefaultCostRatio
	if effectiveDefaultRatio == nil {
		effectiveDefaultRatio = req.ChannelRatio
	}
	if effectiveDefaultRatio != nil {
		if *effectiveDefaultRatio < 0 {
			common.ApiError(c, fmt.Errorf("渠道成本倍率不能为负数"))
			return
		}
		profile.DefaultCostRatio = *effectiveDefaultRatio
		updates["channel_ratio"] = *effectiveDefaultRatio
		auditData["default_cost_ratio"] = *effectiveDefaultRatio
	}

	if req.GroupCostRatios != nil {
		cleanGroupRatios := make(map[string]float64, len(req.GroupCostRatios))
		for g, r := range req.GroupCostRatios {
			g = strings.TrimSpace(g)
			if g == "" {
				continue
			}
			if r < 0 {
				common.ApiError(c, fmt.Errorf("分组 %s 的成本倍率不能为负数", g))
				return
			}
			cleanGroupRatios[g] = r
		}
		profile.GroupCostRatios = cleanGroupRatios
		auditData["group_cost_ratios"] = cleanGroupRatios
	}

	if req.ModelCostRules != nil {
		cleanRules := normalizeSourceModelCostRules(*req.ModelCostRules)
		profile.ModelCostRules = cleanRules
		// 同步 per_call 价格至 profile.ModelPrices（仅用于来源成本展示，绝不覆写全局对客 ModelPrice）
		flatPrices := make(map[string]float64)
		for _, rule := range cleanRules {
			if rule.Mode == "per_call" && rule.Price >= 0 {
				for _, m := range rule.Models {
					flatPrices[m] = rule.Price
				}
			}
		}
		profile.ModelPrices = flatPrices
		auditData["model_cost_rules"] = cleanRules
	} else if req.ModelPrices != nil {
		cleanPrices := make(map[string]float64)
		byPrice := make(map[float64][]string)
		for mName, price := range req.ModelPrices {
			mName = strings.TrimSpace(mName)
			if mName != "" && price >= 0 {
				cleanPrices[mName] = price
				byPrice[price] = append(byPrice[price], mName)
			}
		}
		profile.ModelPrices = cleanPrices
		rules := make([]SourceModelCostRule, 0, len(byPrice))
		for p, ms := range byPrice {
			sort.Strings(ms)
			rules = append(rules, SourceModelCostRule{
				Group:  "*",
				Models: ms,
				Mode:   "per_call",
				Price:  p,
			})
		}
		sort.Slice(rules, func(i, j int) bool { return rules[i].Price < rules[j].Price })
		profile.ModelCostRules = rules
		auditData["model_prices"] = cleanPrices
	}

	if req.Group != nil && req.ApplyGroupToAllChannels {
		cleanGroup := strings.TrimSpace(*req.Group)
		if cleanGroup != "" {
			updates["group"] = cleanGroup
			auditData["group"] = cleanGroup
		}
	}

	if req.CostTier != nil {
		updates["cost_tier"] = *req.CostTier
		profile.CostTier = *req.CostTier
		auditData["cost_tier"] = *req.CostTier
	}

	// 保存来源成本档案到全局 Option
	savedProfiles[cleanKey] = profile
	saveUpstreamSourceCostProfiles(savedProfiles)

	// 1. 更新渠道基础字段
	if len(updates) > 0 {
		err := model.DB.Model(&model.Channel{}).Where("id IN ?", matchedChannelIds).Updates(updates).Error
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}

	// 2. 按渠道所属分组精确同步 channel_ratio 与 settings.source_cost（不污染全局对客 ModelPrice）
	for _, ch := range matchedChannels {
		chUpdates := make(map[string]any)

		// 若该渠道所属分组在 GroupCostRatios 中有配置，优先将该渠道的 channel_ratio 设为对应分组成本倍率
		for _, g := range strings.Split(ch.Group, ",") {
			g = strings.TrimSpace(g)
			if r, ok := profile.GroupCostRatios[g]; ok && r >= 0 {
				chUpdates["channel_ratio"] = r
				break
			}
		}

		settingsMap := make(map[string]any)
		if ch.OtherSettings != "" {
			_ = json.Unmarshal([]byte(ch.OtherSettings), &settingsMap)
		}
		settingsMap["source_cost"] = profile
		if len(profile.ModelPrices) > 0 {
			settingsMap["model_prices"] = profile.ModelPrices
		} else {
			delete(settingsMap, "model_prices")
		}
		if newSettingsBytes, err := json.Marshal(settingsMap); err == nil {
			chUpdates["settings"] = string(newSettingsBytes)
		}

		if len(chUpdates) > 0 {
			_ = model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Updates(chUpdates).Error
		}
	}

	// 3. 同步能力 abilities（仅当显式要求覆盖渠道分组时）
	if req.Group != nil && req.ApplyGroupToAllChannels {
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
		"message": "来源成本配置已保存并同步至对应渠道",
		"data": gin.H{
			"updated_count": len(matchedChannelIds),
			"base_url":      req.BaseURL,
			"profile":       profile,
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
