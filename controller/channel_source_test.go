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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSourceURL(t *testing.T) {
	key1, display1 := normalizeSourceURL("https://api.openai-sb.com/")
	require.Equal(t, "https://api.openai-sb.com", key1)
	require.Equal(t, "https://api.openai-sb.com/", display1)

	key2, display2 := normalizeSourceURL("  https://api.closeai.asia/v1/  ")
	require.Equal(t, "https://api.closeai.asia/v1", key2)
	require.Equal(t, "https://api.closeai.asia/v1/", display2)

	key3, display3 := normalizeSourceURL("")
	require.Equal(t, "default", key3)
	require.Equal(t, "官方直连 / 默认地址", display3)
}

func TestUpdateUpstreamSourceRequestJSON(t *testing.T) {
	raw := `{
		"base_url": "https://api.openai-sb.com",
		"wechat_name": "老张-总代理",
		"channel_ratio": 0.25,
		"group": "vip,svip",
		"cost_tier": 2,
		"model_prices": {
			"midjourney": 0.1,
			"gpt-4o": 0.005
		}
	}`

	var req UpdateUpstreamSourceRequest
	err := json.Unmarshal([]byte(raw), &req)
	require.NoError(t, err)

	require.Equal(t, "https://api.openai-sb.com", req.BaseURL)
	require.NotNil(t, req.WechatName)
	require.Equal(t, "老张-总代理", *req.WechatName)
	require.NotNil(t, req.ChannelRatio)
	require.Equal(t, 0.25, *req.ChannelRatio)
	require.NotNil(t, req.Group)
	require.Equal(t, "vip,svip", *req.Group)
	require.NotNil(t, req.CostTier)
	require.Equal(t, 2, *req.CostTier)
	require.Equal(t, 0.1, req.ModelPrices["midjourney"])
	require.Equal(t, 0.005, req.ModelPrices["gpt-4o"])
}

func TestNormalizeSourceModelCostRules(t *testing.T) {
	raw := []SourceModelCostRule{
		{
			Group:  "  cc反重力按次  ",
			Models: []string{"claude-opus-4-6-thinking-c", "claude-opus-4-6-c", "claude-opus-4-6-c"},
			Mode:   "per_call",
			Price:  0.08,
		},
		{
			Group:       "",
			Models:      []string{"gemini-4-argon"},
			Mode:        "per_token",
			InputPrice:  0.4,
			OutputPrice: 1.6,
		},
	}

	cleaned := normalizeSourceModelCostRules(raw)
	require.Len(t, cleaned, 2)
	require.Equal(t, "cc反重力按次", cleaned[0].Group)
	require.Equal(t, []string{"claude-opus-4-6-c", "claude-opus-4-6-thinking-c"}, cleaned[0].Models)
	require.Equal(t, "per_call", cleaned[0].Mode)
	require.Equal(t, 0.08, cleaned[0].Price)

	require.Equal(t, "*", cleaned[1].Group)
	require.Equal(t, "per_token", cleaned[1].Mode)
	require.Equal(t, 0.4, cleaned[1].InputPrice)
	require.Equal(t, 1.6, cleaned[1].OutputPrice)
}

func TestChannelTodayIncomeResponseJSON(t *testing.T) {
	resp := ChannelTodayIncomeResponse{
		Incomes: map[int]ChannelIncomeItem{
			101: {ChannelId: 101, TodayQuota: 1000000, TodayIncome: 2.0},
		},
		TotalQuota:     1000000,
		TotalIncome:    2.0,
		NextUpdateAt:   1700000000,
		IntervalMinute: 30,
	}

	bytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded ChannelTodayIncomeResponse
	err = json.Unmarshal(bytes, &decoded)
	require.NoError(t, err)
	require.Equal(t, 2.0, decoded.TotalIncome)
	require.Equal(t, int64(1000000), decoded.TotalQuota)
	require.Equal(t, 30, decoded.IntervalMinute)
	require.Equal(t, 2.0, decoded.Incomes[101].TodayIncome)
}
