package gemini

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func buildUsageFromGeminiMetadata(metadata *dto.GeminiUsageMetadata, fallbackPromptTokens int) dto.Usage {
	usage := relayconvert.UsageFromGeminiMetadata(metadata, fallbackPromptTokens)
	if usage == nil {
		return dto.Usage{}
	}
	return *usage
}

func attachEstimatedGeminiBillingUsage(usage *dto.Usage) *dto.Usage {
	if usage != nil && usage.BillingUsage == nil {
		usage.BillingUsage = dto.NewEstimatedGeminiChatBillingUsage(usage)
	}
	return usage
}

func zeroBillingUsageCompletion(usage *dto.Usage) {
	if usage == nil {
		return
	}
	usage.CompletionTokens = 0
	usage.TotalTokens = usage.PromptTokens
	if usage.BillingUsage != nil {
		if usage.BillingUsage.OpenAIUsage != nil {
			usage.BillingUsage.OpenAIUsage.CompletionTokens = 0
			usage.BillingUsage.OpenAIUsage.OutputTokens = 0
			usage.BillingUsage.OpenAIUsage.TotalTokens = usage.BillingUsage.OpenAIUsage.PromptTokens
		}
		if usage.BillingUsage.ClaudeUsage != nil {
			usage.BillingUsage.ClaudeUsage.OutputTokens = 0
		}
		if usage.BillingUsage.GeminiUsageMetadata != nil {
			usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount = 0
			usage.BillingUsage.GeminiUsageMetadata.ThoughtsTokenCount = 0
			usage.BillingUsage.GeminiUsageMetadata.TotalTokenCount = usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount + usage.BillingUsage.GeminiUsageMetadata.ToolUsePromptTokenCount
		}
	} else {
		attachEstimatedGeminiBillingUsage(usage)
	}
}

// patchGeminiZeroCompletionUsage estimates completion tokens locally when upstream
// usageMetadata was billable but reported zero completion tokens even though output
// content was actually received. Typical case: the client aborts a stream before the
// final chunk that carries candidatesTokenCount, leaving prompt-only metadata; without
// this patch the output side would settle at zero quota.
func patchGeminiZeroCompletionUsage(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage, responseText string, imageCount int) {
	if usage == nil {
		return
	}
	needPatch := usage.CompletionTokens == 0
	if !needPatch && usage.CompletionTokens == 1 && len(strings.TrimSpace(responseText)) > 10 {
		needPatch = true
	}
	if !needPatch {
		return
	}
	if responseText == "" && imageCount == 0 {
		return
	}
	estimated := service.ResponseText2Usage(c, responseText, info.UpstreamModelName, usage.PromptTokens)
	if estimated.CompletionTokens > usage.CompletionTokens {
		usage.CompletionTokens = estimated.CompletionTokens
	}
	if imageCount != 0 && usage.CompletionTokens == 0 {
		usage.CompletionTokens = imageCount * 1400
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	// Settlement prefers BillingUsage, so fill the missing completion in the
	// original upstream dialect without discarding cache or modality details.
	if usage.BillingUsage != nil {
		usage.BillingUsage = dto.CloneBillingUsageWithEstimatedCompletion(usage.BillingUsage, usage.CompletionTokens)
	} else {
		usage.BillingUsage = dto.NewEstimatedGeminiChatBillingUsage(usage)
	}
}

func geminiResponseUsageText(response *dto.GeminiChatResponse) string {
	if response == nil {
		return ""
	}
	var text strings.Builder
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				text.WriteString(part.Text)
			}
			if part.FunctionCall != nil {
				text.WriteString(part.FunctionCall.FunctionName)
				if part.FunctionCall.Arguments != nil {
					if argsBytes, err := common.Marshal(part.FunctionCall.Arguments); err == nil {
						text.Write(argsBytes)
					}
				}
			}
			if part.ExecutableCode != nil {
				text.WriteString(part.ExecutableCode.Code)
			}
		}
	}
	return text.String()
}

func markGeminiGoogleSearchCall(c *gin.Context, response *dto.GeminiChatResponse) {
	if c == nil || response == nil {
		return
	}
	for _, candidate := range response.Candidates {
		if candidate.GroundingMetadata != nil && len(candidate.GroundingMetadata.WebSearchQueries) > 0 {
			c.Set("gemini_google_search_call", true)
			return
		}
	}
}

