package gemini

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamResponseGeminiChat2OpenAIAttachesUsageMetadata(t *testing.T) {
	t.Parallel()

	withUsage, isStop := streamResponseGeminiChat2OpenAI(&dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{{
			Content: dto.GeminiChatContent{
				Role:  "model",
				Parts: []dto.GeminiPart{{Text: "hello"}},
			},
		}},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:     3868,
			CandidatesTokenCount: 0,
			TotalTokenCount:      3868,
		},
	})
	require.False(t, isStop)
	require.NotNil(t, withUsage)
	require.NotNil(t, withUsage.Usage)
	require.Equal(t, 3868, withUsage.Usage.PromptTokens)
	require.Equal(t, 3868, withUsage.Usage.TotalTokens)
	require.NotNil(t, withUsage.Usage.BillingUsage)
	require.Equal(t, dto.BillingUsageSourceGeminiChat, withUsage.Usage.BillingUsage.Source)
	require.Equal(t, dto.BillingUsageSemanticGemini, withUsage.Usage.BillingUsage.Semantic)
	require.NotNil(t, withUsage.Usage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, 3868, withUsage.Usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
	require.False(t, withUsage.Usage.BillingUsage.Estimated)

	withoutUsage, _ := streamResponseGeminiChat2OpenAI(&dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{{
			Content: dto.GeminiChatContent{
				Role:  "model",
				Parts: []dto.GeminiPart{{Text: "hello"}},
			},
		}},
	})
	require.NotNil(t, withoutUsage)
	require.Nil(t, withoutUsage.Usage)
}

func TestGeminiChatStreamHandlerClaudeFirstFrameUsesUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatClaude,
		OriginModelName: "gemini-2.5-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-2.5-flash",
		},
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{
			LastMessagesType: relaycommon.LastMessageTypeNone,
		},
	}
	info.SetEstimatePromptTokens(4994)

	chunkData, err := common.Marshal(dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{{
			Content: dto.GeminiChatContent{
				Role:  "model",
				Parts: []dto.GeminiPart{{Text: "hello"}},
			},
		}},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount: 3868,
			TotalTokenCount:  3868,
		},
	})
	require.NoError(t, err)
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader([]byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n"))),
	}

	usage, newAPIError := GeminiChatStreamHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 3868, usage.PromptTokens)

	var startUsage, deltaUsage *dto.ClaudeUsage
	for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
		payload, ok := strings.CutPrefix(strings.TrimSpace(line), "data: ")
		if !ok {
			continue
		}
		var event dto.ClaudeResponse
		if err := common.UnmarshalJsonStr(payload, &event); err != nil {
			continue
		}
		switch event.Type {
		case "message_start":
			if event.Message != nil {
				startUsage = event.Message.Usage
			}
		case "message_delta":
			deltaUsage = event.Usage
		}
	}

	require.NotNil(t, startUsage)
	require.Equal(t, 3868, startUsage.InputTokens)
	require.NotNil(t, startUsage.BillingUsage)
	require.Equal(t, dto.BillingUsageSourceGeminiChat, startUsage.BillingUsage.Source)
	require.Equal(t, dto.BillingUsageSemanticGemini, startUsage.BillingUsage.Semantic)
	require.NotNil(t, startUsage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, 3868, startUsage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
	require.False(t, startUsage.BillingUsage.Estimated)

	require.NotNil(t, deltaUsage)
	require.Equal(t, 3868, deltaUsage.InputTokens)
	require.NotNil(t, deltaUsage.BillingUsage)
	require.Equal(t, dto.BillingUsageSourceGeminiChat, deltaUsage.BillingUsage.Source)
	require.Equal(t, dto.BillingUsageSemanticGemini, deltaUsage.BillingUsage.Semantic)
	require.NotNil(t, deltaUsage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, 3868, deltaUsage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
}

func TestGeminiChatHandlerCompletionTokensExcludeToolUsePromptTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiStreamHandlerCompletionTokensExcludeToolUsePromptTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	chunk := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "partial"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	chunkData, err := common.Marshal(chunk)
	require.NoError(t, err)

	streamBody := []byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiTextGenerationHandlerPromptTokensIncludeToolUsePromptTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash-preview:generateContent", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiTextGenerationHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiChatHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}

func TestGeminiStreamHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	chunk := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "partial"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	chunkData, err := common.Marshal(chunk)
	require.NoError(t, err)

	streamBody := []byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}

func TestGeminiTextGenerationHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash-preview:generateContent", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiTextGenerationHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}

func TestGeminiChatHandlerMissingUsageMetadataBuildsEstimatedBillingUsage(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	body := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}`)
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
	require.Equal(t, dto.BillingUsageSourceGeminiChat, usage.BillingUsage.Source)
	require.Equal(t, dto.BillingUsageSemanticGemini, usage.BillingUsage.Semantic)
	require.NotNil(t, usage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, usage.PromptTokens, usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
	require.Equal(t, usage.CompletionTokens, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
}

func TestGeminiStreamHandlerPromptOnlyUsageMetadataEstimatesCompletionTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	// Simulates a client aborting the stream before the final chunk: text was
	// streamed but the last observed usageMetadata only carries prompt tokens.
	chunk := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "partial streamed answer before disconnect"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount: 151,
			TotalTokenCount:  151,
		},
	}

	chunkData, err := common.Marshal(chunk)
	require.NoError(t, err)

	streamBody := []byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 151, usage.PromptTokens)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
	require.NotNil(t, usage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, usage.CompletionTokens, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
}

func TestGeminiChatHandlerPromptOnlyUsageMetadataEstimatesCompletionTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "answer text without candidate token count"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount: 151,
			TotalTokenCount:  151,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 151, usage.PromptTokens)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
}

func TestGeminiStreamHandlerEmptyUsageMetadataBuildsEstimatedBillingUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	streamBody := []byte("data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"partial\"}]}}],\"usageMetadata\":{}}\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
	require.Equal(t, dto.BillingUsageSourceGeminiChat, usage.BillingUsage.Source)
	require.NotNil(t, usage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, usage.PromptTokens, usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
	require.Equal(t, usage.CompletionTokens, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
}

func TestGeminiChatHandler_ProhibitedContent_GracefulRefusal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAI,
		OriginModelName: "gemini-3-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash",
		},
	}

	respBody := `{"candidates":[],"promptFeedback":{"blockReason":"PROHIBITED_CONTENT","safetyRatings":[{"category":"HARM_CATEGORY_HARASSMENT","probability":"HIGH"}]}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(respBody))),
	}

	usage, newApiErr := GeminiChatHandler(c, info, resp)
	require.Nil(t, newApiErr)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.CompletionTokens)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "PROHIBITED_CONTENT")
	require.Contains(t, w.Body.String(), "content_filter")
	require.Equal(t, "gemini_block_reason=PROHIBITED_CONTENT", common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason))
}

func TestGeminiTextGenerationHandler_ProhibitedContent_NativePreserve(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash:generateContent", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash",
		},
	}

	respBody := `{"candidates":[],"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(respBody))),
	}

	usage, newApiErr := GeminiTextGenerationHandler(c, info, resp)
	require.Nil(t, newApiErr)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.CompletionTokens)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "PROHIBITED_CONTENT")
	require.Equal(t, "gemini_block_reason=PROHIBITED_CONTENT", common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason))
}

func TestGeminiStreamHandler_ProhibitedContent_GracefulStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash",
		},
	}

	streamBody := []byte("data: {\"candidates\":[],\"promptFeedback\":{\"blockReason\":\"PROHIBITED_CONTENT\"}}\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	called := false
	usage, newApiErr := geminiStreamHandler(c, info, resp, func(data string, r *dto.GeminiChatResponse) bool {
		called = true
		require.Len(t, r.Candidates, 1)
		require.Contains(t, r.Candidates[0].Content.Parts[0].Text, "PROHIBITED_CONTENT")
		require.Equal(t, "SAFETY", *r.Candidates[0].FinishReason)
		return true
	})
	require.Nil(t, newApiErr)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.CompletionTokens)
	require.True(t, called)
	require.Equal(t, "gemini_block_reason=PROHIBITED_CONTENT", common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason))
}

func TestGeminiResponsesHandler_ProhibitedContent_GracefulRefusal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAIResponses,
		OriginModelName: "gemini-3-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash",
		},
	}

	respBody := `{"candidates":[],"promptFeedback":{"blockReason":"PROHIBITED_CONTENT","safetyRatings":[{"category":"HARM_CATEGORY_HARASSMENT","probability":"HIGH"}]}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(respBody))),
	}

	usage, newApiErr := GeminiResponsesHandler(c, info, resp)
	require.Nil(t, newApiErr)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.CompletionTokens)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "PROHIBITED_CONTENT")
	require.Equal(t, "gemini_block_reason=PROHIBITED_CONTENT", common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason))
}

