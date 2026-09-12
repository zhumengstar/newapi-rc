package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeAdaptiveWeightsKeepsTotalAtConfiguredLimit(t *testing.T) {
	candidates := []adaptiveCandidate{
		{MinWeight: 900, MaxWeight: 1000},
		{MinWeight: 900, MaxWeight: 1000},
	}

	weights := normalizeAdaptiveWeights([]float64{1, 0.5}, candidates, adaptiveRoutingTotalWeight)

	assert.Equal(t, []uint{150, 150}, weights)
	assert.Equal(t, uint(300), sumAdaptiveWeights(weights))
}

func TestAdaptiveCandidateScorePenalizesSlowChannel(t *testing.T) {
	candidate := adaptiveCandidate{
		SlowThresholdMs: 4000,
		Metric: adaptiveMetric{
			Requests:        10,
			Successes:       10,
			TTFTSumMs:       50000,
			TTFTCount:       10,
			ModelPriceSum:   1,
			ModelPriceCount: 10,
		},
	}

	score := adaptiveCandidateScore(candidate, 0.1)

	assert.LessOrEqual(t, score, 0.01)
}

func TestAdaptiveImageChannelsUseTotalDuration(t *testing.T) {
	candidate := adaptiveCandidate{
		UseTotalTime:    true,
		SlowThresholdMs: 4000,
		Metric: adaptiveMetric{
			Requests:   10,
			Successes:  10,
			TTFTSumMs:  100,
			TTFTCount:  10,
			TotalMs:    60000,
			TotalCount: 10,
		},
	}

	assert.LessOrEqual(t, adaptiveCandidateScore(candidate, 0), 0.01)
}

func TestAdaptiveImageRequestDetectedFromRequestPath(t *testing.T) {
	assert.True(t, isAdaptiveImageRequest(`{"request_path":"/v1/images/generations"}`))
	assert.True(t, isAdaptiveImageRequest(`{"request_path":"/v1/images/edits"}`))
	assert.True(t, isAdaptiveImageRequest(`{"request_path":"/v1/image_generation"}`))
	assert.False(t, isAdaptiveImageRequest(`{"request_path":"/v1/chat/completions"}`))
}

func TestAdaptiveCandidateScoreUsesSuccessLatencyAndCost(t *testing.T) {
	fast := adaptiveCandidate{
		SlowThresholdMs: 4000,
		Metric: adaptiveMetric{
			Requests:        10,
			Successes:       10,
			TTFTSumMs:       10000,
			TTFTCount:       10,
			ModelPriceSum:   1,
			ModelPriceCount: 10,
		},
	}
	slow := fast
	slow.Metric.Successes = 8
	slow.Metric.TTFTSumMs = 30000
	slow.Metric.ModelPriceSum = 2

	assert.Greater(t, adaptiveCandidateScore(fast, 0.1), adaptiveCandidateScore(slow, 0.1))
}

func TestAdaptiveCostScorePrefersLowerUpstreamCost(t *testing.T) {
	metric := adaptiveMetric{Requests: 10, Successes: 10}
	lowCost := adaptiveCandidate{CostScore: 1, Metric: metric}
	highCost := adaptiveCandidate{CostScore: 0.05, Metric: metric}
	candidates := applyAdaptiveCostScores([]adaptiveCandidate{
		{CostScore: 0.01},
		{CostScore: 0.1},
	})

	assert.Greater(t, candidates[0].CostScore, candidates[1].CostScore)
	assert.Greater(t, adaptiveCandidateScore(lowCost, 0), adaptiveCandidateScore(highCost, 0))
}

func TestAdaptiveBucketCooldownBlocksRecentApplication(t *testing.T) {
	candidates := []adaptiveCandidate{
		{LastAppliedAt: 100, CooldownSeconds: 60},
		{LastAppliedAt: 0, CooldownSeconds: 60},
	}

	assert.True(t, adaptiveBucketInCooldown(candidates, 150))
	assert.False(t, adaptiveBucketInCooldown(candidates, 161))
}

func TestAbilityUsesAdaptiveRoutingMatchesExactAbilityGroupPolicy(t *testing.T) {
	channel := model.Channel{Group: "gpt,image"}
	policies := map[string]model.ChannelControlPolicy{
		"image": {Group: "image", Enabled: true, AdaptiveEnabled: true},
	}

	assert.False(t, abilityUsesAdaptiveRouting(channel, "gpt", nil, policies))
	assert.True(t, abilityUsesAdaptiveRouting(channel, "image", nil, policies))
}
