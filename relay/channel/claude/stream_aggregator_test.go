package claude

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeStreamAggregator_ThinkingTextAndToolUse(t *testing.T) {
	agg := NewClaudeStreamAggregator()

	// 1. message_start
	agg.Feed(&dto.ClaudeResponse{
		Type: "message_start",
		Message: &dto.ClaudeMediaMessage{
			Id:    "msg_claude_test",
			Model: "claude-3-7-sonnet-20250219",
			Role:  "assistant",
			Usage: &dto.ClaudeUsage{
				InputTokens:              100,
				CacheReadInputTokens:     20,
				CacheCreationInputTokens: 10,
			},
		},
	})

	// 2. content_block_start: thinking (block 0)
	agg.Feed(&dto.ClaudeResponse{
		Type:  "content_block_start",
		Index: kitutil.GetPointer(0),
		ContentBlock: &dto.ClaudeMediaMessage{
			Type: "thinking",
		},
	})

	// 3. content_block_delta: thinking delta
	agg.Feed(&dto.ClaudeResponse{
		Type:  "content_block_delta",
		Index: kitutil.GetPointer(0),
		Delta: &dto.ClaudeMediaMessage{
			Type:     "thinking_delta",
			Thinking: kitutil.GetPointer("Let me analyze the weather query."),
		},
	})

	// 4. content_block_delta: signature delta
	agg.Feed(&dto.ClaudeResponse{
		Type:  "content_block_delta",
		Index: kitutil.GetPointer(0),
		Delta: &dto.ClaudeMediaMessage{
			Type:      "signature_delta",
			Signature: "signature_xyz",
		},
	})

	// 5. content_block_stop: block 0
	agg.Feed(&dto.ClaudeResponse{
		Type:  "content_block_stop",
		Index: kitutil.GetPointer(0),
	})

	// 6. content_block_start: text (block 1)
	agg.Feed(&dto.ClaudeResponse{
		Type:  "content_block_start",
		Index: kitutil.GetPointer(1),
		ContentBlock: &dto.ClaudeMediaMessage{
			Type: "text",
		},
	})

	// 7. content_block_delta: text delta
	agg.Feed(&dto.ClaudeResponse{
		Type:  "content_block_delta",
		Index: kitutil.GetPointer(1),
		Delta: &dto.ClaudeMediaMessage{
			Type: "text_delta",
			Text: kitutil.GetPointer("Checking the forecast now."),
		},
	})

	// 8. content_block_stop: block 1
	agg.Feed(&dto.ClaudeResponse{
		Type:  "content_block_stop",
		Index: kitutil.GetPointer(1),
	})

	// 9. content_block_start: tool_use (block 2)
	agg.Feed(&dto.ClaudeResponse{
		Type:  "content_block_start",
		Index: kitutil.GetPointer(2),
		ContentBlock: &dto.ClaudeMediaMessage{
			Type: "tool_use",
			Id:   "toolu_12345",
			Name: "get_weather",
		},
	})

	// 10. content_block_delta: input_json_delta
	agg.Feed(&dto.ClaudeResponse{
		Type:  "content_block_delta",
		Index: kitutil.GetPointer(2),
		Delta: &dto.ClaudeMediaMessage{
			Type:        "input_json_delta",
			PartialJson: kitutil.GetPointer("{\"city\": \"Tokyo\"}"),
		},
	})

	// 11. content_block_stop: block 2
	agg.Feed(&dto.ClaudeResponse{
		Type:  "content_block_stop",
		Index: kitutil.GetPointer(2),
	})

	// 12. message_delta: stop_reason tool_use
	stopReason := "tool_use"
	agg.Feed(&dto.ClaudeResponse{
		Type: "message_delta",
		Delta: &dto.ClaudeMediaMessage{
			StopReason: &stopReason,
		},
		Usage: &dto.ClaudeUsage{
			OutputTokens: 50,
		},
	})

	claudeInfo := &ClaudeResponseInfo{
		ResponseId:   "msg_claude_test",
		Model:        "claude-3-7-sonnet-20250219",
		ResponseText: strings.Builder{},
		Usage: &dto.Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         20,
				CachedCreationTokens: 10,
			},
		},
	}

	resp := agg.Build(claudeInfo, "claude-3-7-sonnet-20250219")
	require.NotNil(t, resp)
	assert.Equal(t, "msg_claude_test", resp.Id)
	assert.Equal(t, "claude-3-7-sonnet-20250219", resp.Model)
	assert.Equal(t, "tool_use", resp.StopReason)
	require.NotNil(t, resp.Usage)
	assert.Equal(t, 100, resp.Usage.InputTokens)
	assert.Equal(t, 50, resp.Usage.OutputTokens)
	assert.Equal(t, 20, resp.Usage.CacheReadInputTokens)
	assert.Equal(t, 10, resp.Usage.CacheCreationInputTokens)

	require.Len(t, resp.Content, 3)

	// Block 0: thinking
	assert.Equal(t, "thinking", resp.Content[0].Type)
	require.NotNil(t, resp.Content[0].Thinking)
	assert.Equal(t, "Let me analyze the weather query.", *resp.Content[0].Thinking)
	assert.Equal(t, "signature_xyz", resp.Content[0].Signature)

	// Block 1: text
	assert.Equal(t, "text", resp.Content[1].Type)
	require.NotNil(t, resp.Content[1].Text)
	assert.Equal(t, "Checking the forecast now.", *resp.Content[1].Text)

	// Block 2: tool_use
	assert.Equal(t, "tool_use", resp.Content[2].Type)
	assert.Equal(t, "toolu_12345", resp.Content[2].Id)
	assert.Equal(t, "get_weather", resp.Content[2].Name)
	require.NotNil(t, resp.Content[2].Input)
	inputBytes, _ := json.Marshal(resp.Content[2].Input)
	assert.JSONEq(t, `{"city":"Tokyo"}`, string(inputBytes))

	// Test conversion to OpenAI format
	oaiResp := ResponseClaude2OpenAI(resp)
	require.NotNil(t, oaiResp)
	require.Len(t, oaiResp.Choices, 1)
	oaiChoice := oaiResp.Choices[0]
	assert.Equal(t, "tool_calls", oaiChoice.FinishReason)
	assert.Equal(t, "Checking the forecast now.", oaiChoice.Message.StringContent())
	require.NotNil(t, oaiChoice.Message.ReasoningContent)
	assert.Equal(t, "Let me analyze the weather query.", *oaiChoice.Message.ReasoningContent)
	parsedTools := oaiChoice.Message.ParseToolCalls()
	require.Len(t, parsedTools, 1)
	assert.Equal(t, "toolu_12345", parsedTools[0].ID)
	assert.Equal(t, "get_weather", parsedTools[0].Function.Name)
	assert.JSONEq(t, `{"city":"Tokyo"}`, parsedTools[0].Function.Arguments)
}

