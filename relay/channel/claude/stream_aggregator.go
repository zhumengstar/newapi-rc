package claude

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

type claudeBlockBuilder struct {
	blockType     string
	text          strings.Builder
	thinking      strings.Builder
	signature     string
	toolId        string
	toolName      string
	toolInputJson strings.Builder
	citations     []json.RawMessage
}

type ClaudeStreamAggregator struct {
	id         string
	model      string
	role       string
	stopReason string
	blocks     map[int]*claudeBlockBuilder
	blockOrder []int
}

func NewClaudeStreamAggregator() *ClaudeStreamAggregator {
	return &ClaudeStreamAggregator{
		blocks:     make(map[int]*claudeBlockBuilder),
		blockOrder: make([]int, 0),
	}
}

func (agg *ClaudeStreamAggregator) Feed(claudeResponse *dto.ClaudeResponse) {
	if claudeResponse == nil {
		return
	}
	if claudeResponse.StopReason != "" {
		agg.stopReason = claudeResponse.StopReason
	}
	if claudeResponse.Delta != nil {
		if claudeResponse.Delta.StopReason != nil && *claudeResponse.Delta.StopReason != "" {
			agg.stopReason = *claudeResponse.Delta.StopReason
		}
	}

	switch claudeResponse.Type {
	case "message_start":
		if claudeResponse.Message != nil {
			if claudeResponse.Message.Id != "" {
				agg.id = claudeResponse.Message.Id
			}
			if claudeResponse.Message.Model != "" {
				agg.model = claudeResponse.Message.Model
			}
			if claudeResponse.Message.Role != "" {
				agg.role = claudeResponse.Message.Role
			}
			if claudeResponse.Message.StopReason != nil && *claudeResponse.Message.StopReason != "" {
				agg.stopReason = *claudeResponse.Message.StopReason
			}
		}
	case "content_block_start":
		if claudeResponse.Index != nil && claudeResponse.ContentBlock != nil {
			idx := *claudeResponse.Index
			b := &claudeBlockBuilder{
				blockType: claudeResponse.ContentBlock.Type,
			}
			if b.blockType == "tool_use" {
				b.toolId = claudeResponse.ContentBlock.Id
				b.toolName = claudeResponse.ContentBlock.Name
			} else if b.blockType == "text" && claudeResponse.ContentBlock.Text != nil {
				b.text.WriteString(*claudeResponse.ContentBlock.Text)
			} else if b.blockType == "thinking" && claudeResponse.ContentBlock.Thinking != nil {
				b.thinking.WriteString(*claudeResponse.ContentBlock.Thinking)
			}
			if _, exists := agg.blocks[idx]; !exists {
				agg.blockOrder = append(agg.blockOrder, idx)
			}
			agg.blocks[idx] = b
		}
	case "content_block_delta":
		if claudeResponse.Index != nil && claudeResponse.Delta != nil {
			idx := *claudeResponse.Index
			b := agg.blocks[idx]
			if b == nil {
				b = &claudeBlockBuilder{}
				agg.blocks[idx] = b
				agg.blockOrder = append(agg.blockOrder, idx)
			}
			if claudeResponse.Delta.Text != nil {
				b.text.WriteString(*claudeResponse.Delta.Text)
			}
			if claudeResponse.Delta.Thinking != nil {
				b.thinking.WriteString(*claudeResponse.Delta.Thinking)
			}
			if claudeResponse.Delta.Signature != "" {
				b.signature = claudeResponse.Delta.Signature
			}
			if claudeResponse.Delta.PartialJson != nil {
				b.toolInputJson.WriteString(*claudeResponse.Delta.PartialJson)
			}
			if len(claudeResponse.Delta.Citation) > 0 {
				b.citations = append(b.citations, claudeResponse.Delta.Citation)
			}
		}
	}
}

func (agg *ClaudeStreamAggregator) Build(claudeInfo *ClaudeResponseInfo, defaultModel string) *dto.ClaudeResponse {
	respId := agg.id
	if respId == "" && claudeInfo != nil {
		respId = claudeInfo.ResponseId
	}
	model := agg.model
	if model == "" && claudeInfo != nil {
		model = claudeInfo.Model
	}
	if model == "" {
		model = defaultModel
	}
	role := agg.role
	if role == "" {
		role = "assistant"
	}
	stopReason := agg.stopReason
	if stopReason == "" {
		stopReason = "end_turn"
	}

	claudeResp := &dto.ClaudeResponse{
		Id:         respId,
		Type:       "message",
		Role:       role,
		Model:      model,
		StopReason: stopReason,
		Content:    make([]dto.ClaudeMediaMessage, 0, len(agg.blockOrder)),
	}

	hasToolUse := false
	for _, idx := range agg.blockOrder {
		b := agg.blocks[idx]
		if b == nil {
			continue
		}
		switch b.blockType {
		case "text":
			textStr := b.text.String()
			msg := dto.ClaudeMediaMessage{
				Type: "text",
				Text: &textStr,
			}
			if len(b.citations) > 0 {
				if citBytes, err := common.Marshal(b.citations); err == nil {
					msg.Citations = citBytes
				}
			}
			claudeResp.Content = append(claudeResp.Content, msg)
		case "thinking":
			thinkingStr := b.thinking.String()
			msg := dto.ClaudeMediaMessage{
				Type:      "thinking",
				Thinking:  &thinkingStr,
				Signature: b.signature,
			}
			claudeResp.Content = append(claudeResp.Content, msg)
		case "tool_use":
			hasToolUse = true
			var inputMap any
			jsonStr := strings.TrimSpace(b.toolInputJson.String())
			if jsonStr == "" {
				inputMap = map[string]any{}
			} else {
				if err := common.Unmarshal([]byte(jsonStr), &inputMap); err != nil {
					inputMap = map[string]any{}
				}
			}
			msg := dto.ClaudeMediaMessage{
				Type:  "tool_use",
				Id:    b.toolId,
				Name:  b.toolName,
				Input: inputMap,
			}
			claudeResp.Content = append(claudeResp.Content, msg)
		default:
			if b.text.Len() > 0 {
				textStr := b.text.String()
				claudeResp.Content = append(claudeResp.Content, dto.ClaudeMediaMessage{
					Type: "text",
					Text: &textStr,
				})
			}
		}
	}

	if len(claudeResp.Content) == 0 {
		textStr := ""
		if claudeInfo != nil {
			textStr = claudeInfo.ResponseText.String()
		}
		claudeResp.Content = append(claudeResp.Content, dto.ClaudeMediaMessage{
			Type: "text",
			Text: &textStr,
		})
	}

	if hasToolUse && (agg.stopReason == "" || agg.stopReason == "end_turn") {
		claudeResp.StopReason = "tool_use"
	}

	if claudeInfo != nil && claudeInfo.Usage != nil {
		claudeResp.Usage = &dto.ClaudeUsage{
			InputTokens:                 claudeInfo.Usage.PromptTokens,
			OutputTokens:                claudeInfo.Usage.CompletionTokens,
			CacheReadInputTokens:        claudeInfo.Usage.PromptTokensDetails.CachedTokens,
			CacheCreationInputTokens:    claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens,
			ClaudeCacheCreation5mTokens: claudeInfo.Usage.ClaudeCacheCreation5mTokens,
			ClaudeCacheCreation1hTokens: claudeInfo.Usage.ClaudeCacheCreation1hTokens,
		}
	}

	return claudeResp
}
