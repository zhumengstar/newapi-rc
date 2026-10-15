package gemini

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GeminiTextGenerationHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	logger.LogDebug(c, "Gemini native response body: %s", responseBody)

	// 解析为 Gemini 原生响应格式
	var geminiResponse dto.GeminiChatResponse
	err = common.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	countGeminiBillableFunctionCalls(info, &geminiResponse)

	if IsImageGenerationModel(info) {
		if len(geminiResponse.Candidates) == 0 || !HasGeminiInlineImages(&geminiResponse) {
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_native_image_empty_or_blocked")
			return nil, NewImageModerationRejectError()
		}
	}

	isSafetyBlock := false
	var blockReason string
	var finishMsg *string

	if geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != nil && IsGeminiBlockReason(*geminiResponse.PromptFeedback.BlockReason) {
		isSafetyBlock = true
		blockReason = *geminiResponse.PromptFeedback.BlockReason
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, fmt.Sprintf("gemini_block_reason=%s", blockReason))
	} else {
		for _, cand := range geminiResponse.Candidates {
			if cand.FinishReason != nil && IsGeminiBlockReason(*cand.FinishReason) {
				isSafetyBlock = true
				blockReason = *cand.FinishReason
				finishMsg = cand.FinishMessage
				common.SetContextKey(c, constant.ContextKeyAdminRejectReason, fmt.Sprintf("gemini_block_reason=%s", blockReason))
				break
			}
		}
	}

	if len(geminiResponse.Candidates) == 0 {
		if isSafetyBlock {
			refusalText := FormatGeminiBlockRefusalText(blockReason, finishMsg)
			safetyReason := blockReason
			geminiResponse.Candidates = []dto.GeminiChatCandidate{
				{
					Content: dto.GeminiChatContent{
						Role: "model",
						Parts: []dto.GeminiPart{
							{Text: refusalText},
						},
					},
					FinishReason: &safetyReason,
				},
			}
			newBody, errMarshal := common.Marshal(&geminiResponse)
			if errMarshal == nil {
				responseBody = newBody
			}
			usage := buildUsageFromGeminiResponse(c, info, &geminiResponse)
			zeroBillingUsageCompletion(&usage)
			service.IOCopyBytesGracefully(c, resp, responseBody)
			return &usage, nil
		}
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_empty_candidates")
		service.IOCopyBytesGracefully(c, resp, responseBody)
		return nil, nil
	}

	// 如果命中审查且 content 为空，回填明确拒绝文本，拒绝白屏空回
	if isSafetyBlock {
		modified := false
		for i := range geminiResponse.Candidates {
			cand := &geminiResponse.Candidates[i]
			if len(cand.Content.Parts) == 0 && cand.FinishReason != nil && IsGeminiBlockReason(*cand.FinishReason) {
				cand.Content.Role = "model"
				cand.Content.Parts = []dto.GeminiPart{
					{Text: FormatGeminiBlockRefusalText(*cand.FinishReason, cand.FinishMessage)},
				}
				modified = true
			}
		}
		if modified {
			newBody, errMarshal := common.Marshal(&geminiResponse)
			if errMarshal == nil {
				responseBody = newBody
			}
		}
	}

	// 计算使用量（优先上游 UsageMetadata，缺失时本地估算并保留 Gemini 计费语义）
	usage := buildUsageFromGeminiResponse(c, info, &geminiResponse)
	if isSafetyBlock {
		zeroBillingUsageCompletion(&usage)
	}

	if usage.CompletionTokens == 0 {
		logger.LogWarn(c, fmt.Sprintf("GEMINI_NATIVE_ZERO_TOKEN_DEBUG: req_id=%s, model=%s, candidates_count=%d, has_usage=%v, prompt_tok=%d, cand_tok=%d, body=%s",
			c.GetString(common.RequestIdKey), info.UpstreamModelName, len(geminiResponse.Candidates),
			geminiResponse.HasUsageMetadata, geminiResponse.UsageMetadata.PromptTokenCount, geminiResponse.UsageMetadata.CandidatesTokenCount,
			string(responseBody)))
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &usage, nil
}

func NativeGeminiEmbeddingHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	logger.LogDebug(c, "Gemini native embedding response body: %s", responseBody)

	usage := service.ResponseText2Usage(c, "", info.UpstreamModelName, info.GetEstimatePromptTokens())

	if info.IsGeminiBatchEmbedding {
		var geminiResponse dto.GeminiBatchEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	} else {
		var geminiResponse dto.GeminiEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return usage, nil
}

func GeminiTextGenerationStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	helper.SetEventStreamHeaders(c)

	return geminiStreamHandler(c, info, resp, func(data string, geminiResponse *dto.GeminiChatResponse) bool {
		err := helper.StringData(c, data)
		if err != nil {
			logger.LogError(c, "failed to write stream data: "+err.Error())
			return false
		}
		info.SendResponseCount++
		info.MarkStreamContentSent()
		return true
	})
}
