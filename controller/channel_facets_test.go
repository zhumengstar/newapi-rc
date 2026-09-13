package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestChannelModelFamily(t *testing.T) {
	assert.Equal(t, "OpenAI", getChannelModelFamily("gpt-4o"))
	assert.Equal(t, "OpenAI", getChannelModelFamily("o1-preview"))
	assert.Equal(t, "OpenAI", getChannelModelFamily("o3-mini"))
	assert.Equal(t, "OpenAI", getChannelModelFamily("dall-e-3"))
	assert.Equal(t, "Claude", getChannelModelFamily("claude-sonnet-4-6"))
	assert.Equal(t, "Gemini", getChannelModelFamily("gemini-2.5-pro"))
	assert.Equal(t, "Gemini", getChannelModelFamily("nano-banana-pro-c"))
	assert.Equal(t, "Grok", getChannelModelFamily("grok-composer-2.5-fast"))
	assert.Equal(t, "DeepSeek", getChannelModelFamily("deepseek-r1"))
	assert.Equal(t, "Other", getChannelModelFamily("my-custom-model"))
}

func TestChannelModelType(t *testing.T) {
	assert.Equal(t, channelModelTypeText, getChannelModelType("gpt-3.5-turbo"))
	assert.Equal(t, channelModelTypeText, getChannelModelType("deepseek-r1"))
	assert.Equal(t, channelModelTypeMultimodal, getChannelModelType("gpt-4o"))
	assert.Equal(t, channelModelTypeMultimodal, getChannelModelType("claude-3-5-sonnet"))
	assert.Equal(t, channelModelTypeMultimodal, getChannelModelType("gemini-2.0-flash"))
	assert.Equal(t, channelModelTypeMultimodal, getChannelModelType("qwen-vl-max"))
	assert.Equal(t, channelModelTypeImage, getChannelModelType("gpt-image-2"))
	assert.Equal(t, channelModelTypeImage, getChannelModelType("dall-e-3"))
	assert.Equal(t, channelModelTypeImage, getChannelModelType("sd-2.0-720-900"))
	assert.Equal(t, channelModelTypeVideo, getChannelModelType("seedance-video"))
	assert.Equal(t, channelModelTypeVideo, getChannelModelType("veo-2"))
	assert.Equal(t, channelModelTypeVideo, getChannelModelType("kling-v1"))
}

func TestChannelBillingType(t *testing.T) {
	pricing := map[string]model.Pricing{
		"gpt-4o": {
			ModelName: "gpt-4o",
			QuotaType: 0,
		},
		"dall-e-3": {
			ModelName: "dall-e-3",
			QuotaType: 1,
		},
	}

	assert.Equal(t, channelBillingTypePerToken, getChannelBillingType("gpt-4o", pricing))
	assert.Equal(t, channelBillingTypePerRequest, getChannelBillingType("dall-e-3", pricing))
	assert.Equal(t, channelBillingTypePerToken, getChannelBillingType("unknown-model", pricing))
}

func TestChannelFilterHelpers(t *testing.T) {
	channels := []*model.Channel{
		{
			Id:     1,
			Models: "gpt-4o,gpt-image-2",
		},
		{
			Id:     2,
			Models: "claude-sonnet-4-6",
		},
		{
			Id:     3,
			Models: "gemini-2.5-pro,veo-2",
		},
	}

	pricing := map[string]model.Pricing{
		"gpt-image-2": {ModelName: "gpt-image-2", QuotaType: 1},
	}

	t.Run("filter by family", func(t *testing.T) {
		openAIChannels := filterChannelsByModelFamily(channels, "OpenAI")
		assert.Len(t, openAIChannels, 1)
		assert.Equal(t, 1, openAIChannels[0].Id)

		allChannels := filterChannelsByModelFamily(channels, "all")
		assert.Len(t, allChannels, 3)
	})

	t.Run("filter by model type", func(t *testing.T) {
		videoChannels := filterChannelsByModelType(channels, channelModelTypeVideo)
		assert.Len(t, videoChannels, 1)
		assert.Equal(t, 3, videoChannels[0].Id)
	})

	t.Run("count families", func(t *testing.T) {
		counts := countChannelModelFamilies(channels)
		assert.Equal(t, int64(3), counts["all"])
		assert.Equal(t, int64(1), counts["OpenAI"])
		assert.Equal(t, int64(1), counts["Claude"])
		assert.Equal(t, int64(1), counts["Gemini"])
	})

	t.Run("filter by billing type", func(t *testing.T) {
		perReq := make([]*model.Channel, 0)
		for _, ch := range channels {
			if _, ok := getChannelBillingTypes(ch.Models, pricing)[channelBillingTypePerRequest]; ok {
				perReq = append(perReq, ch)
			}
		}
		assert.Len(t, perReq, 1)
		assert.Equal(t, 1, perReq[0].Id)
	})
}
