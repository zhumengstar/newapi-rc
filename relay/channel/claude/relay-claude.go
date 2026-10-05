package claude

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

func stopReasonClaude2OpenAI(reason string) string {
	return relayconvert.StopReasonClaudeToOpenAI(reason)
}

func maybeMarkClaudeRefusal(c *gin.Context, stopReason string) {
	if c == nil {
		return
	}
	if strings.EqualFold(stopReason, "refusal") {
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "claude_stop_reason=refusal")
	}
}

func StreamResponseClaude2OpenAI(claudeResponse *dto.ClaudeResponse) *dto.ChatCompletionsStreamResponse {
	return relayconvert.StreamResponseClaude2OpenAI(claudeResponse)
}

func ResponseClaude2OpenAI(claudeResponse *dto.ClaudeResponse) *dto.OpenAITextResponse {
	return relayconvert.ResponseClaude2OpenAI(claudeResponse)
}

type ClaudeResponseInfo = relayconvert.ClaudeResponseInfo

func cacheCreationTokensForOpenAIUsage(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	openAIUsage := relayconvert.UsageFromClaudeUsage(usage)
	if openAIUsage == nil {
		return 0
	}
	return openAIUsage.PromptTokens - usage.PromptTokens - usage.PromptTokensDetails.CachedTokens
}

func buildOpenAIStyleUsageFromClaudeUsage(usage *dto.Usage) dto.Usage {
	mapped := relayconvert.UsageFromClaudeUsage(usage)
	if mapped == nil {
		return dto.Usage{}
	}
	return *mapped
}

func buildMessageDeltaPatchUsage(claudeResponse *dto.ClaudeResponse, claudeInfo *ClaudeResponseInfo) *dto.ClaudeUsage {
	return relayconvert.BuildMessageDeltaPatchUsage(claudeResponse, claudeInfo)
}

func shouldSkipClaudeMessageDeltaUsagePatch(info *relaycommon.RelayInfo) bool {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return true
	}
	if info == nil {
		return false
	}
	return info.ChannelSetting.PassThroughBodyEnabled
}

func patchClaudeMessageDeltaUsageData(data string, usage *dto.ClaudeUsage) string {
	return relayconvert.PatchClaudeMessageDeltaUsageData(data, usage)
}

func FormatClaudeResponseInfo(claudeResponse *dto.ClaudeResponse, oaiResponse *dto.ChatCompletionsStreamResponse, claudeInfo *ClaudeResponseInfo) bool {
	return relayconvert.FormatClaudeResponseInfo(claudeResponse, oaiResponse, claudeInfo)
}

func shouldStripClaudeCacheCreation(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info != nil && info.ChannelMeta != nil {
		if strings.Contains(strings.ToLower(info.ChannelMeta.ChannelBaseUrl), "mysandbox") {
			return true
		}
		switch info.ChannelMeta.ChannelId {
		case 384, 385, 393, 394, 401, 402, 403, 404:
			return true
		}
	}
	if c != nil {
		baseURL := common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl)
		if strings.Contains(strings.ToLower(baseURL), "mysandbox") {
			return true
		}
		channelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		switch channelId {
		case 384, 385, 393, 394, 401, 402, 403, 404:
			return true
		}
	}
	return false
}

func stripClaudeUsageCacheCreation(u *dto.ClaudeUsage) int {
	if u == nil {
		return 0
	}
	creationTokens := u.GetCacheCreationTotalTokens()
	if creationTokens > 0 {
		u.InputTokens += creationTokens
		u.CacheCreationInputTokens = 0
		u.CacheCreation = nil
		u.ClaudeCacheCreation5mTokens = 0
		u.ClaudeCacheCreation1hTokens = 0
	}
	return creationTokens
}