func countGeminiBillableFunctionCalls(info *relaycommon.RelayInfo, response *dto.GeminiChatResponse) {
	if info == nil || response == nil {
		return
	}
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall == nil {
				continue
			}
			if part.FunctionCall.WillContinue != nil && *part.FunctionCall.WillContinue {
				continue
			}
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, part.FunctionCall.FunctionName)
		}
	}
}

func buildUsageFromGeminiResponse(c *gin.Context, info *relaycommon.RelayInfo, response *dto.GeminiChatResponse) dto.Usage {
	ensureGeminiInlineImageUsageMetadata(response)
	metadata := response.GetUsageMetadata()
	if dto.HasGeminiUsageMetadataTokens(metadata) {
		usage := buildUsageFromGeminiMetadata(metadata, info.GetEstimatePromptTokens())
		patchGeminiZeroCompletionUsage(c, info, &usage, geminiResponseUsageText(response), geminiResponseInlineImageCount(response))
		return usage
	}
	usage := service.ResponseText2Usage(c, geminiResponseUsageText(response), info.UpstreamModelName, info.GetEstimatePromptTokens())
	if imgBaseTokens := geminiResponseInlineImageBaseTokens(response); imgBaseTokens > 0 && usage.CompletionTokens < imgBaseTokens {
		usage.CompletionTokens += imgBaseTokens
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	attachEstimatedGeminiBillingUsage(usage)
	return *usage
}

func decodeBase64Prefix(b64 string, maxChars int) []byte {
	s := strings.TrimSpace(b64)
	if s == "" {
		return nil
	}
	if maxChars > 0 && len(s) > maxChars {
		s = s[:(maxChars/4)*4]
	}
	if raw, err := base64.StdEncoding.DecodeString(s); err == nil && len(raw) > 0 {
		return raw
	}
	if raw, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "=")); err == nil && len(raw) > 0 {
		return raw
	}
	return nil
}

func parseImageDimensionsFromBytes(raw []byte) (int, int) {
	if len(raw) >= 24 && string(raw[:8]) == "\x89PNG\r\n\x1a\n" {
		w := int(binary.BigEndian.Uint32(raw[16:20]))
		h := int(binary.BigEndian.Uint32(raw[20:24]))
		return w, h
	}
	if len(raw) >= 4 && raw[0] == 0xff && raw[1] == 0xd8 {
		i := 2
		for i+4 <= len(raw) {
			if raw[i] != 0xff {
				i++
				continue
			}
			for i < len(raw) && raw[i] == 0xff {
				i++
			}
			if i >= len(raw) {
				break
			}
			marker := raw[i]
			i++
			if marker == 0xd8 || marker == 0xd9 || (marker >= 0xd0 && marker <= 0xd7) || marker == 0x01 {
				continue
			}
			if i+2 > len(raw) {
				break
			}
			segLen := int(binary.BigEndian.Uint16(raw[i : i+2]))
			if segLen < 2 {
				break
			}
			switch marker {
			case 0xc0, 0xc1, 0xc2, 0xc3, 0xc5, 0xc6, 0xc7, 0xc9, 0xca, 0xcb, 0xcd, 0xce, 0xcf:
				if i+7 <= len(raw) {
					h := int(binary.BigEndian.Uint16(raw[i+3 : i+5]))
					w := int(binary.BigEndian.Uint16(raw[i+5 : i+7]))
					return w, h
				}
				return 0, 0
			}
			i += segLen
		}
	}
	if len(raw) >= 30 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP" {
		chunkType := string(raw[12:16])
		switch chunkType {
		case "VP8 ":
			if raw[23] == 0x9d && raw[24] == 0x01 && raw[25] == 0x2a {
				w := int(binary.LittleEndian.Uint16(raw[26:28]) & 0x3fff)
				h := int(binary.LittleEndian.Uint16(raw[28:30]) & 0x3fff)
				return w, h
			}
		case "VP8L":
			bits := binary.LittleEndian.Uint32(raw[21:25])
			w := int(bits&0x3fff) + 1
			h := int((bits>>14)&0x3fff) + 1
			return w, h
		case "VP8X":
			w := int(raw[24]) | int(raw[25])<<8 | int(raw[26])<<16 + 1
			h := int(raw[27]) | int(raw[28])<<8 | int(raw[29])<<16 + 1
			return w, h
		}
	}
	return 0, 0
}

