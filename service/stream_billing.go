package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// IsClientCanceled 判断请求是否是由下游客户端断开连接或超时主动取消
func IsClientCanceled(c *gin.Context, err error) bool {
	if c != nil && c.Request != nil && c.Request.Context() != nil {
		if errors.Is(c.Request.Context().Err(), context.Canceled) {
			return true
		}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return true
		}
		var apiErr *types.NewAPIError
		if errors.As(err, &apiErr) && apiErr != nil && apiErr.StatusCode == 499 {
			return true
		}
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "context canceled") || strings.Contains(errStr, "client_gone") || strings.Contains(errStr, "client closed") || strings.Contains(errStr, "broken pipe") || strings.Contains(errStr, "connection reset by peer") {
			return true
		}
	}
	return false
}

// ShouldSettlePartialStream 检查请求是否在流式传输期间已经部分输出，需要按实际已接收/输出的 Token 执行扣费结算（防止长请求中途中断导致 0 元免单）
func ShouldSettlePartialStream(c *gin.Context, info *relaycommon.RelayInfo, usage any) bool {
	if info == nil || (!info.IsStream && !info.ConvertNonStreamToStream) {
		return false
	}
	if c.GetBool("partial_stream_settled") {
		return false
	}
	if info.Billing != nil && info.Billing.IsSettled() {
		return false
	}

	// 1. 如果已有明确的 Token 产生
	if usage != nil {
		if u, ok := usage.(*dto.Usage); ok && u != nil {
			if u.CompletionTokens > 0 || (u.PromptTokens > 0 && info.ReceivedResponseCount > 0) {
				return true
			}
		}
	}

	// 2. 如果流式已接收到 chunks 并转发给客户端
	if info.ReceivedResponseCount > 0 {
		return true
	}

	return false
}

// ShouldSettleClientCanceled 检查是否是客户端主动断连且需要进行计费结算
func ShouldSettleClientCanceled(c *gin.Context, info *relaycommon.RelayInfo, err error) bool {
	if info == nil {
		return false
	}
	if c.GetBool("partial_stream_settled") {
		return false
	}
	if info.Billing != nil && info.Billing.IsSettled() {
		return false
	}
	if info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone {
		return true
	}
	return IsClientCanceled(c, err)
}

// SettleInterruptedRequestIfNeeded 统一检查并结算因客户端断连或流式中途中断的请求
func SettleInterruptedRequestIfNeeded(c *gin.Context, info *relaycommon.RelayInfo, usage any, apiErr *types.NewAPIError) bool {
	if ShouldSettleClientCanceled(c, info, apiErr) {
		return SettleClientCanceled(c, info, usage, apiErr)
	}
	if ShouldSettlePartialStream(c, info, usage) {
		return SettlePartialStream(c, info, usage, apiErr)
	}
	return false
}

// settleInterruptedRequest 统一执行中断请求的扣费结算与日志记录
func settleInterruptedRequest(c *gin.Context, info *relaycommon.RelayInfo, usage any, logMsg string, extraContent []string) bool {
	if info == nil {
		return false
	}
	if c.GetBool("partial_stream_settled") {
		return true
	}
	if info.Billing != nil && info.Billing.IsSettled() {
		return true
	}

	var u *dto.Usage
	if usage != nil {
		if actualUsage, ok := usage.(*dto.Usage); ok && actualUsage != nil {
			u = actualUsage
		}
	}
	if u == nil {
		u = &dto.Usage{}
	}

	if u.PromptTokens == 0 {
		u.PromptTokens = max(0, info.GetEstimatePromptTokens())
	}
	// 如果经过流式传输但未统计到 completion tokens，按接收到的 chunk 数进行保底统计（每个 chunk 估算 15 tokens），并防止溢出
	if u.CompletionTokens == 0 && info.ReceivedResponseCount > 0 {
		u.CompletionTokens = max(1, min(info.ReceivedResponseCount*15, 1<<30))
	}
	if u.TotalTokens == 0 || u.TotalTokens < u.PromptTokens+u.CompletionTokens {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	if u.BillingUsage != nil && u.CompletionTokens > 0 {
		u.BillingUsage = dto.CloneBillingUsageWithEstimatedCompletion(u.BillingUsage, u.CompletionTokens)
	}

	logger.LogWarn(c, logMsg)

	var containAudioTokens = u.CompletionTokenDetails.AudioTokens > 0 || u.PromptTokensDetails.AudioTokens > 0
	var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

	if containAudioTokens && containsAudioRatios {
		PostAudioConsumeQuota(c, info, u, "")
	} else {
		PostTextConsumeQuota(c, info, u, extraContent)
	}

	// 标记已结算，确保后续的 defer Refund 不会退费，避免重复记录 0 额度错误日志
	c.Set("partial_stream_settled", true)
	common.SetContextKey(c, constant.ContextKeyErrorLogRecorded, true)
	return true
}

// SettlePartialStream 对中途中断的流式请求按实际产生的 Token 进行强制结算并记录日志
func SettlePartialStream(c *gin.Context, info *relaycommon.RelayInfo, usage any, apiErr *types.NewAPIError) bool {
	if info == nil {
		return false
	}
	errMsg := "上游中途异常中断"
	if apiErr != nil {
		errMsg = fmt.Sprintf("上游异常中断(%d): %s", apiErr.StatusCode, common.LocalLogPreview(apiErr.Error()))
	}
	extraContent := []string{fmt.Sprintf("流式中途异常中断，已按实际输出结算 [%s]", errMsg)}
	logMsg := fmt.Sprintf("流式请求中途中断但已输出内容，执行保底按实结算: userId=%d, channelId=%d, model=%s, reason=%s",
		info.UserId, info.ChannelId, info.OriginModelName, errMsg)

	return settleInterruptedRequest(c, info, usage, logMsg, extraContent)
}

// SettleClientCanceled 对客户端主动取消/断开连接的请求进行按实扣费结算（防止客户端超时断连导致0元免单）
func SettleClientCanceled(c *gin.Context, info *relaycommon.RelayInfo, usage any, apiErr *types.NewAPIError) bool {
	if info == nil {
		return false
	}
	extraContent := []string{"客户端主动断开连接，按已消耗Token结算"}
	logMsg := fmt.Sprintf("客户端主动断开连接(499)，执行按实扣费结算: userId=%d, channelId=%d, model=%s",
		info.UserId, info.ChannelId, info.OriginModelName)

	return settleInterruptedRequest(c, info, usage, logMsg, extraContent)
}