func TestClaudeStreamAggregator_HasMeaningfulContent(t *testing.T) {
	// Case 1: Empty aggregator
	aggEmpty := NewClaudeStreamAggregator()
	assert.False(t, aggEmpty.HasMeaningfulContent())

	// Case 2: Aggregator fed empty text block
	aggWhitespace := NewClaudeStreamAggregator()
	aggWhitespace.Feed(&dto.ClaudeResponse{
		Type:  "content_block_start",
		Index: kitutil.GetPointer(0),
		ContentBlock: &dto.ClaudeMediaMessage{
			Type: "text",
			Text: kitutil.GetPointer("   \n\t  "),
		},
	})
	assert.False(t, aggWhitespace.HasMeaningfulContent())

	// Case 3: Aggregator with text
	aggText := NewClaudeStreamAggregator()
	aggText.Feed(&dto.ClaudeResponse{
		Type:  "content_block_start",
		Index: kitutil.GetPointer(0),
		ContentBlock: &dto.ClaudeMediaMessage{
			Type: "text",
			Text: kitutil.GetPointer("Hello from Claude"),
		},
	})
	assert.True(t, aggText.HasMeaningfulContent())

	// Case 4: Aggregator with thinking only
	aggThinking := NewClaudeStreamAggregator()
	aggThinking.Feed(&dto.ClaudeResponse{
		Type:  "content_block_start",
		Index: kitutil.GetPointer(0),
		ContentBlock: &dto.ClaudeMediaMessage{
			Type:     "thinking",
			Thinking: kitutil.GetPointer("Reasoning..."),
		},
	})
	assert.True(t, aggThinking.HasMeaningfulContent())

	// Case 5: Aggregator with tool_use only
	aggTool := NewClaudeStreamAggregator()
	aggTool.Feed(&dto.ClaudeResponse{
		Type:  "content_block_start",
		Index: kitutil.GetPointer(0),
		ContentBlock: &dto.ClaudeMediaMessage{
			Type: "tool_use",
			Id:   "toolu_123",
			Name: "calc",
		},
	})
	assert.True(t, aggTool.HasMeaningfulContent())
}
