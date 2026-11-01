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
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

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
	Group            string   `json:"group"`                        // "*" 表示该地址下所有分组通用，或指定具体分组名
	Models           []string `json:"models"`                       // 模型名称列表
	Mode             string   `json:"mode"`                         // "per_call" (按次 $/次), "per_token" (按量 $/M tokens), "ratio" (按模型独立倍率 x)
	Price            float64  `json:"price,omitempty"`              // mode == "per_call" 时的单次成本 ($/次)
	InputPrice       float64  `json:"input_price,omitempty"`        // mode == "per_token" 时的输入成本 ($/1M tokens)
	OutputPrice      float64  `json:"output_price,omitempty"`       // mode == "per_token" 时的输出成本 ($/1M tokens)
	CostRatio        float64  `json:"cost_ratio,omitempty"`         // mode == "ratio" 时的模型独立成本倍率 (x)
	CacheReadRatio   float64  `json:"cache_read_ratio,omitempty"`   // 可选：缓存读取折扣倍率 (如 0.1)
	CacheCreateRatio float64  `json:"cache_create_ratio,omitempty"` // 可选：缓存创建倍率 (如 1.25)
}

type SourceChannelBrief struct {
	Id           int     `json:"id"`
	Name         string  `json:"name"`
	Group        string  `json:"group"`
	Status       int     `json:"status"`
	Priority     int64   `json:"priority"`
	Weight       uint    `json:"weight"`
	ChannelRatio float64 `json:"channel_ratio"`
	TodayQuota   int64   `json:"today_quota"`
	TodayIncome  float64 `json:"today_income"`
}

type SourceCostProfile struct {
	BaseURL           string                `json:"base_url"`
	WechatName        string                `json:"wechat_name,omitempty"`
	DashboardURL      string                `json:"dashboard_url,omitempty"`
	UpstreamUsername  string                `json:"upstream_username,omitempty"`
	UpstreamPassword  string                `json:"upstream_password,omitempty"`
	UpstreamPlatform  string                `json:"upstream_platform,omitempty"` // "newapi" | "sub2api" | "auto"
	CostDiscountScale float64               `json:"cost_discount_scale,omitempty"`
	UpstreamBalance   *float64              `json:"upstream_balance,omitempty"`
	UpstreamUsedQuota *float64              `json:"upstream_used_quota,omitempty"`
	UpstreamUpdatedAt int64                 `json:"upstream_updated_at,omitempty"`
	DefaultCostRatio  float64               `json:"default_cost_ratio,omitempty"`
	CostTier          int                   `json:"cost_tier,omitempty"`
	GroupCostRatios   map[string]float64    `json:"group_cost_ratios,omitempty"`
	ChannelCostRatios map[int]float64       `json:"channel_cost_ratios,omitempty"`
	ModelCostRules    []SourceModelCostRule `json:"model_cost_rules,omitempty"`
	ModelPrices       map[string]float64    `json:"model_prices,omitempty"`
}

type UpstreamSourceDTO struct {
	BaseURL             string                     `json:"base_url"`
	DisplayBaseURL      string                     `json:"display_base_url"`
	DashboardURL        string                     `json:"dashboard_url"`
	UpstreamUsername    string                     `json:"upstream_username"`
	UpstreamPassword    string                     `json:"upstream_password"`
	UpstreamPlatform    string                     `json:"upstream_platform"`
	CostDiscountScale   float64                    `json:"cost_discount_scale"`
	UpstreamBalance     *float64                   `json:"upstream_balance,omitempty"`
	UpstreamUsedQuota   *float64                   `json:"upstream_used_quota,omitempty"`
	UpstreamUpdatedAt   int64                      `json:"upstream_updated_at,omitempty"`
	WechatName          string                     `json:"wechat_name"`
	ChannelCount        int                        `json:"channel_count"`
	EnabledChannelCount int                        `json:"enabled_channel_count"`
	ChannelIds          []int                      `json:"channel_ids"`
	ChannelNames        []string                   `json:"channel_names"`
	Channels            []SourceChannelBrief       `json:"channels"`
	Groups              []string                   `json:"groups"`
	GroupChannelCounts  map[string]int             `json:"group_channel_counts"`
	GroupModels         map[string][]string        `json:"group_models"`
	GroupCustomerRatios map[string]float64         `json:"group_customer_ratios"`
	ChannelRatio        float64                    `json:"channel_ratio"`
	DefaultCostRatio    float64                    `json:"default_cost_ratio"`
	GroupCostRatios     map[string]float64         `json:"group_cost_ratios"`
	ChannelCostRatios   map[int]float64            `json:"channel_cost_ratios"`
	ModelCostRules      []SourceModelCostRule      `json:"model_cost_rules"`
	CostTier            int                        `json:"cost_tier"`
	Models              []string                   `json:"models"`
	ModelPrices         map[string]float64         `json:"model_prices"`
	TodayQuota          int64                      `json:"today_quota"`
	TodayIncome         float64                    `json:"today_income"`
	TodayEstimatedCost  float64                    `json:"today_estimated_cost"`
	TodayGrossProfit    float64                    `json:"today_gross_profit"`
	TodayGrossMargin    float64                    `json:"today_gross_margin"`
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
	DashboardURL            *string                `json:"dashboard_url"`
	UpstreamUsername        *string                `json:"upstream_username"`
	UpstreamPassword        *string                `json:"upstream_password"`
	UpstreamPlatform        *string                `json:"upstream_platform"`
	CostDiscountScale       *float64               `json:"cost_discount_scale"`
	UpstreamBalance         *float64               `json:"upstream_balance"`
	UpstreamUsedQuota       *float64               `json:"upstream_used_quota"`
	ChannelRatio            *float64               `json:"channel_ratio"`
	DefaultCostRatio        *float64               `json:"default_cost_ratio"`
	GroupCostRatios         map[string]float64     `json:"group_cost_ratios"`
	ChannelCostRatios       map[int]float64        `json:"channel_cost_ratios"`
	ChannelStatuses         map[int]int            `json:"channel_statuses"`
	ModelCostRules          *[]SourceModelCostRule `json:"model_cost_rules"`
	Group                   *string                `json:"group"`
	ApplyGroupToAllChannels bool                   `json:"apply_group_to_all_channels"`
	CostTier                *int                   `json:"cost_tier"`
	ModelPrices             map[string]float64     `json:"model_prices"`
}