func stripClaudeInfoUsageCacheCreation(usage *dto.Usage) {
	if usage == nil {
		return
	}
	creation := usage.PromptTokensDetails.CacheCreationTokensTotal()
	if creation == 0 {
		creation = usage.ClaudeCacheCreation5mTokens + usage.ClaudeCacheCreation1hTokens
	}
	if creation > 0 {
		usage.PromptTokens += creation
		usage.PromptTokensDetails.CachedCreationTokens = 0
		usage.PromptTokensDetails.CacheWriteTokens = 0
		usage.ClaudeCacheCreation5mTokens = 0
		usage.ClaudeCacheCreation1hTokens = 0
	}
	if usage.BillingUsage != nil {
		if usage.BillingUsage.ClaudeUsage != nil {
			cUsage := usage.BillingUsage.ClaudeUsage
			cCreation := cUsage.GetCacheCreationTotalTokens()
			if cCreation > 0 {
				cUsage.InputTokens += cCreation
				cUsage.CacheCreationInputTokens = 0
				cUsage.CacheCreation = nil
				cUsage.ClaudeCacheCreation5mTokens = 0
				cUsage.ClaudeCacheCreation1hTokens = 0
			}
		}
		if usage.BillingUsage.OpenAIUsage != nil {
			oUsage := usage.BillingUsage.OpenAIUsage
			oCreation := oUsage.PromptTokensDetails.CacheCreationTokensTotal()
			if oCreation == 0 {
				oCreation = oUsage.ClaudeCacheCreation5mTokens + oUsage.ClaudeCacheCreation1hTokens
			}
			if oCreation > 0 {
				oUsage.PromptTokens += oCreation
				oUsage.PromptTokensDetails.CachedCreationTokens = 0
				oUsage.PromptTokensDetails.CacheWriteTokens = 0
				oUsage.ClaudeCacheCreation5mTokens = 0
				oUsage.ClaudeCacheCreation1hTokens = 0
			}
		}
	}
}

func stripClaudeStreamChunkCacheCreation(resp *dto.ClaudeResponse, data string) string {
	if resp == nil {
		return data
	}
	if resp.Message != nil && resp.Message.Usage != nil {
		stripClaudeUsageCacheCreation(resp.Message.Usage)
		data, _ = sjson.Set(data, "message.usage.input_tokens", resp.Message.Usage.InputTokens)
		data, _ = sjson.Delete(data, "message.usage.cache_creation_input_tokens")
		data, _ = sjson.Delete(data, "message.usage.cache_creation")
		data, _ = sjson.Delete(data, "message.usage.claude_cache_creation_5_m_tokens")
		data, _ = sjson.Delete(data, "message.usage.claude_cache_creation_1_h_tokens")
	}
	if resp.Usage != nil {
		stripClaudeUsageCacheCreation(resp.Usage)
		data, _ = sjson.Set(data, "usage.input_tokens", resp.Usage.InputTokens)
		data, _ = sjson.Delete(data, "usage.cache_creation_input_tokens")
		data, _ = sjson.Delete(data, "usage.cache_creation")
		data, _ = sjson.Delete(data, "usage.claude_cache_creation_5_m_tokens")
		data, _ = sjson.Delete(data, "usage.claude_cache_creation_1_h_tokens")
	}
	return data
}

func HandleStreamResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, data string) *types.NewAPIError {
	var claudeResponse dto.ClaudeResponse
	err := common.UnmarshalJsonStr(data, &claudeResponse)
	if err != nil {
		common.SysLog("error unmarshalling stream response: " + err.Error())
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if shouldStripClaudeCacheCreation(c, info) {
		data = stripClaudeStreamChunkCacheCreation(&claudeResponse, data)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		return types.WithClaudeError(*claudeError, http.StatusInternalServerError)
	}
	if claudeResponse.StopReason != "" {
		maybeMarkClaudeRefusal(c, claudeResponse.StopReason)
	}
	if claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil {
		maybeMarkClaudeRefusal(c, *claudeResponse.Delta.StopReason)
	}
	if info.RelayFormat == types.RelayFormatClaude {
		FormatClaudeResponseInfo(&claudeResponse, nil, claudeInfo)
		if shouldStripClaudeCacheCreation(c, info) && claudeInfo != nil {
			stripClaudeInfoUsageCacheCreation(claudeInfo.Usage)
		}

		if claudeResponse.Type == "message_start" {
			// message_start, 获取usage
			if claudeResponse.Message != nil {
				info.UpstreamModelName = claudeResponse.Message.Model
			}
		} else if claudeResponse.Type == "message_delta" {
			// 确保 message_delta 的 usage 包含完整的 input_tokens 和 cache 相关字段
			// 解决 AWS Bedrock 等上游返回的 message_delta 缺少这些字段的问题
			if !shouldSkipClaudeMessageDeltaUsagePatch(info) {
				data = patchClaudeMessageDeltaUsageData(data, buildMessageDeltaPatchUsage(&claudeResponse, claudeInfo))
			}
			if shouldStripClaudeCacheCreation(c, info) {
				data, _ = sjson.Delete(data, "usage.cache_creation_input_tokens")
				data, _ = sjson.Delete(data, "usage.cache_creation")
				data, _ = sjson.Delete(data, "usage.claude_cache_creation_5_m_tokens")
				data, _ = sjson.Delete(data, "usage.claude_cache_creation_1_h_tokens")
			}
		}
		countClaudeStreamBillableTools(c, info, &claudeResponse)
		if !info.ConvertNonStreamToStream {
			helper.ClaudeChunkData(c, claudeResponse, data)
		}
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		state, err := claudeToChatStreamState(info)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		response, err := state.ConvertChunk(&claudeResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}

		if !FormatClaudeResponseInfo(&claudeResponse, response, claudeInfo) {
			return nil
		}

		countClaudeStreamBillableTools(c, info, &claudeResponse)

		if response == nil {
			return nil
		}
		if !info.ConvertNonStreamToStream {
			err = helper.ObjectData(c, response)
			if err != nil {
				logger.LogError(c, "send_stream_response_failed: "+err.Error())
			}
		}
	} else if info.RelayFormat == types.RelayFormatGemini {
		state, err := claudeToGeminiStreamState(info)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		results, err := service.ConvertStreamResponseChunk(c, info, state, &claudeResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		if !FormatClaudeResponseInfo(&claudeResponse, nil, claudeInfo) {
			return nil
		}
		countClaudeStreamBillableTools(c, info, &claudeResponse)
		if !info.ConvertNonStreamToStream {
			if sendErr := sendGeminiStreamResults(c, results); sendErr != nil {
				return sendErr
			}
		}
	}
	return nil
}

func claudeToChatStreamState(info *relaycommon.RelayInfo) (*relayconvert.ClaudeToChatStreamState, error) {
	if info != nil && info.ClaudeToChatStreamState != nil {
		state, ok := info.ClaudeToChatStreamState.(*relayconvert.ClaudeToChatStreamState)
		if !ok || state == nil {
			return nil, fmt.Errorf("invalid Claude-to-Chat stream state %T", info.ClaudeToChatStreamState)
		}
		return state, nil
	}

	state := relayconvert.NewClaudeToChatStreamState()
	if info != nil {
		info.ClaudeToChatStreamState = state
	}
	return state, nil
}

func claudeToGeminiStreamState(info *relaycommon.RelayInfo) (*relayconvert.ResponseStreamState, error) {
	if info != nil && info.ChatToGeminiStreamState != nil {
		state, ok := info.ChatToGeminiStreamState.(*relayconvert.ResponseStreamState)
		if !ok || state == nil {
			return nil, fmt.Errorf("invalid Claude-to-Gemini stream state %T", info.ChatToGeminiStreamState)
		}
		return state, nil
	}

	state, err := relayconvert.NewResponseStreamState(types.RelayFormatClaude, types.RelayFormatGemini, relayconvert.ResponseStreamOptions{})
	if err != nil {
		return nil, err
	}
	if info != nil {
		info.ChatToGeminiStreamState = state
	}
	return state, nil
}

func sendGeminiStreamResults(c *gin.Context, results []relayconvert.ResponseResult) *types.NewAPIError {
	for _, result := range results {
		geminiResponse, ok := result.Value.(*dto.GeminiChatResponse)
		if !ok {
			return types.NewError(fmt.Errorf("expected Gemini stream response, got %T", result.Value), types.ErrorCodeBadResponseBody)
		}
		if geminiResponse == nil {
			continue
		}
		data, err := common.Marshal(geminiResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		c.Render(-1, common.CustomEvent{Data: "data: " + string(data)})
		_ = helper.FlushWriter(c)
	}
	return nil
}

func countClaudeStreamBillableTools(c *gin.Context, info *relaycommon.RelayInfo, claudeResponse *dto.ClaudeResponse) {
	if claudeResponse == nil {
		return
	}
	if claudeResponse.Type == "content_block_start" &&
		claudeResponse.ContentBlock != nil &&
		claudeResponse.ContentBlock.Type == "tool_use" {
		info.CountBillableToolCall(dto.BuildInCallToolUse, claudeResponse.ContentBlock.Name)
	}
	if claudeResponse.Type == "message_delta" &&
		claudeResponse.Usage != nil &&
		claudeResponse.Usage.ServerToolUse != nil &&
		claudeResponse.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", claudeResponse.Usage.ServerToolUse.WebSearchRequests)
	}
}

func HandleStreamFinalResponse(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo) {
	if claudeInfo.Usage.PromptTokens == 0 {
		//上游出错
	}
	if claudeInfo.Usage.CompletionTokens == 0 || !claudeInfo.Done {
		if common.DebugEnabled {
			common.SysLog("claude response usage is not complete, maybe upstream error")
		}
		// 只补缺失字段，不整份覆盖——保留 message_start 已拿到的 cache 字段
		fallback := service.ResponseText2Usage(c, claudeInfo.ResponseText.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		if claudeInfo.Usage.CompletionTokens == 0 ||
			(!claudeInfo.Done && fallback.CompletionTokens > claudeInfo.Usage.CompletionTokens) {
			claudeInfo.Usage.CompletionTokens = fallback.CompletionTokens
		}
		if claudeInfo.Usage.PromptTokens == 0 {
			claudeInfo.Usage.PromptTokens = fallback.PromptTokens
		}
		// 如果经过流式传输接收到了 chunks，但实际文本估算依然为 0，按接收到的 chunk 数保底统计，绝不能返回 0
		if claudeInfo.Usage.CompletionTokens == 0 && info.ReceivedResponseCount > 0 {
			claudeInfo.Usage.CompletionTokens = max(1, min(info.ReceivedResponseCount*15, 1<<30))
		}
		claudeInfo.Usage.TotalTokens = claudeInfo.Usage.PromptTokens + claudeInfo.Usage.CompletionTokens
	}
	if claudeInfo.Usage != nil {
		claudeInfo.Usage.UsageSemantic = "anthropic"
		if shouldStripClaudeCacheCreation(c, info) {
			stripClaudeInfoUsageCacheCreation(claudeInfo.Usage)
		}
		if claudeInfo.Usage.BillingUsage != nil && claudeInfo.Usage.CompletionTokens > 0 {
			claudeInfo.Usage.BillingUsage = dto.CloneBillingUsageWithEstimatedCompletion(claudeInfo.Usage.BillingUsage, claudeInfo.Usage.CompletionTokens)
		}
	}
	relayconvert.FinalizeClaudeStreamBillingUsage(claudeInfo)
	if shouldStripClaudeCacheCreation(c, info) && claudeInfo != nil {
		stripClaudeInfoUsageCacheCreation(claudeInfo.Usage)
	}

	if info.ConvertNonStreamToStream {
		return
	}

	if info.RelayFormat == types.RelayFormatClaude {
		// 当中途异常中断且下游客户端已在接收流式数据时，优雅发送最终 message_delta 与 message_stop 附带实际统计
		if !claudeInfo.Done && c.Writer != nil && c.Writer.Written() && claudeInfo.Usage != nil && claudeInfo.Usage.CompletionTokens > 0 {
			deltaData := fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":%d}}`, claudeInfo.Usage.CompletionTokens)
			helper.ClaudeChunkData(c, dto.ClaudeResponse{Type: "message_delta"}, deltaData)
			helper.ClaudeChunkData(c, dto.ClaudeResponse{Type: "message_stop"}, `{"type":"message_stop"}`)
			claudeInfo.Done = true
		}
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		if info.ShouldIncludeUsage {
			openAIUsage := buildOpenAIStyleUsageFromClaudeUsage(claudeInfo.Usage)
			response := helper.GenerateFinalUsageResponse(claudeInfo.ResponseId, claudeInfo.Created, info.UpstreamModelName, openAIUsage)
			err := helper.ObjectData(c, response)
			if err != nil {
				common.SysLog("send final response failed: " + err.Error())
			}
		}
		helper.Done(c)
	} else if info.RelayFormat == types.RelayFormatGemini {
		state, err := claudeToGeminiStreamState(info)
		if err != nil {
			common.SysLog("error creating Gemini stream state: " + err.Error())
			return
		}
		results, err := service.FinalizeStreamResponse(c, info, state)
		if err != nil {
			common.SysLog("error finalizing Gemini stream response: " + err.Error())
			return
		}
		if sendErr := sendGeminiStreamResults(c, results); sendErr != nil {
			common.SysLog("send final Gemini stream response failed: " + sendErr.Error())
		}
	}
}

func sendNonStreamResponseFromClaude(c *gin.Context, info *relaycommon.RelayInfo, claudeResp *dto.ClaudeResponse, claudeInfo *ClaudeResponseInfo) *types.NewAPIError {
	maybeMarkClaudeRefusal(c, claudeResp.StopReason)
	for _, block := range claudeResp.Content {
		if block.Type == "tool_use" {
			info.CountBillableToolCall(dto.BuildInCallToolUse, block.Name)
		}
	}
	if claudeResp.Usage != nil && claudeResp.Usage.ServerToolUse != nil && claudeResp.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", claudeResp.Usage.ServerToolUse.WebSearchRequests)
	}

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		c.JSON(http.StatusOK, claudeResp)
		return nil
	case types.RelayFormatOpenAI:
		openaiResponse := ResponseClaude2OpenAI(claudeResp)
		if claudeInfo.Usage != nil {
			openaiResponse.Usage = buildOpenAIStyleUsageFromClaudeUsage(claudeInfo.Usage)
		}
		c.JSON(http.StatusOK, openaiResponse)
		return nil
	case types.RelayFormatOpenAIResponses:
		convertResult, err := service.ConvertResponse(c, info, types.RelayFormatOpenAIResponses, claudeResp)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		c.JSON(http.StatusOK, convertResult.Value)
		return nil
	case types.RelayFormatGemini:
		convertResult, err := service.ConvertResponse(c, info, types.RelayFormatGemini, claudeResp)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		c.JSON(http.StatusOK, convertResult.Value)
		return nil
	default:
		c.JSON(http.StatusOK, claudeResp)
		return nil
	}
}

func ClaudeStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	claudeInfo := &ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	claudeAgg := NewClaudeStreamAggregator()
	var err *types.NewAPIError
	var pendingChunks []string
	flushed := false

	flushPending := func() *types.NewAPIError {
		if flushed {
			return nil
		}
		flushed = true
		for _, pendingData := range pendingChunks {
			if ferr := HandleStreamResponseData(c, info, claudeInfo, pendingData); ferr != nil {
				return ferr
			}
		}
		pendingChunks = nil
		return nil
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var claudeResponse dto.ClaudeResponse
		if uerr := common.UnmarshalJsonStr(data, &claudeResponse); uerr == nil {
			if shouldStripClaudeCacheCreation(c, info) {
				data = stripClaudeStreamChunkCacheCreation(&claudeResponse, data)
			}
			claudeAgg.Feed(&claudeResponse)
		}
		if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
			err = types.WithClaudeError(*claudeError, http.StatusInternalServerError)
			sr.Stop(err)
			return
		}

		if info.ConvertNonStreamToStream {
			err = HandleStreamResponseData(c, info, claudeInfo, data)
			if err != nil {
				sr.Stop(err)
			}
			return
		}

		if !flushed {
			if claudeAgg.HasMeaningfulContent() || claudeInfo.ResponseText.Len() > 0 {
				if ferr := flushPending(); ferr != nil {
					err = ferr
					sr.Stop(err)
					return
				}
				err = HandleStreamResponseData(c, info, claudeInfo, data)
				if err != nil {
					sr.Stop(err)
				}
			} else {
				pendingChunks = append(pendingChunks, data)
			}
		} else {
			err = HandleStreamResponseData(c, info, claudeInfo, data)
			if err != nil {
				sr.Stop(err)
			}
		}
	})
	if err != nil {
		if flushed && (info.ReceivedResponseCount > 0 || claudeInfo.ResponseText.Len() > 0 || (claudeInfo.Usage != nil && (claudeInfo.Usage.PromptTokens > 0 || claudeInfo.Usage.CompletionTokens > 0))) {
			HandleStreamFinalResponse(c, info, claudeInfo)
			return claudeInfo.Usage, err
		}
		// 首包产生实质内容前遇到上游错误，客户端从未被写入，重置接收计数并返回错误，允许外层切渠道重试且不扣费
		info.ReceivedResponseCount = 0
		return nil, err
	}

	if !flushed && !info.ConvertNonStreamToStream {
		info.ReceivedResponseCount = 0
		logger.LogWarn(c, fmt.Sprintf("upstream claude returned empty stream response before writing to client, req_id=%s, model=%s", c.GetString(common.RequestIdKey), info.UpstreamModelName))
		return nil, types.NewOpenAIError(
			fmt.Errorf("upstream claude returned empty stream response"),
			types.ErrorCodeEmptyResponse,
			http.StatusBadGateway,
		)
	}

	HandleStreamFinalResponse(c, info, claudeInfo)

	if info.ConvertNonStreamToStream {
		if !claudeAgg.HasMeaningfulContent() && strings.TrimSpace(claudeInfo.ResponseText.String()) == "" {
			logger.LogWarn(c, fmt.Sprintf("upstream claude returned empty stream response (no content, reasoning, or tool_use), req_id=%s, model=%s", c.GetString(common.RequestIdKey), info.UpstreamModelName))
			return nil, types.NewOpenAIError(
				fmt.Errorf("upstream claude returned empty stream response"),
				types.ErrorCodeEmptyResponse,
				http.StatusBadGateway,
			)
		}
		claudeResp := claudeAgg.Build(claudeInfo, info.UpstreamModelName)
		if sendErr := sendNonStreamResponseFromClaude(c, info, claudeResp, claudeInfo); sendErr != nil {
			return claudeInfo.Usage, sendErr
		}
	}

	return claudeInfo.Usage, nil
}

func HandleClaudeResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, httpResp *http.Response, data []byte) *types.NewAPIError {
	var claudeResponse dto.ClaudeResponse
	err := common.Unmarshal(data, &claudeResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		return types.WithClaudeError(*claudeError, http.StatusInternalServerError)
	}
	maybeMarkClaudeRefusal(c, claudeResponse.StopReason)
	if claudeInfo.Usage == nil {
		claudeInfo.Usage = &dto.Usage{}
	}
	if claudeResponse.Usage != nil {
		if shouldStripClaudeCacheCreation(c, info) {
			stripClaudeUsageCacheCreation(claudeResponse.Usage)
		}
		claudeInfo.Usage.PromptTokens = claudeResponse.Usage.InputTokens
		claudeInfo.Usage.CompletionTokens = claudeResponse.Usage.OutputTokens
		claudeInfo.Usage.TotalTokens = claudeResponse.Usage.InputTokens + claudeResponse.Usage.OutputTokens
		claudeInfo.Usage.UsageSemantic = "anthropic"
		claudeInfo.Usage.BillingUsage = dto.CloneBillingUsage(claudeResponse.Usage.BillingUsage)
		if claudeInfo.Usage.BillingUsage == nil {
			claudeInfo.Usage.BillingUsage = dto.NewClaudeMessagesBillingUsage(claudeResponse.Usage)
		}
		claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Usage.CacheReadInputTokens
		claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Usage.CacheCreationInputTokens
		claudeInfo.Usage.ClaudeCacheCreation5mTokens = claudeResponse.Usage.GetCacheCreation5mTokens()
		claudeInfo.Usage.ClaudeCacheCreation1hTokens = claudeResponse.Usage.GetCacheCreation1hTokens()
		if shouldStripClaudeCacheCreation(c, info) {
			stripClaudeInfoUsageCacheCreation(claudeInfo.Usage)
		}
	}
	var responseData []byte
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		openaiResponse := ResponseClaude2OpenAI(&claudeResponse)
		openaiResponse.Usage = buildOpenAIStyleUsageFromClaudeUsage(claudeInfo.Usage)
		responseData, err = common.Marshal(openaiResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	case types.RelayFormatOpenAIResponses:
		convertResult, err := service.ConvertResponse(c, info, types.RelayFormatOpenAIResponses, &claudeResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responsesResponse, ok := convertResult.Value.(*dto.OpenAIResponsesResponse)
		if !ok {
			return types.NewError(fmt.Errorf("expected OpenAI Responses response, got %T", convertResult.Value), types.ErrorCodeBadResponseBody)
		}
		if responseID := helper.GetResponseID(c); responseID != "" {
			responsesResponse.ID = responseID
		}
		responseData, err = common.Marshal(responsesResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	case types.RelayFormatClaude:
		if shouldStripClaudeCacheCreation(c, info) && claudeResponse.Usage != nil {
			dataStr := string(data)
			dataStr, _ = sjson.Set(dataStr, "usage.input_tokens", claudeResponse.Usage.InputTokens)
			dataStr, _ = sjson.Delete(dataStr, "usage.cache_creation_input_tokens")
			dataStr, _ = sjson.Delete(dataStr, "usage.cache_creation")
			dataStr, _ = sjson.Delete(dataStr, "usage.claude_cache_creation_5_m_tokens")
			dataStr, _ = sjson.Delete(dataStr, "usage.claude_cache_creation_1_h_tokens")
			responseData = []byte(dataStr)
		} else {
			responseData = data
		}
	case types.RelayFormatGemini:
		{
			convertResult, convertErr := service.ConvertResponse(c, info, types.RelayFormatGemini, &claudeResponse)
			if convertErr != nil {
				return types.NewError(convertErr, types.ErrorCodeBadResponseBody)
			}
			geminiResponse, ok := convertResult.Value.(*dto.GeminiChatResponse)
			if !ok {
				return types.NewError(fmt.Errorf("expected Gemini generateContent response, got %T", convertResult.Value), types.ErrorCodeBadResponseBody)
			}
			responseData, err = common.Marshal(geminiResponse)
			if err != nil {
				return types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		}
	}

	if claudeResponse.Usage != nil && claudeResponse.Usage.ServerToolUse != nil && claudeResponse.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", claudeResponse.Usage.ServerToolUse.WebSearchRequests)
	}

	for _, block := range claudeResponse.Content {
		if block.Type == "tool_use" {
			info.CountBillableToolCall(dto.BuildInCallToolUse, block.Name)
		}
	}

	service.IOCopyBytesGracefully(c, httpResp, responseData)
	return nil
}

func ClaudeHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	claudeInfo := &ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	logger.LogDebug(c, "responseBody: %s", responseBody)
	handleErr := HandleClaudeResponseData(c, info, claudeInfo, resp, responseBody)
	if handleErr != nil {
		return nil, handleErr
	}
	return claudeInfo.Usage, nil
}
