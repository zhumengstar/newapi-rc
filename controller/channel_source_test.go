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
