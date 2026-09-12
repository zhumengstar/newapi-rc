package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeduplicateChannelGuardEventsPrefersSuccessfulTerminalLog(t *testing.T) {
	logs := []model.Log{
		{Id: 2, CreatedAt: 101, Type: model.LogTypeError, ChannelId: 7, RequestId: "req-1", Content: "status_code=502 upstream failed"},
		{Id: 1, CreatedAt: 100, Type: model.LogTypeConsume, ChannelId: 7, RequestId: "req-1", Quota: 100},
		{Id: 3, CreatedAt: 102, Type: model.LogTypeError, ChannelId: 7, RequestId: "req-2", Content: "status_code=503 upstream failed"},
	}

	events := deduplicateChannelGuardEvents(logs)
	require.Len(t, events, 2)
	var successful bool
	for _, event := range events {
		if event.RequestID == "7:req-1" {
			successful = event.Type == model.LogTypeConsume && event.Quota > 0
		}
	}
	assert.True(t, successful)
}

func TestFindChannelGuardCandidatesUsesHardRuleAndIgnoresNonChannelErrors(t *testing.T) {
	events := []channelGuardEvent{
		{ID: 1, CreatedAt: 100, ChannelID: 9, Type: model.LogTypeError, Status: 413, Content: "payload too large"},
		{ID: 2, CreatedAt: 101, ChannelID: 9, Type: model.LogTypeError, Status: 403, Content: "insufficient quota"},
		{ID: 3, CreatedAt: 102, ChannelID: 9, Type: model.LogTypeError, Status: 403, Content: "insufficient quota"},
		{ID: 4, CreatedAt: 103, ChannelID: 9, Type: model.LogTypeError, Status: 403, Content: "insufficient quota"},
	}

	candidates := findChannelGuardCandidates(events, 110)
	require.Len(t, candidates, 1)
	assert.Equal(t, "http_403:quota", candidates[0].Signature)
	assert.Equal(t, 3, candidates[0].Count)
	assert.True(t, candidates[0].Hard)
}

func TestFindChannelGuardCandidatesTripsTransientStreak(t *testing.T) {
	events := make([]channelGuardEvent, 0, channelErrorGuardTransientStreak)
	for i := 0; i < channelErrorGuardTransientStreak; i++ {
		events = append(events, channelGuardEvent{
			ID: i + 1, CreatedAt: int64(200 + i), ChannelID: 11,
			Type: model.LogTypeError, Status: 503, Content: "upstream timeout",
		})
	}

	candidates := findChannelGuardCandidates(events, 220)
	require.Len(t, candidates, 1)
	assert.Equal(t, "http_503:timeout", candidates[0].Signature)
	assert.Equal(t, channelErrorGuardTransientStreak, candidates[0].Streak)
}

func TestChannelGuardSignatureClassifiesStatusFromMessage(t *testing.T) {
	signature, eligible, hard := channelGuardSignature(channelGuardEvent{
		Type: model.LogTypeError, Status: 401, Content: "invalid api key",
	})
	assert.Equal(t, "http_401:invalid_token", signature)
	assert.True(t, eligible)
	assert.True(t, hard)

	_, eligible, _ = channelGuardSignature(channelGuardEvent{
		Type: model.LogTypeError, Status: 400, Content: "model not found",
	})
	assert.False(t, eligible)

	logs := []model.Log{{Id: 1, CreatedAt: 1, ChannelId: 3, Type: model.LogTypeError, RequestId: "req", Content: "status_code=504 upstream unavailable"}}
	events := deduplicateChannelGuardEvents(logs)
	require.Len(t, events, 1)
	assert.Equal(t, 504, events[0].Status)
}
