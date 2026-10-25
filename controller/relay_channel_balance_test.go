package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelBalanceExhaustionRetryAndMasking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	// 1. All variations of channel balance errors MUST trigger retry (shouldRetry = true)
	balanceCases := []*types.NewAPIError{
		types.NewOpenAIError(errors.New("status_code=403, Insufficient account balance"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden),
		types.NewOpenAIError(errors.New("status_code=403, 余额不足"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden),
		types.NewOpenAIError(errors.New("status_code=400, 当前分组余额不足"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest),
		types.NewOpenAIError(errors.New("status_code=402, Payment Required"), types.ErrorCodeBadResponseStatusCode, http.StatusPaymentRequired),
		types.NewOpenAIError(errors.New("status_code=503, 429 RESOURCE_EXHAUSTED: Individual quota reached. Please upgrade your subscription"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable),
		types.NewOpenAIError(errors.New("Your credit balance is too low"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden),
		types.NewOpenAIError(errors.New("You exceeded your current quota"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden),
		types.NewOpenAIError(errors.New("out of credits"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden),
	}

	for _, err := range balanceCases {
		require.True(t, shouldRetry(ctx, err, 1), "failed to retry for: %s", err.Error())
		require.True(t, service.IsFastCircuitError(err), "failed fast circuit for: %s", err.Error())

		// Test OpenAI format masking
		oaiErr := err.ToOpenAIError()
		require.Equal(t, types.ChannelBalanceExhaustedMaskedMessage, oaiErr.Message)

		// Test Claude format masking
		claudeErr := err.ToClaudeError()
		require.Equal(t, types.ChannelBalanceExhaustedMaskedMessage, claudeErr.Message)
	}

	// 2. Error with request ID preserves request ID
	withReqId := types.NewOpenAIError(errors.New("status_code=403, Insufficient account balance (request id: 202610250810297960980528268d9d658cAtPSK)"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden)
	require.Equal(t, "The service is temporarily unavailable. Please try again shortly. (request id: 202610250810297960980528268d9d658cAtPSK)", withReqId.ToOpenAIError().Message)

	// 3. User own quota error MUST NOT be retried and MUST NOT be masked
	userQuotaErr := types.NewErrorWithStatusCode(errors.New("用户额度不足, 剩余额度: 0"), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden)
	require.False(t, shouldRetry(ctx, userQuotaErr, 1))
	require.False(t, service.IsFastCircuitError(userQuotaErr))
	require.Equal(t, "用户额度不足, 剩余额度: 0", userQuotaErr.ToOpenAIError().Message)
}
