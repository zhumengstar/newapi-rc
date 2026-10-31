package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	// 1. 核心判定：下游客户端的 HTTP 连接已断开/取消（真正的客户中断）
	if c != nil && c.Request != nil && c.Request.Context() != nil {
		if errors.Is(c.Request.Context().Err(), context.Canceled) {
			return true
		}
	}
	// 2. 检查错误是否显式标记为客户端中断（例如 499 或客户端主动挂断/reset）
	if err != nil {
		var apiErr *types.NewAPIError
		if errors.As(err, &apiErr) && apiErr != nil {
			if apiErr.StatusCode == 499 {
				return true
			}
			if apiErr.StatusCode == 504 || strings.Contains(strings.ToLower(apiErr.Error()), "first-byte timeout") {
				return false
			}
		}
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "first-byte timeout") {
			return false
		}
		if strings.Contains(errStr, "client_gone") || strings.Contains(errStr, "client closed") || strings.Contains(errStr, "broken pipe") || strings.Contains(errStr, "connection reset by peer") {
			return true
		}
	}
	// 3. 如果下游客户端仍然正常连接，上游的超时或连接中断绝不是客户中断，不能转嫁给客户
	if c != nil && c.Request != nil && c.Request.Context() != nil && c.Request.Context().Err() == nil {
		return false
	}
	// 4. 当缺少下游客户端上下文时（如部分离线或测试），回退到检查 context.Canceled
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return true
		}
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "context canceled") {
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
	if !IsClientCanceled(c, err) {
		return false
	}

	// 针对按次固定计费模型（如出图、MJ等固定价格模型，info.PriceData.UsePrice 或 info.PriceData.ModelPrice > 0）：
	// 严格防作弊与防误扣原则：
	// 1. 若上游已产生返回（FirstResponseTime > 0 或 ReceivedResponseCount > 0 或 StreamContentSent），说明上游已交付内容，必须坚决扣费防白嫖；
	// 2. 若上游毫无返回（零字节/零响应头/零Chunk），且客户端等待耗时达到超时级别（>= 30秒），
	//    确认为上游假死/排队超时导致的客户端被迫超时断开，绝不扣除整次费用，应退款给客户端并判定为上游超时；
	// 3. 若等待时间极短（< 30秒）秒断，不属于超时断开，按原有规则结算防恶意秒断滥刷。
	if info.PriceData.UsePrice || info.PriceData.ModelPrice > 0 {
		hasUpstreamResponse := !info.FirstResponseTime.IsZero() || info.ReceivedResponseCount > 0 || info.StreamContentSent
		if !hasUpstreamResponse && !info.StartTime.IsZero() && time.Since(info.StartTime) >= 30*time.Second {
			logger.LogWarn(c, fmt.Sprintf("按次模型客户端超时断开但上游零返回(等待%v): userId=%d, channelId=%d, model=%s, 判定为上游超时不予扣费并退款",
				time.Since(info.StartTime), info.UserId, info.GetChannelID(), info.GetOriginModelName()))
			return false
		}
	}

	return true
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

// settleInterruptedRequest 统一执行中断请求的扣费结算与日志记录（客户中断由客户按实际消耗买单）
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

