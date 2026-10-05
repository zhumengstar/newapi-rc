package service

import (
	"context"
	"fmt"
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
	"github.com/stretchr/testify/require"
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

	t.Run("ConvertNonStreamToStream request with received responses should settle", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{
			IsStream:                 false,
			ConvertNonStreamToStream: true,
			ReceivedResponseCount:    5,
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

func TestSettlePartialStreamPreservesEstimatedCompletionTokensWithBillingUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("token_name", "test-token")

	info := &relaycommon.RelayInfo{
		IsStream:              true,
		UserId:                1,
		ChannelMeta:           &relaycommon.ChannelMeta{ChannelId: 1},
		OriginModelName:       "claude-opus-4-6",
		ReceivedResponseCount: 5,
		StartTime:             time.Now().Add(-5 * time.Second),
	}

	apiErr := types.NewOpenAIError(assert.AnError, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	// 模拟流式中断时：顶部 usage 估算出了 200 completion tokens，但 BillingUsage 仍为 0
	usage := &dto.Usage{
		PromptTokens:     1000,
		CompletionTokens: 200,
		TotalTokens:      1200,
		BillingUsage: dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{
			InputTokens:  1000,
			OutputTokens: 0,
		}),
	}

	ok := SettlePartialStream(c, info, usage, apiErr)
	assert.True(t, ok)
	assert.True(t, c.GetBool("partial_stream_settled"))

	// 验证 BillingUsage 中的 OutputTokens 已同步更新为 200，绝不为 0
	require.NotNil(t, usage.BillingUsage)
	require.NotNil(t, usage.BillingUsage.ClaudeUsage)
	assert.Equal(t, 200, usage.BillingUsage.ClaudeUsage.OutputTokens)

	// 验证 effectiveBillingUsage 提取时保留 200，不降为 0
	eff := effectiveBillingUsage(usage)
	assert.Equal(t, 200, eff.CompletionTokens)
}

func TestIsClientCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Context canceled in gin request context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
		c.Request = req

		assert.True(t, IsClientCanceled(c, nil))
	})

	t.Run("Context canceled in error", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		assert.True(t, IsClientCanceled(c, context.Canceled))
		assert.True(t, IsClientCanceled(c, fmt.Errorf("Post http://upstream: context canceled")))
		assert.True(t, IsClientCanceled(c, types.NewErrorWithStatusCode(fmt.Errorf("client closed"), types.ErrorCodeDoRequestFailed, 499)))
		assert.False(t, IsClientCanceled(c, fmt.Errorf("connection refused")))
	})
}

func TestShouldSettleClientCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Client canceled should settle even if non-stream", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
		info := &relaycommon.RelayInfo{
			IsStream: false,
		}

		assert.True(t, ShouldSettleClientCanceled(c, info, nil))
	})

	t.Run("Already settled should not settle again", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
		c.Set("partial_stream_settled", true)
		info := &relaycommon.RelayInfo{
			IsStream: false,
		}

		assert.False(t, ShouldSettleClientCanceled(c, info, nil))
	})
}

func TestSettleClientCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("token_name", "test-token")

	info := &relaycommon.RelayInfo{
		IsStream:        false,
		UserId:          1,
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelId: 1},
		OriginModelName: "claude-opus-4-6",
		StartTime:       time.Now().Add(-5 * time.Second),
	}
	info.SetEstimatePromptTokens(1500)

	apiErr := types.NewErrorWithStatusCode(context.Canceled, types.ErrorCodeDoRequestFailed, 499)
	ok := SettleClientCanceled(c, info, nil, apiErr)
	assert.True(t, ok)
	assert.True(t, c.GetBool("partial_stream_settled"))
	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyErrorLogRecorded))
}

func TestSettleInterruptedRequestIfNeeded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("settles client canceled request", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		c.Set("token_name", "test-token")

		info := &relaycommon.RelayInfo{
			IsStream:        false,
			UserId:          1,
			ChannelMeta:     &relaycommon.ChannelMeta{ChannelId: 1},
			OriginModelName: "claude-opus-4-6",
			StartTime:       time.Now().Add(-5 * time.Second),
		}
		info.SetEstimatePromptTokens(100)

		apiErr := types.NewErrorWithStatusCode(context.Canceled, types.ErrorCodeDoRequestFailed, 499)
		ok := SettleInterruptedRequestIfNeeded(c, info, nil, apiErr)
		assert.True(t, ok)
		assert.True(t, c.GetBool("partial_stream_settled"))
	})

	t.Run("settles partial stream output request", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		c.Set("token_name", "test-token")

		info := &relaycommon.RelayInfo{
			IsStream:              true,
			UserId:                1,
			ChannelMeta:           &relaycommon.ChannelMeta{ChannelId: 1},
			OriginModelName:       "claude-opus-4-6",
			ReceivedResponseCount: 3,
			StartTime:             time.Now().Add(-5 * time.Second),
		}
		info.SetEstimatePromptTokens(100)

		apiErr := types.NewErrorWithStatusCode(fmt.Errorf("stream chunk read error"), types.ErrorCodeDoRequestFailed, 500)
		ok := SettleInterruptedRequestIfNeeded(c, info, nil, apiErr)
		assert.True(t, ok)
		assert.True(t, c.GetBool("partial_stream_settled"))
	})

	t.Run("returns false when nothing to settle", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		info := &relaycommon.RelayInfo{
			IsStream: false,
		}

		apiErr := types.NewErrorWithStatusCode(fmt.Errorf("upstream 500"), types.ErrorCodeDoRequestFailed, 500)
		ok := SettleInterruptedRequestIfNeeded(c, info, nil, apiErr)
		assert.False(t, ok)
		assert.False(t, c.GetBool("partial_stream_settled"))
	})

	t.Run("Scenario 2: settles client canceled before first stream chunk (ReceivedResponseCount == 0)", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 模拟客户端在首个 chunk 前断连挂断
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
		c.Set("token_name", "test-token")

		status := relaycommon.NewStreamStatus()
		status.SetEndReason(relaycommon.StreamEndReasonClientGone, context.Canceled)

		info := &relaycommon.RelayInfo{
			IsStream:              true,
			UserId:                1,
			ChannelMeta:           &relaycommon.ChannelMeta{ChannelId: 1},
			OriginModelName:       "claude-opus-4-6",
			ReceivedResponseCount: 0, // 首个 chunk 尚未收到
			StreamStatus:          status,
			StartTime:             time.Now().Add(-5 * time.Second),
		}
		info.SetEstimatePromptTokens(250)

		apiErr := types.NewErrorWithStatusCode(context.Canceled, types.ErrorCodeDoRequestFailed, 499)
		ok := SettleInterruptedRequestIfNeeded(c, info, nil, apiErr)
		assert.True(t, ok)
		assert.True(t, c.GetBool("partial_stream_settled"))
		assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyErrorLogRecorded))
	})
}