func TestGeminiSafetyBlockBillsPromptTokensAcrossHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("native_generateContent_with_usageMetadata", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash:generateContent", nil)

		info := &relaycommon.RelayInfo{
			OriginModelName: "gemini-3-flash",
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "gemini-3-flash",
			},
		}

		respBody := `{"candidates":[],"promptFeedback":{"blockReason":"SAFETY"},"usageMetadata":{"promptTokenCount":42,"totalTokenCount":42}}`
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(respBody))),
		}

		usage, err := GeminiTextGenerationHandler(c, info, resp)
		require.Nil(t, err)
		require.NotNil(t, usage)
		require.Equal(t, 42, usage.PromptTokens)
		require.Equal(t, 0, usage.CompletionTokens)
		require.Equal(t, 42, usage.TotalTokens)
		require.NotNil(t, usage.BillingUsage)
		require.Equal(t, 0, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
		require.Equal(t, 42, usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
		require.Equal(t, "gemini_block_reason=SAFETY", common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason))
	})

	t.Run("chat_completions_with_usageMetadata", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		info := &relaycommon.RelayInfo{
			RelayFormat:     types.RelayFormatOpenAI,
			OriginModelName: "gemini-3-flash",
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "gemini-3-flash",
			},
		}

		respBody := `{"candidates":[],"promptFeedback":{"blockReason":"SAFETY"},"usageMetadata":{"promptTokenCount":55,"totalTokenCount":55}}`
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte(respBody))),
		}

		usage, err := GeminiChatHandler(c, info, resp)
		require.Nil(t, err)
		require.NotNil(t, usage)
		require.Equal(t, 55, usage.PromptTokens)
		require.Equal(t, 0, usage.CompletionTokens)
		require.Equal(t, 55, usage.TotalTokens)
		require.NotNil(t, usage.BillingUsage)
		require.Equal(t, 0, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
		require.Equal(t, 55, usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
		require.Equal(t, "gemini_block_reason=SAFETY", common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason))
	})

	t.Run("stream_with_usageMetadata", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

		oldStreamingTimeout := constant.StreamingTimeout
		constant.StreamingTimeout = 300
		t.Cleanup(func() {
			constant.StreamingTimeout = oldStreamingTimeout
		})

		info := &relaycommon.RelayInfo{
			OriginModelName: "gemini-3-flash",
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "gemini-3-flash",
			},
		}

		streamBody := []byte("data: {\"candidates\":[],\"promptFeedback\":{\"blockReason\":\"SAFETY\"},\"usageMetadata\":{\"promptTokenCount\":68,\"totalTokenCount\":68}}\n")
		resp := &http.Response{
			Body: io.NopCloser(bytes.NewReader(streamBody)),
		}

		called := false
		usage, err := geminiStreamHandler(c, info, resp, func(data string, r *dto.GeminiChatResponse) bool {
			called = true
			return true
		})
		require.Nil(t, err)
		require.NotNil(t, usage)
		require.True(t, called)
		require.Equal(t, 68, usage.PromptTokens)
		require.Equal(t, 0, usage.CompletionTokens)
		require.Equal(t, 68, usage.TotalTokens)
		require.NotNil(t, usage.BillingUsage)
		require.Equal(t, 0, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
		require.Equal(t, 68, usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
		require.Equal(t, "gemini_block_reason=SAFETY", common.GetContextKeyString(c, constant.ContextKeyAdminRejectReason))
	})
}