func parseBase64ImageDimensions(b64Data string) (int, int) {
	if raw := decodeBase64Prefix(b64Data, 65536); len(raw) > 0 {
		if w, h := parseImageDimensionsFromBytes(raw); w > 0 && h > 0 {
			return w, h
		}
	}
	if len(b64Data) > 65536 {
		if raw := decodeBase64Prefix(b64Data, 0); len(raw) > 0 {
			return parseImageDimensionsFromBytes(raw)
		}
	}
	return 0, 0
}

// geminiImageBaseTokensForDimensions returns Google's official base output token count
// for a generated image according to its pixel resolution tier (512 / 1K / 2K / 4K).
func geminiImageBaseTokensForDimensions(w, h int) int {
	if w <= 0 || h <= 0 {
		return 0
	}
	maxDim := w
	if h > maxDim {
		maxDim = h
	}
	switch {
	case maxDim <= 640:
		return 747
	case maxDim <= 1536:
		return 1120
	case maxDim <= 3072:
		return 1680
	default:
		return 2520
	}
}

func geminiPartInlineImageBaseTokens(part *dto.GeminiPart) int {
	if part == nil || part.InlineData == nil || part.InlineData.Data == "" {
		return 0
	}
	mime := strings.ToLower(strings.TrimSpace(part.InlineData.MimeType))
	if mime != "" && !strings.HasPrefix(mime, "image/") {
		return 0
	}
	w, h := parseBase64ImageDimensions(part.InlineData.Data)
	return geminiImageBaseTokensForDimensions(w, h)
}

func geminiResponseInlineImageBaseTokens(response *dto.GeminiChatResponse) int {
	if response == nil {
		return 0
	}
	total := 0
	for i := range response.Candidates {
		for j := range response.Candidates[i].Content.Parts {
			total += geminiPartInlineImageBaseTokens(&response.Candidates[i].Content.Parts[j])
		}
	}
	return total
}

// ensureGeminiInlineImageUsageMetadata checks if the response contains generated inline images
// whose official resolution-tier base token count (747 / 1120 / 1680 / 2520) was omitted by a
// wrapped upstream (such as nano-banana-2 reporting only thinking tokens ~600-1100), and adds
// the missing base image tokens so usageMetadata and logs match official Gemini image token counts.
func ensureGeminiInlineImageUsageMetadata(response *dto.GeminiChatResponse) bool {
	if response == nil {
		return false
	}
	imgBaseTokens := geminiResponseInlineImageBaseTokens(response)
	if imgBaseTokens <= 0 {
		return false
	}
	if response.UsageMetadata.CandidatesTokenCount >= imgBaseTokens {
		return false
	}
	response.UsageMetadata.CandidatesTokenCount += imgBaseTokens
	total := response.UsageMetadata.PromptTokenCount +
		response.UsageMetadata.ToolUsePromptTokenCount +
		response.UsageMetadata.CandidatesTokenCount +
		response.UsageMetadata.ThoughtsTokenCount
	if total > response.UsageMetadata.TotalTokenCount {
		response.UsageMetadata.TotalTokenCount = total
	}
	response.HasUsageMetadata = true
	return true
}

func geminiResponseInlineImageCount(response *dto.GeminiChatResponse) int {
	if response == nil {
		return 0
	}
	count := 0
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && part.InlineData.MimeType != "" {
				count++
			}
		}
	}
	return count
}

func responseGeminiChat2OpenAI(c *gin.Context, response *dto.GeminiChatResponse) *dto.OpenAITextResponse {
	return relayconvert.ResponseGeminiChat2OpenAI(helper.GetResponseID(c), common.GetTimestamp(), response)
}

func streamResponseGeminiChat2OpenAI(geminiResponse *dto.GeminiChatResponse) (*dto.ChatCompletionsStreamResponse, bool) {
	return relayconvert.StreamResponseGeminiChat2OpenAI(geminiResponse)
}

