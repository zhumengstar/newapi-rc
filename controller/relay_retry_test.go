package controller

import (
	"errors"
	"net/http"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryHonorsBudgetAndSpecificChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	err := types.NewErrorWithStatusCode(errors.New("channel invalid key"), types.ErrorCodeChannelInvalidKey, http.StatusUnauthorized)

	require.False(t, shouldRetry(ctx, err, 0))
	ctx.Set("specific_channel_id", 123)
	require.False(t, shouldRetry(ctx, err, 1))
}

func TestShouldRetryRejectsClientFailuresEvenWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	previous := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { operation_setting.AutomaticRetryStatusCodeRanges = previous })
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 400, End: 599}}

	tests := []*types.NewAPIError{
		types.NewErrorWithStatusCode(errors.New("bad json"), types.ErrorCodeInvalidRequest, http.StatusBadRequest),
		types.NewErrorWithStatusCode(errors.New("quota"), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden),
		types.NewErrorWithStatusCode(errors.New("blocked"), types.ErrorCodePromptBlocked, http.StatusForbidden),
		types.NewErrorWithStatusCode(errors.New("model"), types.ErrorCodeModelNotFound, http.StatusNotFound),
	}
	for _, testErr := range tests {
		require.False(t, shouldRetry(ctx, testErr, 1), testErr.GetErrorCode())
	}
}

func TestShouldRetryRecoverableUpstreamFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	tests := []*types.NewAPIError{
		types.NewOpenAIError(errors.New("connection reset"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError),
		types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests),
		types.NewOpenAIError(errors.New("insufficient credit"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden),
		types.NewErrorWithStatusCode(errors.New("no available capacity"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry()),
		types.NewOpenAIError(errors.New("Unable to process input image. Please retry or report in https://..."), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest),
		// 上游返回 500 敏感词或内容安全拦截，必须允许重试换下一个渠道
		types.NewErrorWithStatusCode(errors.New("sensitive_words_detected"), types.ErrorCodeSensitiveWordsDetected, http.StatusInternalServerError),
		types.NewOpenAIError(errors.New("status_code=500, sensitive_words_detected"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError),
		types.NewOpenAIError(errors.New("status_code=500, prompt was blocked by content filter"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError),
	}
	for _, testErr := range tests {
		require.True(t, shouldRetry(ctx, testErr, 1), testErr.Error())
	}
}

func TestShouldRetryLocalSensitiveWordsRejectsRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	// 本地敏感词拦截带 skip_retry，禁止发起重试
	localErr := types.NewError(errors.New("local sensitive words detected"), types.ErrorCodeSensitiveWordsDetected, types.ErrOptionWithSkipRetry(), types.ErrOptionWithStatusCode(http.StatusBadRequest))
	require.False(t, shouldRetry(ctx, localErr, 1))
}

func TestShouldRetryRejectsNonRecoverableUpstreamFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	err := types.NewOpenAIError(errors.New("invalid argument"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)
	require.False(t, shouldRetry(ctx, err, 1))
}

func TestRetrySelectionFailurePreservesPreviousUpstreamError(t *testing.T) {
	upstreamErr := types.NewOpenAIError(errors.New("temporary upstream failure"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable)
	selectionErr := types.NewError(errors.New("no alternative channel"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	require.Same(t, upstreamErr, preservePreviousRelayError(upstreamErr, selectionErr))
	require.Same(t, selectionErr, preservePreviousRelayError(nil, selectionErr))
}

func TestSlowImageAttemptStopsRetryOnlyForImages(t *testing.T) {
	imageInfo := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}
	textInfo := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions}
	require.True(t, shouldStopRetryAfterSlowImageAttempt(imageInfo, 60*time.Second))
	require.False(t, shouldStopRetryAfterSlowImageAttempt(imageInfo, 59*time.Second))
	require.False(t, shouldStopRetryAfterSlowImageAttempt(textInfo, 90*time.Second))
}

func TestEmptyResponsesOutputCanRetryBeforeUsableOutput(t *testing.T) {
	empty := types.NewOpenAIError(errors.New("upstream responses returned no output"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	other := types.NewOpenAIError(errors.New("invalid upstream response"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	require.True(t, isEmptyResponsesOutputError(empty))
	require.False(t, isEmptyResponsesOutputError(other))
}

func TestRetryAfterIsBounded(t *testing.T) {
	require.Equal(t, time.Second, boundedRetryAfter(2*time.Hour))
	require.Equal(t, 250*time.Millisecond, boundedRetryAfter(250*time.Millisecond))
	require.Zero(t, boundedRetryAfter(0))
}

func TestShouldStopRetryAfterResponseStarted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	// Stream request without content sent (e.g. only ping sent) -> allow retry (returns false)
	streamInfoNoContent := &relaycommon.RelayInfo{IsStream: true, StreamContentSent: false}
	err502 := types.NewOpenAIError(errors.New("Upstream request failed"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	require.False(t, shouldStopRetryAfterResponseStarted(ctx, streamInfoNoContent, err502))

	// Stream request with content sent -> stop retry (returns true)
	streamInfoWithContent := &relaycommon.RelayInfo{IsStream: true, StreamContentSent: true}
	require.True(t, shouldStopRetryAfterResponseStarted(ctx, streamInfoWithContent, err502))

	// Responses empty output error always allows retry
	emptyResponses := types.NewOpenAIError(errors.New("upstream responses returned no output"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	require.False(t, shouldStopRetryAfterResponseStarted(ctx, streamInfoWithContent, emptyResponses))
}

func TestIsHostLevelFailure(t *testing.T) {
	timeoutErr := types.NewErrorWithStatusCode(errors.New("upstream stream first-byte timeout after 60s"), types.ErrorCodeBadResponse, http.StatusGatewayTimeout)
	require.True(t, isHostLevelFailure(timeoutErr))

	badGatewayErr := types.NewOpenAIError(errors.New("Upstream request failed"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	require.True(t, isHostLevelFailure(badGatewayErr))

	unavailableErr := types.NewOpenAIError(errors.New("Upstream service temporarily unavailable"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	require.True(t, isHostLevelFailure(unavailableErr))

	rateLimitErr := types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	require.False(t, isHostLevelFailure(rateLimitErr))

	authErr := types.NewOpenAIError(errors.New("invalid api key"), types.ErrorCodeChannelInvalidKey, http.StatusUnauthorized)
	require.False(t, isHostLevelFailure(authErr))
}