type ProbeUpstreamSourceRequest struct {
	BaseURL           string   `json:"base_url"`
	DashboardURL      string   `json:"dashboard_url"`
	UpstreamUsername  string   `json:"upstream_username"`
	UpstreamPassword  string   `json:"upstream_password"`
	UpstreamPlatform  string   `json:"upstream_platform"`
	CostDiscountScale *float64 `json:"cost_discount_scale"`
}

const upstreamSourceCostOptionKey = "UpstreamSourceCostConfig"

var knownRelayDashboardMap = map[string]string{
	"https://direct.apiloop.top":             "https://www.apiloop.top",
	"https://direct-vapi.chuangxiangai.asia": "https://vapi.chuangxiangai.asia",
	"https://global-api.mysandbox.vip":       "https://mysandbox.vip",
	"https://cnd-coo-new.pages.dev":          "https://api.code-relay.com",
	"https://fuck.cccapi.top":                "https://cccapi.top",
	"https://api.vakv.cn":                    "https://vakv.cn",
}

func resolveDefaultDashboardURL(cleanKey string, displayURL string, configuredDashboard string) string {
	trimmed := strings.TrimSpace(configuredDashboard)
	if trimmed != "" {
		return strings.TrimRight(trimmed, "/")
	}
	if mapped, ok := knownRelayDashboardMap[cleanKey]; ok {
		return mapped
	}
	if cleanKey == "default" {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(displayURL), "/")
}

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
		if mode != "per_token" && mode != "ratio" {
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
		switch mode {
		case "per_token":
			if r.InputPrice < 0 && r.OutputPrice < 0 {
				continue
			}
			if r.InputPrice >= 0 {
				cleanRule.InputPrice = r.InputPrice
			}
			if r.OutputPrice >= 0 {
				cleanRule.OutputPrice = r.OutputPrice
			}
			if r.CacheReadRatio > 0 {
				cleanRule.CacheReadRatio = r.CacheReadRatio
			}
			if r.CacheCreateRatio > 0 {
				cleanRule.CacheCreateRatio = r.CacheCreateRatio
			}
		case "ratio":
			if r.CostRatio < 0 {
				continue
			}
			cleanRule.CostRatio = r.CostRatio
		default:
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

func estimateGroupCostUSD(groupName string, groupIncomeUSD float64, custRatio float64, costRatio float64, hasCostRatio bool, rules []SourceModelCostRule) float64 {
	if groupIncomeUSD <= 0 {
		return 0
	}
	// Check if there is a per_call rule specifically for this group
	for _, r := range rules {
		if (r.Group == groupName || r.Group == "*") && r.Mode == "per_call" && r.Price > 0 {
			// Customer per-call revenue baseline:
			// For cc反重力按次, customer pays $0.20/call * custRatio; for image groups, ~$0.03-$0.05/call
			if strings.Contains(groupName, "cc") && strings.Contains(groupName, "按次") {
				custCallPrice := 0.20 * math.Max(custRatio, 0.1)
				return groupIncomeUSD * (r.Price / custCallPrice)
			}
		}
	}
	if !hasCostRatio || costRatio < 0 {
		return 0
	}
	if custRatio <= 0 {
		custRatio = 1.0
	}
	// For cc反重力按次 when billed by token upstream (e.g. costRatio = 0.15x while customer pays 1x = $0.20/call)
	if strings.Contains(groupName, "按次") && custRatio >= 0.8 && costRatio < 0.8 {
		return groupIncomeUSD * costRatio
	}
	return groupIncomeUSD * (costRatio / custRatio)
}

func GetChannelSources(c *gin.Context) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	customerGroupRatios := ratio_setting.GetGroupRatioCopy()
	savedProfiles := loadUpstreamSourceCostProfiles()
	incomeDetail := model.GetCachedChannelTodayIncomeDetailFast(3000 * time.Millisecond)

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
				CostDiscountScale:   1.0,
				UpstreamPlatform:    "auto",
				ChannelIds:          make([]int, 0),
				ChannelNames:        make([]string, 0),
				Channels:            make([]SourceChannelBrief, 0),
				GroupChannelCounts:  make(map[string]int),
				GroupModels:         make(map[string][]string),
				GroupCustomerRatios: make(map[string]float64),
				GroupCostRatios:     make(map[string]float64),
				ChannelCostRatios:   make(map[int]float64),
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

		var chTodayQuota int64
		if incomeDetail != nil && incomeDetail.ChannelIncomes != nil {
			chTodayQuota = incomeDetail.ChannelIncomes[ch.Id]
			src.TodayQuota += chTodayQuota
		}

		chRatioVal := 0.0
		if ch.ChannelRatio != nil && *ch.ChannelRatio > 0 {
			chRatioVal = *ch.ChannelRatio
			src.ChannelCostRatios[ch.Id] = chRatioVal
		}
		var chWeight uint
		if ch.Weight != nil {
			chWeight = *ch.Weight
		}
		src.Channels = append(src.Channels, SourceChannelBrief{
			Id:           ch.Id,
			Name:         ch.Name,
			Group:        ch.Group,
			Status:       ch.Status,
			Priority:     ch.GetPriority(),
			Weight:       chWeight,
			ChannelRatio: chRatioVal,
			TodayQuota:   chTodayQuota,
			TodayIncome:  float64(chTodayQuota) / common.QuotaPerUnit,
		})

		if src.WechatName == "" && ch.Contact != "" {
			src.WechatName = ch.Contact
		}
		if src.ChannelRatio == 0 && chRatioVal > 0 {
			src.ChannelRatio = chRatioVal
			src.DefaultCostRatio = chRatioVal
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
		if len(chGroups) == 1 && chRatioVal > 0 {
			if _, exists := src.GroupCostRatios[chGroups[0]]; !exists {
				src.GroupCostRatios[chGroups[0]] = chRatioVal
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
			if saved.DashboardURL != "" {
				src.DashboardURL = saved.DashboardURL
			}
			if saved.UpstreamUsername != "" {
				src.UpstreamUsername = saved.UpstreamUsername
			}
			if saved.UpstreamPassword != "" {
				src.UpstreamPassword = saved.UpstreamPassword
			}
			if saved.UpstreamPlatform != "" {
				src.UpstreamPlatform = saved.UpstreamPlatform
			}
			if saved.CostDiscountScale > 0 {
				src.CostDiscountScale = saved.CostDiscountScale
			}
			if saved.UpstreamBalance != nil {
				src.UpstreamBalance = saved.UpstreamBalance
			}
			if saved.UpstreamUsedQuota != nil {
				src.UpstreamUsedQuota = saved.UpstreamUsedQuota
			}
			if saved.UpstreamUpdatedAt > 0 {
				src.UpstreamUpdatedAt = saved.UpstreamUpdatedAt
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
			for chId, r := range saved.ChannelCostRatios {
				if r >= 0 {
					src.ChannelCostRatios[chId] = r
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

		src.DashboardURL = resolveDefaultDashboardURL(cleanKey, src.DisplayBaseURL, src.DashboardURL)

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

		// 核算该来源今日预计采购总成本与净毛利
		var totalEstCost float64
		var accountedGroupIncome float64
		for g, gItem := range src.GroupIncomes {
			if gItem.TodayIncome <= 0 {
				continue
			}
			accountedGroupIncome += gItem.TodayIncome
			custR := src.GroupCustomerRatios[g]
			if custR <= 0 {
				if cr, ok := customerGroupRatios[g]; ok && cr > 0 {
					custR = cr
				} else {
					custR = 1.0
				}
			}
			costR, hasR := src.GroupCostRatios[g]
			if !hasR && src.DefaultCostRatio > 0 {
				costR = src.DefaultCostRatio
				hasR = true
			}
			totalEstCost += estimateGroupCostUSD(g, gItem.TodayIncome, custR, costR, hasR, src.ModelCostRules)
		}
		// 若 TodayIncome 大于分组合计（例如单分组渠道未落入 GroupIncomes 明细）
		if src.TodayIncome > accountedGroupIncome && (src.TodayIncome-accountedGroupIncome) > 0.001 {
			remIncome := src.TodayIncome - accountedGroupIncome
			if len(src.Groups) == 1 {
				g := src.Groups[0]
				custR := src.GroupCustomerRatios[g]
				costR, hasR := src.GroupCostRatios[g]
				if !hasR && src.DefaultCostRatio > 0 {
					costR = src.DefaultCostRatio
					hasR = true
				}
				totalEstCost += estimateGroupCostUSD(g, remIncome, custR, costR, hasR, src.ModelCostRules)
			} else if src.DefaultCostRatio > 0 {
				totalEstCost += remIncome * src.DefaultCostRatio
			}
		}

		src.TodayEstimatedCost = math.Round(totalEstCost*10000) / 10000
		if src.TodayIncome > 0 && src.TodayEstimatedCost > 0 {
			src.TodayGrossProfit = math.Round((src.TodayIncome-src.TodayEstimatedCost)*10000) / 10000
			src.TodayGrossMargin = math.Round(((src.TodayIncome-src.TodayEstimatedCost)/src.TodayIncome)*1000) / 10
		}

		// 渠道列表按今日收入降序、ID 升序排列
		sort.Slice(src.Channels, func(i, j int) bool {
			if src.Channels[i].TodayQuota != src.Channels[j].TodayQuota {
				return src.Channels[i].TodayQuota > src.Channels[j].TodayQuota
			}
			return src.Channels[i].Id < src.Channels[j].Id
		})

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
	if profile.ChannelCostRatios == nil {
		profile.ChannelCostRatios = make(map[int]float64)
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
	if req.DashboardURL != nil {
		profile.DashboardURL = strings.TrimRight(strings.TrimSpace(*req.DashboardURL), "/")
		auditData["dashboard_url"] = profile.DashboardURL
	}
	if req.UpstreamUsername != nil {
		profile.UpstreamUsername = strings.TrimSpace(*req.UpstreamUsername)
		auditData["upstream_username"] = profile.UpstreamUsername
	}
	if req.UpstreamPassword != nil {
		profile.UpstreamPassword = strings.TrimSpace(*req.UpstreamPassword)
	}
	if req.UpstreamPlatform != nil {
		profile.UpstreamPlatform = strings.TrimSpace(*req.UpstreamPlatform)
	}
	if req.CostDiscountScale != nil && *req.CostDiscountScale > 0 {
		profile.CostDiscountScale = *req.CostDiscountScale
		auditData["cost_discount_scale"] = *req.CostDiscountScale
	}
	if req.UpstreamBalance != nil {
		profile.UpstreamBalance = req.UpstreamBalance
		profile.UpstreamUpdatedAt = time.Now().Unix()
	}
	if req.UpstreamUsedQuota != nil {
		profile.UpstreamUsedQuota = req.UpstreamUsedQuota
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

	if req.ChannelCostRatios != nil {
		cleanChRatios := make(map[int]float64, len(req.ChannelCostRatios))
		for chId, r := range req.ChannelCostRatios {
			if r >= 0 {
				cleanChRatios[chId] = r
			}
		}
		profile.ChannelCostRatios = cleanChRatios
		auditData["channel_cost_ratios"] = cleanChRatios
	}

	if req.ModelCostRules != nil {
		cleanRules := normalizeSourceModelCostRules(*req.ModelCostRules)
		profile.ModelCostRules = cleanRules
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

	// 2. 按渠道所属分组或单渠道覆盖同步 channel_ratio、status 与 settings.source_cost
	for _, ch := range matchedChannels {
		chUpdates := make(map[string]any)

		// 优先使用分组成本倍率
		for _, g := range strings.Split(ch.Group, ",") {
			g = strings.TrimSpace(g)
			if r, ok := profile.GroupCostRatios[g]; ok && r >= 0 {
				chUpdates["channel_ratio"] = r
				break
			}
		}
		// 若显式指定了该渠道 ID 的独立成本倍率，则最高优先级覆盖
		if chRatio, ok := profile.ChannelCostRatios[ch.Id]; ok && chRatio > 0 {
			chUpdates["channel_ratio"] = chRatio
		}
		// 若显式修改了该渠道的启停状态
		if req.ChannelStatuses != nil {
			if nextStatus, ok := req.ChannelStatuses[ch.Id]; ok && (nextStatus == 1 || nextStatus == 2) {
				chUpdates["status"] = nextStatus
			}
		}

		settingsMap := make(map[string]any)
		if ch.OtherSettings != "" {
			_ = json.Unmarshal([]byte(ch.OtherSettings), &settingsMap)
		}
		// 不把明文密码冗余写进每个 channel 的 settings JSON，仅保存在全局 Option
		safeProfile := profile
		safeProfile.UpstreamPassword = ""
		settingsMap["source_cost"] = safeProfile
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

	// 3. 同步能力 abilities
	if (req.Group != nil && req.ApplyGroupToAllChannels) || len(req.ChannelStatuses) > 0 {
		for _, ch := range matchedChannels {
			if req.Group != nil && req.ApplyGroupToAllChannels {
				ch.Group = strings.TrimSpace(*req.Group)
			}
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

// ProbeUpstreamSource 实时登录上游后台查询账户余额，并通过渠道 Token 探测 `/api/log/token` 真实扣费倍率
func ProbeUpstreamSource(c *gin.Context) {
	var req ProbeUpstreamSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	cleanKey, displayURL := normalizeSourceURL(req.BaseURL)
	savedProfiles := loadUpstreamSourceCostProfiles()
	profile := savedProfiles[cleanKey]
	profile.BaseURL = cleanKey

	dashboardURL := strings.TrimSpace(req.DashboardURL)
	if dashboardURL == "" {
		dashboardURL = profile.DashboardURL
	}
	targetConsoleURL := resolveDefaultDashboardURL(cleanKey, displayURL, dashboardURL)

	username := strings.TrimSpace(req.UpstreamUsername)
	if username == "" {
		username = profile.UpstreamUsername
	}
	password := strings.TrimSpace(req.UpstreamPassword)
	if password == "" {
		password = profile.UpstreamPassword
	}
	platform := strings.TrimSpace(req.UpstreamPlatform)
	if platform == "" {
		platform = profile.UpstreamPlatform
	}
	if platform == "" {
		platform = "auto"
	}

	scale := 1.0
	if req.CostDiscountScale != nil && *req.CostDiscountScale > 0 {
		scale = *req.CostDiscountScale
	} else if profile.CostDiscountScale > 0 {
		scale = profile.CostDiscountScale
	}

	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Jar:     jar,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	var detectedBalance *float64
	var detectedUsed *float64
	detectedPlatform := platform
	var loginMessage string

	// Step 1: 尝试通过账号密码登录上游获取实时余额
	if username != "" && password != "" && targetConsoleURL != "" {
		// 1A: Try NewAPI /api/user/login
		if platform == "auto" || platform == "newapi" {
			loginBody, _ := json.Marshal(map[string]string{
				"username": username,
				"password": password,
			})
			loginReq, err := http.NewRequest("POST", targetConsoleURL+"/api/user/login", bytes.NewReader(loginBody))
			if err == nil {
				loginReq.Header.Set("Content-Type", "application/json")
				loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
				if resp, err := httpClient.Do(loginReq); err == nil {
					defer resp.Body.Close()
					bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
					var loginRes struct {
						Success bool   `json:"success"`
						Message string `json:"message"`
						Data    struct {
							Id        int   `json:"id"`
							Quota     int64 `json:"quota"`
							UsedQuota int64 `json:"used_quota"`
						} `json:"data"`
					}
					if json.Unmarshal(bodyBytes, &loginRes) == nil && loginRes.Success && loginRes.Data.Id > 0 {
						detectedPlatform = "newapi"
						bal := math.Round((float64(loginRes.Data.Quota)/500000.0)*10000) / 10000
						used := math.Round((float64(loginRes.Data.UsedQuota)/500000.0)*10000) / 10000
						detectedBalance = &bal
						detectedUsed = &used

						// Fetch /api/user/self for freshest quota
						selfReq, _ := http.NewRequest("GET", targetConsoleURL+"/api/user/self", nil)
						selfReq.Header.Set("New-Api-User", strconv.Itoa(loginRes.Data.Id))
						selfReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
						if selfResp, err := httpClient.Do(selfReq); err == nil {
							defer selfResp.Body.Close()
							sb, _ := io.ReadAll(io.LimitReader(selfResp.Body, 256*1024))
							var selfRes struct {
								Success bool `json:"success"`
								Data    struct {
									Quota     int64 `json:"quota"`
									UsedQuota int64 `json:"used_quota"`
								} `json:"data"`
							}
							if json.Unmarshal(sb, &selfRes) == nil && selfRes.Success {
								bal2 := math.Round((float64(selfRes.Data.Quota)/500000.0)*10000) / 10000
								used2 := math.Round((float64(selfRes.Data.UsedQuota)/500000.0)*10000) / 10000
								detectedBalance = &bal2
								detectedUsed = &used2
							}
						}
						loginMessage = fmt.Sprintf("NewAPI 账户登录成功，当前余额 $%.2f", *detectedBalance)
					}
				}
			}
		}

		// 1B: Try Sub2API /api/v1/auth/login if NewAPI didn't match
		if detectedBalance == nil && (platform == "auto" || platform == "sub2api") {
			loginBody, _ := json.Marshal(map[string]string{
				"email":    username,
				"password": password,
			})
			loginReq, err := http.NewRequest("POST", targetConsoleURL+"/api/v1/auth/login", bytes.NewReader(loginBody))
			if err == nil {
				loginReq.Header.Set("Content-Type", "application/json")
				loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
				if resp, err := httpClient.Do(loginReq); err == nil {
					defer resp.Body.Close()
					bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
					var subRes struct {
						Code    int    `json:"code"`
						Message string `json:"message"`
						Data    struct {
							AccessToken string `json:"access_token"`
							User        struct {
								Balance float64 `json:"balance"`
							} `json:"user"`
						} `json:"data"`
					}
					if json.Unmarshal(bodyBytes, &subRes) == nil && subRes.Code == 0 && subRes.Data.AccessToken != "" {
						detectedPlatform = "sub2api"
						bal := math.Round(subRes.Data.User.Balance*10000) / 10000
						detectedBalance = &bal
						loginMessage = fmt.Sprintf("Sub2API 账户登录成功，当前余额 $%.2f", *detectedBalance)
					}
				}
			}
		}
	}

	// Step 2: 通过该来源下关联渠道的 API Key 穿透探测 `/api/log/token` 真实扣费流水与令牌余额
	channels, _ := model.GetAllChannels(0, 0, true, false)
	detectedGroupRatios := make(map[string]float64)
	detectedModelCallPrices := make(map[string]float64)
	seenKeys := make(map[string]bool)

	candidateOrigins := []string{strings.TrimRight(displayURL, "/")}
	if targetConsoleURL != "" && targetConsoleURL != candidateOrigins[0] {
		candidateOrigins = append(candidateOrigins, targetConsoleURL)
	}

	for _, ch := range channels {
		if ch == nil {
			continue
		}
		chCleanKey, _ := normalizeSourceURL(ch.GetBaseURL())
		if chCleanKey != cleanKey {
			continue
		}
		rawKey := strings.TrimSpace(strings.Split(strings.TrimSpace(ch.Key), "\n")[0])
		rawKey = strings.TrimSpace(strings.Split(rawKey, ",")[0])
		if rawKey == "" || !strings.HasPrefix(rawKey, "sk-") || seenKeys[rawKey] {
			continue
		}
		seenKeys[rawKey] = true

		chGroups := make([]string, 0)
		for _, g := range strings.Split(ch.Group, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				chGroups = append(chGroups, g)
			}
		}

		for _, origin := range candidateOrigins {
			// 2A: 探测 /api/log/token?key=sk-...
			logURL := fmt.Sprintf("%s/api/log/token?key=%s", origin, url.QueryEscape(rawKey))
			reqLog, err := http.NewRequest("GET", logURL, nil)
			if err == nil {
				reqLog.Header.Set("Authorization", "Bearer "+rawKey)
				reqLog.Header.Set("User-Agent", "Mozilla/5.0")
				if resp, err := httpClient.Do(reqLog); err == nil {
					bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
					resp.Body.Close()
					var tokenLogRes struct {
						Success bool `json:"success"`
						Data    []struct {
							Type      int    `json:"type"`
							ModelName string `json:"model_name"`
							Quota     int64  `json:"quota"`
							Other     string `json:"other"`
						} `json:"data"`
					}
					if json.Unmarshal(bodyBytes, &tokenLogRes) == nil && tokenLogRes.Success && len(tokenLogRes.Data) > 0 {
						for _, row := range tokenLogRes.Data {
							if row.Type != 2 && row.Type != 0 {
								continue
							}
							var otherMap map[string]any
							if json.Unmarshal([]byte(row.Other), &otherMap) != nil {
								continue
							}
							gr, _ := otherMap["group_ratio"].(float64)
							ugr, hasUgr := otherMap["user_group_ratio"].(float64)
							mp, _ := otherMap["model_price"].(float64)

							effRatio := gr
							if hasUgr && ugr > 0 {
								if effRatio > 0 {
									effRatio = effRatio * ugr
								} else {
									effRatio = ugr
								}
							}
							if effRatio > 0 {
								scaledRatio := math.Round(effRatio*scale*10000) / 10000
								for _, g := range chGroups {
									if _, exists := detectedGroupRatios[g]; !exists {
										detectedGroupRatios[g] = scaledRatio
									}
								}
							}
							if mp > 0 && row.ModelName != "" {
								detectedModelCallPrices[row.ModelName] = math.Round(mp*scale*10000) / 10000
							}
						}
					}
				}
			}

			// 2B: 若尚未通过账号密码拿到余额，尝试通过 /api/usage/token 或 /v1/dashboard/billing/subscription 获取 Key 额度
			if detectedBalance == nil {
				usageReq, err := http.NewRequest("GET", origin+"/api/usage/token/", nil)
				if err == nil {
					usageReq.Header.Set("Authorization", "Bearer "+rawKey)
					if resp, err := httpClient.Do(usageReq); err == nil {
						b, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
						resp.Body.Close()
						var usageRes struct {
							Code bool `json:"code"`
							Data struct {
								TotalAvailable float64 `json:"total_available"`
								TotalUsed      float64 `json:"total_used"`
								UnlimitedQuota bool    `json:"unlimited_quota"`
							} `json:"data"`
						}
						if json.Unmarshal(b, &usageRes) == nil && usageRes.Code && !usageRes.Data.UnlimitedQuota && usageRes.Data.TotalAvailable > 0 {
							bal := math.Round((usageRes.Data.TotalAvailable/500000.0)*10000) / 10000
							used := math.Round((usageRes.Data.TotalUsed/500000.0)*10000) / 10000
							detectedBalance = &bal
							detectedUsed = &used
							loginMessage = fmt.Sprintf("通过令牌接口探测到可用余额 $%.2f", bal)
						}
					}
				}
			}
		}
	}

	// Step 3: 持久化探测到的元数据与余额
	if dashboardURL != "" {
		profile.DashboardURL = strings.TrimRight(dashboardURL, "/")
	}
	if username != "" {
		profile.UpstreamUsername = username
	}
	if password != "" {
		profile.UpstreamPassword = password
	}
	if detectedPlatform != "" && detectedPlatform != "auto" {
		profile.UpstreamPlatform = detectedPlatform
	}
	if scale > 0 {
		profile.CostDiscountScale = scale
	}
	if detectedBalance != nil {
		profile.UpstreamBalance = detectedBalance
		profile.UpstreamUpdatedAt = time.Now().Unix()
	}
	if detectedUsed != nil {
		profile.UpstreamUsedQuota = detectedUsed
	}
	savedProfiles[cleanKey] = profile
	saveUpstreamSourceCostProfiles(savedProfiles)

	if loginMessage == "" {
		if len(detectedGroupRatios) > 0 || len(detectedModelCallPrices) > 0 {
			loginMessage = fmt.Sprintf("成功从上游令牌日志探测到 %d 个分组真实倍率", len(detectedGroupRatios))
		} else {
			loginMessage = "已完成上游探测（如站点开启 Cloudflare 验证或需登录凭证，请检查账号密码）"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": loginMessage,
		"data": gin.H{
			"base_url":                   cleanKey,
			"dashboard_url":              targetConsoleURL,
			"platform":                   detectedPlatform,
			"upstream_balance":           profile.UpstreamBalance,
			"upstream_used_quota":        profile.UpstreamUsedQuota,
			"upstream_updated_at":        profile.UpstreamUpdatedAt,
			"detected_group_ratios":      detectedGroupRatios,
			"detected_model_call_prices": detectedModelCallPrices,
			"cost_discount_scale":        scale,
		},
	})
}

// ServeUpstreamAutoLoginUserscript 提供油猴 (Tampermonkey / Violentmonkey) 免密一键自动登录脚本
func ServeUpstreamAutoLoginUserscript(c *gin.Context) {
	script := `// ==UserScript==
// @name         Muling 上游中转站一键免密登录助手 (SSO Bridge)
// @namespace    https://vip.muling.store/
// @version      1.2.0
// @description  在 vip.muling.store 点击上游域名时，自动完成 NewAPI / OneAPI / Sub2API 站点免密登录并直达控制台
// @author       Muling Admin
// @match        *://*/*
// @run-at       document-start
// @grant        none
// ==/UserScript==

(function () {
  'use strict';

  const STORAGE_KEY = '__muling_sso_payload';

  // 1. 在 document-start 阶段捕获 URL hash 中的 #muling_sso=...，防止被 SPA 路由重定向抹除
  try {
    const hash = window.location.hash || '';
    const match = hash.match(/[#&]muling_sso=([A-Za-z0-9+/=%_-]+)/);
    if (match && match[1]) {
      const rawB64 = decodeURIComponent(match[1]).replace(/-/g, '+').replace(/_/g, '/');
      const jsonStr = decodeURIComponent(escape(window.atob(rawB64)));
      const payload = JSON.parse(jsonStr);
      if (payload && payload.u && payload.p) {
        sessionStorage.setItem(STORAGE_KEY, JSON.stringify(payload));
        const cleanHash = hash.replace(/[#&]?muling_sso=[A-Za-z0-9+/=%_-]+/, '');
        history.replaceState(null, '', window.location.pathname + window.location.search + (cleanHash && cleanHash !== '#' ? cleanHash : ''));
      }
    }
  } catch (err) {
    console.warn('[Muling SSO] Failed to parse hash payload:', err);
  }

  const savedRaw = sessionStorage.getItem(STORAGE_KEY);
  if (!savedRaw) return;

  let payload = null;
  try {
    payload = JSON.parse(savedRaw);
  } catch {
    sessionStorage.removeItem(STORAGE_KEY);
    return;
  }

  if (!payload || !payload.u || !payload.p || (payload.ts && Date.now() - payload.ts > 120000)) {
    sessionStorage.removeItem(STORAGE_KEY);
    return;
  }

  function showBanner(msg, bg) {
    let el = document.getElementById('__muling_sso_banner');
    if (!el) {
      el = document.createElement('div');
      el.id = '__muling_sso_banner';
      el.style.cssText = 'position:fixed;top:12px;right:12px;z-index:2147483647;padding:10px 16px;border-radius:8px;color:#fff;font-size:13px;font-weight:600;font-family:system-ui,-apple-system,sans-serif;box-shadow:0 8px 24px rgba(0,0,0,0.25);transition:all 0.25s ease;';
      (document.body || document.documentElement).appendChild(el);
    }
    el.style.background = bg || 'linear-gradient(135deg, #2563eb, #4f46e5)';
    el.textContent = msg;
  }

  function setNativeInputValue(input, val) {
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')?.set;
    if (setter) {
      setter.call(input, val);
    } else {
      input.value = val;
    }
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.dispatchEvent(new Event('change', { bubbles: true }));
  }

  async function executeAutoLogin() {
    showBanner('🚀 正在免密登录上游控制台 (' + payload.u + ') ...');

    const u = payload.u;
    const p = payload.p;
    const platform = payload.platform || 'auto';

    // 方案 A: 直接调用同源登录 API 写入登录凭证与 LocalStorage
    if (platform === 'auto' || platform === 'newapi') {
      try {
        const res = await fetch('/api/user/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({ username: u, password: p })
        });
        const data = await res.json();
        if (data && data.success && data.data) {
          localStorage.setItem('user', JSON.stringify(data.data));
          sessionStorage.removeItem(STORAGE_KEY);
          showBanner('✅ NewAPI 登录成功！正在进入控制台...', 'linear-gradient(135deg, #059669, #10b981)');
          setTimeout(() => {
            window.location.replace('/console');
          }, 150);
          return;
        }
      } catch (e) {}
    }

    if (platform === 'auto' || platform === 'sub2api') {
      try {
        const res = await fetch('/api/v1/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({ email: u, password: p })
        });
        const data = await res.json();
        if (data && data.code === 0 && data.data && data.data.access_token) {
          localStorage.setItem('auth_token', data.data.access_token);
          if (data.data.user) {
            localStorage.setItem('auth_user', JSON.stringify(data.data.user));
          }
          sessionStorage.removeItem(STORAGE_KEY);
          showBanner('✅ Sub2API 登录成功！正在进入后台...', 'linear-gradient(135deg, #059669, #10b981)');
          setTimeout(() => {
            window.location.replace('/dashboard');
          }, 150);
          return;
        }
      } catch (e) {}
    }

    // 方案 B: 若接口需要验证码或页面表单交互，自动填充登录输入框并点击登录按钮
    let attempts = 0;
    const timer = setInterval(() => {
      attempts++;
      const pwdInput = document.querySelector('input[type="password"]');
      const userInput = document.querySelector('input[name="username"], input[name="email"], input[type="email"], input[type="text"]');
      if (pwdInput && userInput) {
        clearInterval(timer);
        sessionStorage.removeItem(STORAGE_KEY);
        setNativeInputValue(userInput, u);
        setNativeInputValue(pwdInput, p);
        showBanner('✨ 已自动填入账号密码，正在提交登录...', 'linear-gradient(135deg, #d97706, #f59e0b)');
        setTimeout(() => {
          const submitBtn = pwdInput.closest('form')?.querySelector('button[type="submit"], button') ||
            Array.from(document.querySelectorAll('button')).find(b => /登录|Sign in|Login/i.test(b.textContent || ''));
          if (submitBtn) submitBtn.click();
        }, 200);
      } else if (attempts > 25) {
        clearInterval(timer);
        sessionStorage.removeItem(STORAGE_KEY);
      }
    }, 200);
  }

  if (document.readyState === 'loading') {
    window.addEventListener('DOMContentLoaded', executeAutoLogin, { once: true });
  } else {
    executeAutoLogin();
  }
})();
`
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.String(http.StatusOK, script)
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