func handleStream(c *gin.Context, info *relaycommon.RelayInfo, resp *dto.ChatCompletionsStreamResponse) error {
	streamData, err := common.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal stream response: %w", err)
	}
	err = openai.HandleStreamFormat(c, info, string(streamData), info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
	if err != nil {
		return fmt.Errorf("failed to handle stream format: %w", err)
	}
	return nil
}

func handleFinalStream(c *gin.Context, info *relaycommon.RelayInfo, resp *dto.ChatCompletionsStreamResponse) error {
	streamData, err := common.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal stream response: %w", err)
	}
	openai.HandleFinalResponse(c, info, string(streamData), resp.Id, resp.Created, resp.Model, resp.GetSystemFingerprint(), resp.Usage, false)
	return nil
}

func geminiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, callback func(data string, geminiResponse *dto.GeminiChatResponse) bool) (*dto.Usage, *types.NewAPIError) {
	var usage = &dto.Usage{}
	var imageCount int
	var imageBaseTokens int
	var hasBillableUsageMetadata bool
	var streamErr error
	var accumulatedUsageMetadata *dto.GeminiUsageMetadata
	responseText := strings.Builder{}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var geminiResponse dto.GeminiChatResponse
		if err := common.UnmarshalJsonStr(data, &geminiResponse); err != nil {
			streamErr = fmt.Errorf("unmarshal Gemini stream response: %w", err)
			sr.Stop(streamErr)
			return
		}

		if len(geminiResponse.Candidates) == 0 && geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != nil {
			blockReason := *geminiResponse.PromptFeedback.BlockReason
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, fmt.Sprintf("gemini_block_reason=%s", blockReason))
			if metadata := geminiResponse.GetUsageMetadata(); dto.HasGeminiUsageMetadataTokens(metadata) {
				accumulatedUsageMetadata = dto.MergeGeminiUsageMetadataNonZero(accumulatedUsageMetadata, metadata)
				mappedUsage := buildUsageFromGeminiMetadata(accumulatedUsageMetadata, info.GetEstimatePromptTokens())
				*usage = mappedUsage
				hasBillableUsageMetadata = true
			}
			refusalText := FormatGeminiBlockRefusalText(blockReason, nil)
			safetyReason := blockReason
			syntheticResp := &dto.GeminiChatResponse{
				Candidates: []dto.GeminiChatCandidate{
					{
						Content: dto.GeminiChatContent{
							Role: "model",
							Parts: []dto.GeminiPart{
								{Text: refusalText},
							},
						},
						FinishReason: &safetyReason,
					},
				},
				PromptFeedback: geminiResponse.PromptFeedback,
			}
			callback(data, syntheticResp)
			sr.Stop(nil)
			return
		}

		markGeminiGoogleSearchCall(c, &geminiResponse)
		countGeminiBillableFunctionCalls(info, &geminiResponse)

		if !strings.Contains(common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason), "gemini_block_reason=") {
			for i := range geminiResponse.Candidates {
				candidate := &geminiResponse.Candidates[i]
				if candidate.FinishReason != nil && IsGeminiBlockReason(*candidate.FinishReason) {
					reason := *candidate.FinishReason
					common.SetContextKey(c, constant.ContextKeyAdminRejectReason, fmt.Sprintf("gemini_block_reason=%s", reason))
					if len(candidate.Content.Parts) == 0 {
						candidate.Content.Role = "model"
						candidate.Content.Parts = []dto.GeminiPart{
							{Text: FormatGeminiBlockRefusalText(reason, candidate.FinishMessage)},
						}
					}
					break
				}
			}
		}

		// 统计图片数量与输出文本
		for i := range geminiResponse.Candidates {
			candidate := &geminiResponse.Candidates[i]
			for j := range candidate.Content.Parts {
				part := &candidate.Content.Parts[j]
				if part.InlineData != nil && part.InlineData.MimeType != "" {
					imageCount++
					imageBaseTokens += geminiPartInlineImageBaseTokens(part)
				}
				if part.Text != "" {
					responseText.WriteString(part.Text)
				}
				if part.FunctionCall != nil {
					responseText.WriteString(part.FunctionCall.FunctionName)
					if part.FunctionCall.Arguments != nil {
						if argsBytes, err := common.Marshal(part.FunctionCall.Arguments); err == nil {
							responseText.Write(argsBytes)
						}
					}
				}
				if part.ExecutableCode != nil {
					responseText.WriteString(part.ExecutableCode.Code)
				}
			}
		}

		// 更新使用量统计
		if metadata := geminiResponse.GetUsageMetadata(); dto.HasGeminiUsageMetadataTokens(metadata) {
			accumulatedUsageMetadata = dto.MergeGeminiUsageMetadataNonZero(accumulatedUsageMetadata, metadata)
			mappedUsage := buildUsageFromGeminiMetadata(accumulatedUsageMetadata, info.GetEstimatePromptTokens())
			*usage = mappedUsage
			hasBillableUsageMetadata = true
		}

		if !callback(data, &geminiResponse) {
			if isGeminiDownstreamStop(c, info) {
				sr.Stop(nil)
				return
			}
			streamErr = errors.New("Gemini stream callback stopped")
			sr.Stop(streamErr)
		}
	})

	if !hasBillableUsageMetadata {
		if info.ReceivedResponseCount > 0 {
			usage = service.ResponseText2Usage(c, responseText.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		} else {
			usage = &dto.Usage{
				PromptTokens: info.GetEstimatePromptTokens(),
				TotalTokens:  info.GetEstimatePromptTokens(),
			}
		}
		if imageBaseTokens > 0 && usage.CompletionTokens < imageBaseTokens {
			usage.CompletionTokens += imageBaseTokens
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
		} else if imageCount != 0 && usage.CompletionTokens == 0 {
			usage.CompletionTokens = imageCount * 1400
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
		}
		attachEstimatedGeminiBillingUsage(usage)
	} else {
		if imageBaseTokens > 0 && usage.CompletionTokens < imageBaseTokens {
			usage.CompletionTokens += imageBaseTokens
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			if usage.BillingUsage != nil {
				usage.BillingUsage = dto.CloneBillingUsageWithEstimatedCompletion(usage.BillingUsage, usage.CompletionTokens)
			}
		}
		patchGeminiZeroCompletionUsage(c, info, usage, responseText.String(), imageCount)
	}

	if strings.Contains(common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason), "gemini_block_reason=") {
		zeroBillingUsageCompletion(usage)
	}

	if streamErr != nil {
		return usage, types.NewOpenAIError(streamErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if info.StreamStatus != nil && !info.StreamStatus.IsNormalEnd() {
		logger.LogWarn(c, fmt.Sprintf("Gemini stream ended unexpectedly: %s", info.StreamStatus.Summary()))
	}

	return usage, nil
}

func isGeminiDownstreamStop(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
		return true
	}
	return info != nil && info.StreamStatus != nil &&
		info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone
}

func GeminiChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	id := helper.GetResponseID(c)
	createAt := common.GetTimestamp()
	finishReason := constant.FinishReasonStop
	toolCallIndexByChoice := make(map[int]map[string]int)
	nextToolCallIndexByChoice := make(map[int]int)
	var responseTextBuilder strings.Builder

	usage, err := geminiStreamHandler(c, info, resp, func(data string, geminiResponse *dto.GeminiChatResponse) bool {
		response, isStop := streamResponseGeminiChat2OpenAI(geminiResponse)

		response.Id = id
		response.Created = createAt
		response.Model = info.UpstreamModelName
		if response.IsToolCall() {
			finishReason = constant.FinishReasonToolCalls
			if info.RelayFormat == types.RelayFormatClaude {
				for choiceIdx := range response.Choices {
					response.Choices[choiceIdx].FinishReason = nil
				}
			}
		}
		for choiceIdx := range response.Choices {
			choiceKey := response.Choices[choiceIdx].Index
			if str := response.Choices[choiceIdx].Delta.GetContentString(); str != "" {
				responseTextBuilder.WriteString(str)
			}
			if rc := response.Choices[choiceIdx].Delta.GetReasoningContent(); rc != "" {
				responseTextBuilder.WriteString(rc)
			}
			for toolIdx := range response.Choices[choiceIdx].Delta.ToolCalls {
				tool := &response.Choices[choiceIdx].Delta.ToolCalls[toolIdx]
				if tool.ID == "" {
					continue
				}
				m := toolCallIndexByChoice[choiceKey]
				if m == nil {
					m = make(map[string]int)
					toolCallIndexByChoice[choiceKey] = m
				}
				if idx, ok := m[tool.ID]; ok {
					tool.SetIndex(idx)
					continue
				}
				idx := nextToolCallIndexByChoice[choiceKey]
				nextToolCallIndexByChoice[choiceKey] = idx + 1
				m[tool.ID] = idx
				tool.SetIndex(idx)
			}
		}

		logger.LogDebug(c, "info.SendResponseCount = %d", info.SendResponseCount)
		if info.SendResponseCount == 0 {
			// send first response
			emptyResponse := helper.GenerateStartEmptyResponse(id, createAt, info.UpstreamModelName, nil)
			// Claude message_start is emitted from this first OpenAI chunk.
			// Carry upstream usage when the current Gemini frame provided it.
			emptyResponse.Usage = response.Usage
			if response.IsToolCall() {
				if len(emptyResponse.Choices) > 0 && len(response.Choices) > 0 {
					toolCalls := response.Choices[0].Delta.ToolCalls
					copiedToolCalls := make([]dto.ToolCallResponse, len(toolCalls))
					for idx := range toolCalls {
						copiedToolCalls[idx] = toolCalls[idx]
						copiedToolCalls[idx].Function.Arguments = ""
					}
					emptyResponse.Choices[0].Delta.ToolCalls = copiedToolCalls
				}
				finishReason = constant.FinishReasonToolCalls
				err := handleStream(c, info, emptyResponse)
				if err != nil {
					logger.LogError(c, err.Error())
				}

				response.ClearToolCalls()
				if response.IsFinished() {
					response.Choices[0].FinishReason = nil
				}
			} else {
				err := handleStream(c, info, emptyResponse)
				if err != nil {
					logger.LogError(c, err.Error())
				}
			}
		}

		info.MarkStreamContentSent()
		err := handleStream(c, info, response)
		if err != nil {
			logger.LogError(c, err.Error())
		}
		if isStop {
			if info.RelayFormat != types.RelayFormatClaude {
				_ = handleStream(c, info, helper.GenerateStopResponse(id, createAt, info.UpstreamModelName, finishReason))
			}
		}
		return true
	})

	if err != nil {
		return usage, err
	}

	if !info.HasStreamContentSent() && strings.TrimSpace(responseTextBuilder.String()) == "" && len(toolCallIndexByChoice) == 0 && !strings.Contains(common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason), "gemini_block_reason=") {
		info.ReceivedResponseCount = 0
		statusCode := http.StatusBadGateway
		errMsg := "upstream gemini returned empty stream response"
		if resp != nil && resp.Request != nil && resp.Request.Context().Err() != nil && (c == nil || c.Request == nil || c.Request.Context().Err() == nil) {
			errMsg = "upstream stream first-byte timeout"
			statusCode = http.StatusGatewayTimeout
		}
		logger.LogWarn(c, fmt.Sprintf("%s before writing to client, req_id=%s, model=%s", errMsg, c.GetString(common.RequestIdKey), info.UpstreamModelName))
		return nil, types.NewOpenAIError(
			fmt.Errorf("%s", errMsg),
			types.ErrorCodeEmptyResponse,
			statusCode,
		)
	}

	response := helper.GenerateFinalUsageResponse(id, createAt, info.UpstreamModelName, *usage)
	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil && !info.ClaudeConvertInfo.Done {
		response = helper.GenerateStopResponse(id, createAt, info.UpstreamModelName, finishReason)
		response.Usage = usage
	}
	handleErr := handleFinalStream(c, info, response)
	if handleErr != nil {
		common.SysLog("send final response failed: " + handleErr.Error())
	}
	return usage, nil
}

func GeminiChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	logger.LogDebug(c, "Gemini response body: %s", responseBody)
	var geminiResponse dto.GeminiChatResponse
	err = common.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	markGeminiGoogleSearchCall(c, &geminiResponse)
	countGeminiBillableFunctionCalls(info, &geminiResponse)
	if len(geminiResponse.Candidates) == 0 {
		if geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != nil {
			blockReason := *geminiResponse.PromptFeedback.BlockReason
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, fmt.Sprintf("gemini_block_reason=%s", blockReason))
			refusalText := fmt.Sprintf("抱歉，由于触发了 Google Gemini 内容安全审查策略 (%s)，请求已被拦截，无法生成回答。请调整您的提示词后重试。", blockReason)
			safetyReason := "SAFETY"
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
		} else {
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_empty_candidates")
			newAPIError := types.NewOpenAIError(
				errors.New("empty response from Gemini API"),
				types.ErrorCodeEmptyResponse,
				http.StatusBadGateway,
			)

			service.ResetStatusCode(newAPIError, c.GetString("status_code_mapping"))
			return nil, newAPIError
		}
	}
	if !strings.Contains(common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason), "gemini_block_reason=") {
		for i := range geminiResponse.Candidates {
			candidate := &geminiResponse.Candidates[i]
			if candidate.FinishReason != nil && IsGeminiBlockReason(*candidate.FinishReason) {
				reason := *candidate.FinishReason
				common.SetContextKey(c, constant.ContextKeyAdminRejectReason, fmt.Sprintf("gemini_block_reason=%s", reason))
				if len(candidate.Content.Parts) == 0 {
					candidate.Content.Role = "model"
					candidate.Content.Parts = []dto.GeminiPart{
						{Text: FormatGeminiBlockRefusalText(reason, candidate.FinishMessage)},
					}
				}
				break
			}
		}
	}
	fullTextResponse := responseGeminiChat2OpenAI(c, &geminiResponse)
	fullTextResponse.Model = info.UpstreamModelName
	usage := buildUsageFromGeminiResponse(c, info, &geminiResponse)
	if strings.Contains(common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason), "gemini_block_reason=") {
		zeroBillingUsageCompletion(&usage)
	}

	if usage.CompletionTokens == 0 {
		logger.LogWarn(c, fmt.Sprintf("GEMINI_ZERO_TOKEN_DEBUG: req_id=%s, model=%s, candidates_count=%d, has_usage=%v, prompt_tok=%d, cand_tok=%d, body=%s",
			c.GetString(common.RequestIdKey), info.UpstreamModelName, len(geminiResponse.Candidates),
			geminiResponse.HasUsageMetadata, geminiResponse.UsageMetadata.PromptTokenCount, geminiResponse.UsageMetadata.CandidatesTokenCount,
			string(responseBody)))
	}

	fullTextResponse.Usage = usage

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		responseBody, err = common.Marshal(fullTextResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	case types.RelayFormatClaude:
		convertResult, err := service.ConvertResponse(c, info, types.RelayFormatClaude, fullTextResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		claudeRespStr, err := common.Marshal(convertResult.Value)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = claudeRespStr
	case types.RelayFormatGemini:
		break
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &usage, nil
}

func GeminiEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewOpenAIError(readErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	var geminiResponse dto.GeminiBatchEmbeddingResponse
	if jsonErr := common.Unmarshal(responseBody, &geminiResponse); jsonErr != nil {
		return nil, types.NewOpenAIError(jsonErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// convert to openai format response
	openAIResponse := dto.OpenAIEmbeddingResponse{
		Object: "list",
		Data:   make([]dto.OpenAIEmbeddingResponseItem, 0, len(geminiResponse.Embeddings)),
		Model:  info.UpstreamModelName,
	}

	for i, embedding := range geminiResponse.Embeddings {
		openAIResponse.Data = append(openAIResponse.Data, dto.OpenAIEmbeddingResponseItem{
			Object:    "embedding",
			Embedding: embedding.Values,
			Index:     i,
		})
	}

	// calculate usage
	// https://ai.google.dev/gemini-api/docs/pricing?hl=zh-cn#text-embedding-004
	// Google has not yet clarified how embedding models will be billed
	// refer to openai billing method to use input tokens billing
	// https://platform.openai.com/docs/guides/embeddings#what-are-embeddings
	usage := service.ResponseText2Usage(c, "", info.UpstreamModelName, info.GetEstimatePromptTokens())
	openAIResponse.Usage = *usage

	jsonResponse, jsonErr := common.Marshal(openAIResponse)
	if jsonErr != nil {
		return nil, types.NewOpenAIError(jsonErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, jsonResponse)
	return usage, nil
}

func GeminiImageHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewOpenAIError(readErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, types.NewOpenAIError(fmt.Errorf("upstream returned error (status %d): %s", resp.StatusCode, string(responseBody)), types.ErrorCodeBadResponseBody, resp.StatusCode)
	}

	openAIResponse := dto.ImageResponse{
		Created: common.GetTimestamp(),
		Data:    make([]dto.ImageData, 0),
	}

	var geminiResponse dto.GeminiImageResponse
	if err := common.Unmarshal(responseBody, &geminiResponse); err == nil && len(geminiResponse.Predictions) > 0 {
		for _, prediction := range geminiResponse.Predictions {
			if prediction.RaiFilteredReason != "" {
				continue // skip filtered image
			}
			openAIResponse.Data = append(openAIResponse.Data, dto.ImageData{
				B64Json: prediction.BytesBase64Encoded,
			})
		}
	}

	if len(openAIResponse.Data) == 0 {
		var geminiChatResponse dto.GeminiChatResponse
		if err := common.Unmarshal(responseBody, &geminiChatResponse); err == nil && len(geminiChatResponse.Candidates) > 0 {
			for _, candidate := range geminiChatResponse.Candidates {
				for _, part := range candidate.Content.Parts {
					if part.InlineData != nil && part.InlineData.Data != "" {
						openAIResponse.Data = append(openAIResponse.Data, dto.ImageData{
							B64Json: part.InlineData.Data,
						})
					}
				}
			}
		}
	}

	if len(openAIResponse.Data) == 0 {
		return nil, types.NewOpenAIError(fmt.Errorf("no images generated: %s", string(responseBody)), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	jsonResponse, jsonErr := common.Marshal(openAIResponse)
	if jsonErr != nil {
		return nil, types.NewError(jsonErr, types.ErrorCodeBadResponseBody)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)

	// https://github.com/google-gemini/cookbook/blob/719a27d752aac33f39de18a8d3cb42a70874917e/quickstarts/Counting_Tokens.ipynb
	// each image has fixed 258 tokens
	const imageTokens = 258
	generatedImages := len(openAIResponse.Data)

	usage := &dto.Usage{
		PromptTokens:     imageTokens * generatedImages, // each generated image has fixed 258 tokens
		CompletionTokens: 0,                             // image generation does not calculate completion tokens
		TotalTokens:      imageTokens * generatedImages,
	}

	return usage, nil
}

type GeminiModelsResponse struct {
	Models        []dto.GeminiModel `json:"models"`
	NextPageToken string            `json:"nextPageToken"`
}

func FetchGeminiModels(baseURL, apiKey, proxyURL string) ([]string, error) {
	client, err := service.GetHttpClientWithProxy(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP客户端失败: %v", err)
	}

	allModels := make([]string, 0)
	nextPageToken := ""
	maxPages := 100 // Safety limit to prevent infinite loops

	for range maxPages {
		url := fmt.Sprintf("%s/v1beta/models", baseURL)
		if nextPageToken != "" {
			url = fmt.Sprintf("%s?pageToken=%s", url, nextPageToken)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		request, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("创建请求失败: %v", err)
		}

		request.Header.Set("x-goog-api-key", apiKey)

		response, err := client.Do(request)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("请求失败: %v", err)
		}

		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			cancel()
			return nil, fmt.Errorf("服务器返回错误 %d: %s", response.StatusCode, string(body))
		}

		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		cancel()
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %v", err)
		}

		var modelsResponse GeminiModelsResponse
		if err = common.Unmarshal(body, &modelsResponse); err != nil {
			return nil, fmt.Errorf("解析响应失败: %v", err)
		}

		for _, model := range modelsResponse.Models {
			modelNameValue, ok := model.Name.(string)
			if !ok {
				continue
			}
			modelName := strings.TrimPrefix(modelNameValue, "models/")
			allModels = append(allModels, modelName)
		}

		nextPageToken = modelsResponse.NextPageToken
		if nextPageToken == "" {
			break
		}
	}

	return allModels, nil
}
