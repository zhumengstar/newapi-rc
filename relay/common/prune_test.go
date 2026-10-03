package common

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func TestParseUniversalContextLimit(t *testing.T) {
	testCases := []struct {
		name       string
		errMsg     string
		wantLimit  int
		wantExceed bool
	}{
		{
			name:       "Gemini 1M with parenthesis",
			errMsg:     "status_code=400, The input token count exceeds the maximum number of tokens allowed (1048576).",
			wantLimit:  1048576,
			wantExceed: true,
		},
		{
			name:       "Gemini 128k without parenthesis",
			errMsg:     "status_code=400, The input token count exceeds the maximum number of tokens allowed 131072.",
			wantLimit:  131072,
			wantExceed: true,
		},
		{
			name:       "OpenAI maximum context length",
			errMsg:     "This model's maximum context length is 128000 tokens. However, your messages resulted in 130000 tokens.",
			wantLimit:  128000,
			wantExceed: true,
		},
		{
			name:       "Claude prompt too long",
			errMsg:     "prompt is too long: 205000 tokens > 200000 maximum",
			wantLimit:  200000,
			wantExceed: true,
		},
		{
			name:       "Generic keyword without number",
			errMsg:     "Error code: 400 - {'error': {'message': 'context_length_exceeded', 'type': 'invalid_request_error'}}",
			wantLimit:  0,
			wantExceed: true,
		},
		{
			name:       "Unrelated error",
			errMsg:     "status_code=401, Invalid API key provided.",
			wantLimit:  0,
			wantExceed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			limit, exceeded := ParseUniversalContextLimit(tc.errMsg)
			assert.Equal(t, tc.wantExceed, exceeded)
			assert.Equal(t, tc.wantLimit, limit)
		})
	}
}

func TestAutoPruneOpenAIRequest(t *testing.T) {
	longText := strings.Repeat("hello world this is a long text paragraph for testing ", 100) // ~5400 chars, ~1800 tokens

	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-4o",
		Messages: []dto.Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: longText},
			{Role: "assistant", Content: longText},
			{Role: "user", Content: longText},
			{Role: "assistant", Content: longText},
			{Role: "user", Content: "Latest question: what is the weather?"},
		},
	}

	// 限制为 2500 tokens，原始总计约为 7200 tokens
	pruned := AutoPruneOpenAIRequest(nil, nil, req, 2500)
	assert.True(t, pruned)

	// 验证 system 提示词被保留
	assert.Equal(t, "system", req.Messages[0].Role)
	assert.Equal(t, "You are a helpful assistant.", req.Messages[0].Content)

	// 验证最新的一条提问被保留
	lastMsg := req.Messages[len(req.Messages)-1]
	assert.Equal(t, "user", lastMsg.Role)
	assert.Equal(t, "Latest question: what is the weather?", lastMsg.Content)

	// 验证第一条非 system 消息必须为 user
	assert.Equal(t, "user", req.Messages[1].Role)

	// 验证总消息数已被裁剪
	assert.Less(t, len(req.Messages), 6)
}

func TestAutoPruneClaudeRequest(t *testing.T) {
	longText := strings.Repeat("claude multi turn test conversation snippet here ", 100)

	req := &dto.ClaudeRequest{
		Model:  "claude-3-7-sonnet-20250219",
		System: "System instructions for claude.",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: longText},
			{Role: "assistant", Content: longText},
			{Role: "user", Content: longText},
			{Role: "assistant", Content: longText},
			{Role: "user", Content: "Claude latest query"},
		},
	}

	// 限制为 2500 tokens
	pruned := AutoPruneClaudeRequest(nil, nil, req, 2500)
	assert.True(t, pruned)

	// 验证系统指令未被改变
	assert.Equal(t, "System instructions for claude.", req.System)

	// 验证最新提问未被裁掉
	lastMsg := req.Messages[len(req.Messages)-1]
	assert.Equal(t, "user", lastMsg.Role)
	assert.Equal(t, "Claude latest query", lastMsg.Content)

	// 验证首条消息仍为 user
	assert.Equal(t, "user", req.Messages[0].Role)

	// 验证消息数量确实减少
	assert.Less(t, len(req.Messages), 5)
}

func TestAutoPruneGeminiChatRequest(t *testing.T) {
	longText := strings.Repeat("gemini multi turn test conversation snippet here ", 100)

	req := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{Role: "user", Parts: []dto.GeminiPart{{Text: longText}}},
			{Role: "model", Parts: []dto.GeminiPart{{Text: longText}}},
			{Role: "user", Parts: []dto.GeminiPart{{Text: longText}}},
			{Role: "model", Parts: []dto.GeminiPart{{Text: longText}}},
			{Role: "user", Parts: []dto.GeminiPart{{Text: "Gemini latest query"}}},
		},
	}

	pruned := AutoPruneGeminiChatRequest(nil, nil, req, 2500)
	assert.True(t, pruned)

	// 验证最新提问未被裁掉
	lastMsg := req.Contents[len(req.Contents)-1]
	assert.Equal(t, "user", lastMsg.Role)
	assert.Equal(t, "Gemini latest query", lastMsg.Parts[0].Text)

	// 验证首条消息仍为 user
	assert.Equal(t, "user", req.Contents[0].Role)
	assert.Less(t, len(req.Contents), 5)
}
