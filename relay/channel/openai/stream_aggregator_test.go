package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIStreamAggregator_TextAndReasoning(t *testing.T) {
	agg := NewOpenAIStreamAggregator()

	// Chunk 1: Role and initial reasoning
	agg.Feed(&dto.ChatCompletionsStreamResponse{
		Id:      "chatcmpl-test",
		Model:   "deepseek-r1",
		Created: 1234567890,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Role:             "assistant",
					ReasoningContent: kitutil.GetPointer("Thinking about "),
				},
			},
		},
	})

	// Chunk 2: More reasoning
	agg.Feed(&dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					ReasoningContent: kitutil.GetPointer("math..."),
				},
			},
		},
	})

	// Chunk 3: Content starts
	agg.Feed(&dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Content: kitutil.GetPointer("The answer "),
				},
			},
		},
	})

	// Chunk 4: Content ends with finish_reason stop
	stopReason := "stop"
	agg.Feed(&dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index:        0,
				FinishReason: &stopReason,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Content: kitutil.GetPointer("is 42."),
				},
			},
		},
		Usage: &dto.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	})

	// Case A: normal mode (ThinkingToContent = false)
	resp := agg.BuildResponse("fallback-id", "fallback-model", 0, nil, false)
	require.NotNil(t, resp)
	assert.Equal(t, "chatcmpl-test", resp.Id)
	assert.Equal(t, "deepseek-r1", resp.Model)
	assert.Equal(t, int64(1234567890), resp.Created)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
	assert.Equal(t, "The answer is 42.", resp.Choices[0].Message.StringContent())
	require.NotNil(t, resp.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "Thinking about math...", *resp.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
	assert.Equal(t, 30, resp.Usage.TotalTokens)

	// Case B: thinkingToContent = true
	respThink := agg.BuildResponse("fallback-id", "fallback-model", 0, nil, true)
	require.NotNil(t, respThink)
	assert.Nil(t, respThink.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "<think>\nThinking about math...\n</think>\nThe answer is 42.", respThink.Choices[0].Message.StringContent())
}

func TestOpenAIStreamAggregator_ToolCalls(t *testing.T) {
	agg := NewOpenAIStreamAggregator()

	// Chunk 1: Start tool call 0 (get_weather)
	agg.Feed(&dto.ChatCompletionsStreamResponse{
		Id:      "chatcmpl-tool",
		Model:   "gpt-4o",
		Created: 1234567890,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Role: "assistant",
					ToolCalls: []dto.ToolCallResponse{
						{
							Index: kitutil.GetPointer(0),
							ID:    "call_weather_1",
							Type:  "function",
							Function: dto.FunctionResponse{
								Name:      "get_weather",
								Arguments: "{\"loc",
							},
						},
					},
				},
			},
		},
	})

	// Chunk 2: More arguments for tool call 0, and start tool call 1 (get_news)
	agg.Feed(&dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					ToolCalls: []dto.ToolCallResponse{
						{
							Index: kitutil.GetPointer(0),
							Function: dto.FunctionResponse{
								Arguments: "ation\": \"Beijing\"}",
							},
						},
						{
							Index: kitutil.GetPointer(1),
							ID:    "call_news_2",
							Type:  "function",
							Function: dto.FunctionResponse{
								Name:      "get_news",
								Arguments: "{\"topic\": \"AI\"}",
							},
						},
					},
				},
			},
		},
	})

	// Chunk 3: finish_reason tool_calls
	toolFinish := constant.FinishReasonToolCalls
	agg.Feed(&dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index:        0,
				FinishReason: &toolFinish,
			},
		},
	})

	resp := agg.BuildResponse("", "", 0, nil, false)
	require.NotNil(t, resp)
	require.Len(t, resp.Choices, 1)
	choice := resp.Choices[0]
	assert.Equal(t, constant.FinishReasonToolCalls, choice.FinishReason)
	assert.Nil(t, choice.Message.Content) // null content on pure tool_calls

	parsedTools := choice.Message.ParseToolCalls()
	require.Len(t, parsedTools, 2)

	assert.Equal(t, "call_weather_1", parsedTools[0].ID)
	assert.Equal(t, "get_weather", parsedTools[0].Function.Name)
	assert.Equal(t, `{"location": "Beijing"}`, parsedTools[0].Function.Arguments)

	assert.Equal(t, "call_news_2", parsedTools[1].ID)
	assert.Equal(t, "get_news", parsedTools[1].Function.Name)
	assert.Equal(t, `{"topic": "AI"}`, parsedTools[1].Function.Arguments)
}
