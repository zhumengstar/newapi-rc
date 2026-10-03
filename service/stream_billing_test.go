package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestShouldSettlePartialStream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Non-stream request should not settle", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		info := &relaycommon.RelayInfo{
			IsStream: false,
		}
		assert.False(t, ShouldSettlePartialStream(c, info, nil))
	})

	t.Run("Stream request with zero received should not settle", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{
			IsStream:              true,
			ReceivedResponseCount: 0,
		}
		assert.False(t, ShouldSettlePartialStream(c, info, nil))
	})

	t.Run("Stream request with received responses should settle", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{
			IsStream:              true,
			ReceivedResponseCount: 5,
		}
		assert.True(t, ShouldSettlePartialStream(c, info, nil))
	})

	t.Run("Stream request with usage completion tokens should settle", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{
			IsStream: true,
		}
		usage := &dto.Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
		}
		assert.True(t, ShouldSettlePartialStream(c, info, usage))
	})

	t.Run("Stream request already settled should not settle again", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		c.Set("partial_stream_settled", true)
		info := &relaycommon.RelayInfo{
			IsStream:              true,
			ReceivedResponseCount: 5,
		}
		assert.False(t, ShouldSettlePartialStream(c, info, nil))
	})

	t.Run("Trusted user with 0 preconsume should settle when stream has responses", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		mockBilling := &recordingBillingSettler{preConsumedQuota: 0}
		info := &relaycommon.RelayInfo{
			IsStream:              true,
			ReceivedResponseCount: 10,
			Billing:               mockBilling,
		}
		assert.True(t, ShouldSettlePartialStream(c, info, nil))
	})
}

func TestSettlePartialStreamMarksErrorLogRecorded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("token_name", "test-token")

	info := &relaycommon.RelayInfo{
		IsStream:              true,
		UserId:                1,
		ChannelMeta:           &relaycommon.ChannelMeta{ChannelId: 1},
		OriginModelName:       "claude-opus-4-6-c",
		ReceivedResponseCount: 10,
		StartTime:             time.Now().Add(-5 * time.Second),
	}

	apiErr := types.NewOpenAIError(assert.AnError, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	usage := &dto.Usage{
		PromptTokens:     500,
		CompletionTokens: 300,
		TotalTokens:      800,
	}

	ok := SettlePartialStream(c, info, usage, apiErr)
	assert.True(t, ok)
	assert.True(t, c.GetBool("partial_stream_settled"))
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyErrorLogRecorded))
}
