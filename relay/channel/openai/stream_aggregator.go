package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type openAIChoiceAggregator struct {
	index            int
	role             string
	content          strings.Builder
	reasoning        strings.Builder
	toolCallsByIndex map[int]*dto.ToolCallResponse
	toolCallOrder    []int
	finishReason     string
	annotations      json.RawMessage
}

type OpenAIStreamAggregator struct {
	id                string
	model             string
	created           int64
	systemFingerprint string
	choices           map[int]*openAIChoiceAggregator
	choiceOrder       []int
	usage             *dto.Usage
}

func NewOpenAIStreamAggregator() *OpenAIStreamAggregator {
	return &OpenAIStreamAggregator{
		choices:     make(map[int]*openAIChoiceAggregator),
		choiceOrder: make([]int, 0),
	}
}

func (agg *OpenAIStreamAggregator) Feed(streamResp *dto.ChatCompletionsStreamResponse) {
	if streamResp == nil {
		return
	}
	if streamResp.Id != "" {
		agg.id = streamResp.Id
	}
	if streamResp.Model != "" {
		agg.model = streamResp.Model
	}
	if streamResp.Created != 0 {
		agg.created = streamResp.Created
	}
	if streamResp.SystemFingerprint != nil && *streamResp.SystemFingerprint != "" {
		agg.systemFingerprint = *streamResp.SystemFingerprint
	}
	if streamResp.Usage != nil {
		agg.usage = dto.MergeUsageNonZero(agg.usage, streamResp.Usage)
	}

	for _, choice := range streamResp.Choices {
		idx := choice.Index
		ch, exists := agg.choices[idx]
		if !exists {
			ch = &openAIChoiceAggregator{
				index:            idx,
				role:             "assistant",
				toolCallsByIndex: make(map[int]*dto.ToolCallResponse),
			}
			agg.choices[idx] = ch
			agg.choiceOrder = append(agg.choiceOrder, idx)
		}
		if choice.Delta.Role != "" {
			ch.role = choice.Delta.Role
		}
		if choice.Delta.Content != nil {
			ch.content.WriteString(*choice.Delta.Content)
		}
		if rc := choice.Delta.GetReasoningContent(); rc != "" {
			ch.reasoning.WriteString(rc)
		}
		if len(choice.Delta.Annotations) > 0 {
			ch.annotations = choice.Delta.Annotations
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" && *choice.FinishReason != "null" {
			ch.finishReason = *choice.FinishReason
		}

		for i, tc := range choice.Delta.ToolCalls {
			toolIdx := i
			if tc.Index != nil {
				toolIdx = *tc.Index
			} else if tc.ID != "" {
				found := false
				for _, existingIdx := range ch.toolCallOrder {
					if existing := ch.toolCallsByIndex[existingIdx]; existing != nil && existing.ID == tc.ID {
						toolIdx = existingIdx
						found = true
						break
					}
				}
				if !found {
					toolIdx = len(ch.toolCallOrder)
				}
			} else {
				toolIdx = len(ch.toolCallOrder)
			}

			existing, ok := ch.toolCallsByIndex[toolIdx]
			if !ok {
				callType := tc.Type
				if callType == nil || callType == "" {
					callType = "function"
				}
				existing = &dto.ToolCallResponse{
					Index: kitutil.GetPointer(toolIdx),
					ID:    tc.ID,
					Type:  callType,
					Function: dto.FunctionResponse{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
				ch.toolCallsByIndex[toolIdx] = existing
				ch.toolCallOrder = append(ch.toolCallOrder, toolIdx)
			} else {
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Type != nil && tc.Type != "" {
					existing.Type = tc.Type
				}
				if tc.Function.Name != "" {
					existing.Function.Name += tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					existing.Function.Arguments += tc.Function.Arguments
				}
			}
		}
	}
}

func (agg *OpenAIStreamAggregator) BuildResponse(defaultId, defaultModel string, defaultCreated int64, finalUsage *dto.Usage, thinkingToContent bool) *dto.OpenAITextResponse {
	respId := agg.id
	if respId == "" {
		respId = defaultId
	}
	model := agg.model
	if model == "" {
		model = defaultModel
	}
	created := agg.created
	if created == 0 {
		created = defaultCreated
	}

	response := &dto.OpenAITextResponse{
		Id:      respId,
		Object:  "chat.completion",
		Created: created,
		Model:   model,
	}
	if finalUsage != nil {
		response.Usage = *finalUsage
	} else if agg.usage != nil {
		response.Usage = *agg.usage
	}

	if len(agg.choiceOrder) == 0 {
		response.Choices = []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: "",
				},
				FinishReason: constant.FinishReasonStop,
			},
		}
		return response
	}

	for _, idx := range agg.choiceOrder {
		ch := agg.choices[idx]
		if ch == nil {
			continue
		}
		choice := dto.OpenAITextResponseChoice{
			Index: ch.index,
			Message: dto.Message{
				Role: ch.role,
			},
			FinishReason: ch.finishReason,
		}
		if choice.Message.Role == "" {
			choice.Message.Role = "assistant"
		}

		contentStr := ch.content.String()
		reasoningStr := ch.reasoning.String()

		if thinkingToContent && reasoningStr != "" {
			contentStr = fmt.Sprintf("<think>\n%s\n</think>\n%s", reasoningStr, contentStr)
			choice.Message.ReasoningContent = nil
		} else if reasoningStr != "" {
			choice.Message.ReasoningContent = &reasoningStr
		}

		if len(ch.toolCallOrder) > 0 {
			var toolList []dto.ToolCallResponse
			for _, tIdx := range ch.toolCallOrder {
				if tc := ch.toolCallsByIndex[tIdx]; tc != nil {
					toolList = append(toolList, *tc)
				}
			}
			if len(toolList) > 0 {
				choice.Message.SetToolCalls(toolList)
				if choice.FinishReason == "" {
					choice.FinishReason = constant.FinishReasonToolCalls
				}
			}
		}

		if contentStr != "" {
			choice.Message.Content = contentStr
		} else if len(ch.toolCallOrder) > 0 {
			choice.Message.Content = nil
		} else {
			choice.Message.Content = ""
		}

		if len(ch.annotations) > 0 {
			choice.Message.Annotations = ch.annotations
		}

		if choice.FinishReason == "" {
			choice.FinishReason = constant.FinishReasonStop
		}

		response.Choices = append(response.Choices, choice)
	}

	return response
}

func SendNonStreamResponseFromOpenAI(c *gin.Context, info *relaycommon.RelayInfo, chatResponse *dto.OpenAITextResponse) *types.NewAPIError {
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		c.JSON(http.StatusOK, chatResponse)
		return nil
	case types.RelayFormatClaude:
		convertResult, err := service.ConvertResponse(c, info, types.RelayFormatClaude, chatResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		c.JSON(http.StatusOK, convertResult.Value)
		return nil
	case types.RelayFormatGemini:
		convertResult, err := service.ConvertResponse(c, info, types.RelayFormatGemini, chatResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		c.JSON(http.StatusOK, convertResult.Value)
		return nil
	case types.RelayFormatOpenAIResponses:
		convertResult, err := service.ConvertResponse(c, info, types.RelayFormatOpenAIResponses, chatResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		c.JSON(http.StatusOK, convertResult.Value)
		return nil
	default:
		c.JSON(http.StatusOK, chatResponse)
		return nil
	}
}
