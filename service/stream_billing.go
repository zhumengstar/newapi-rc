package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// ShouldSettlePartialStream 检查请求是否在流式传输期间已经部分输出，需要按实际已接收/输出的 Token 执行扣费结算（防止长请求中途中断导致 0 元免单）
func ShouldSettlePartialStream(c *gin.Context, info *relaycommon.RelayInfo, usage any) bool {
	if info == nil || !info.IsStream {
		return false
	}
	if c.GetBool("partial_stream_settled") {
		return false
	}
	if info.Billing != nil && !info.Billing.NeedsRefund() {
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

	// 2. 如果流式已接收到 chunks 并转发给客户端，或者客户端已经收到了 response 数据
	if info.ReceivedResponseCount > 0 || (c.Writer != nil && c.Writer.Written() && !info.FirstResponseTime.IsZero()) {
		return true
	}

	return false
}

// SettlePartialStream 对中途中断的流式请求按实际产生的 Token 进行强制结算并记录日志
func SettlePartialStream(c *gin.Context, info *relaycommon.RelayInfo, usage any, apiErr *types.NewAPIError) bool {
	if info == nil {
		return false
	}
	if c.GetBool("partial_stream_settled") {
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
		u.PromptTokens = info.GetEstimatePromptTokens()
	}
	// 如果经过流式传输但未统计到 completion tokens，按接收到的 chunk 数进行保底统计（每个 chunk 估算 15 tokens）
	if u.CompletionTokens == 0 && info.ReceivedResponseCount > 0 {
		u.CompletionTokens = info.ReceivedResponseCount * 15
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}

	errMsg := "上游中途异常中断"
	if apiErr != nil {
		errMsg = fmt.Sprintf("上游异常中断(%d): %s", apiErr.StatusCode, common.LocalLogPreview(apiErr.Error()))
	}
	extraContent := []string{fmt.Sprintf("流式中途异常中断，已按实际输出结算 [%s]", errMsg)}

	logger.LogWarn(c, fmt.Sprintf("流式请求中途中断但已输出内容，执行保底按实结算: userId=%d, channelId=%d, model=%s, promptTokens=%d, completionTokens=%d, reason=%s",
		info.UserId, info.ChannelId, info.OriginModelName, u.PromptTokens, u.CompletionTokens, errMsg))

	var containAudioTokens = u.CompletionTokenDetails.AudioTokens > 0 || u.PromptTokensDetails.AudioTokens > 0
	var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

	if containAudioTokens && containsAudioRatios {
		PostAudioConsumeQuota(c, info, u, "")
	} else {
		PostTextConsumeQuota(c, info, u, extraContent)
	}

	// 标记已结算，避免外层再记录重复的 0 额度错误日志
	c.Set("partial_stream_settled", true)
	common.SetContextKey(c, constant.ContextKeyErrorLogRecorded, true)
	return true
}
