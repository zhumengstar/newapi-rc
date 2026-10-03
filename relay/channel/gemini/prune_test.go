package gemini

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAutoPruneGeminiChatRequest_Normal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{
				Role: "user",
				Parts: []dto.GeminiPart{
					{Text: "你好"},
				},
			},
			{
				Role: "model",
				Parts: []dto.GeminiPart{
					{Text: "你好！有什么我可以帮你的吗？"},
				},
			},
			{
				Role: "user",
				Parts: []dto.GeminiPart{
					{Text: "今天天气怎么样？"},
				},
			},
		},
	}

	info := &relaycommon.RelayInfo{}
	pruned := AutoPruneGeminiChatRequest(c, info, req)
	assert.False(t, pruned)
	assert.Equal(t, 3, len(req.Contents))
	assert.False(t, c.GetBool("context_pruned"))
}

func TestAutoPruneGeminiChatRequest_PrunesEarlyHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 构造 10 轮历史对话，每轮约 300,000 字符（约 100,000 tokens），总计 > 1,000,000 tokens
	largeChunk := strings.Repeat("这是一段用于测试长上下文的历史对话文本内容，重复填充。", 10000) // ~300KB
	contents := make([]dto.GeminiChatContent, 0, 12)
	for i := 0; i < 5; i++ {
		contents = append(contents, dto.GeminiChatContent{
			Role:  "user",
			Parts: []dto.GeminiPart{{Text: "用户历史提问: " + largeChunk}},
		})
		contents = append(contents, dto.GeminiChatContent{
			Role:  "model",
			Parts: []dto.GeminiPart{{Text: "模型历史回答: " + largeChunk}},
		})
	}
	// 最新用户提问
	latestUserQuestion := "这是最新的用户提问，请回答。"
	contents = append(contents, dto.GeminiChatContent{
		Role:  "user",
		Parts: []dto.GeminiPart{{Text: latestUserQuestion}},
	})

	sysInstruction := &dto.GeminiChatContent{
		Role:  "system",
		Parts: []dto.GeminiPart{{Text: "你是一个智能助理"}},
	}

	req := &dto.GeminiChatRequest{
		SystemInstructions: sysInstruction,
		Contents:           contents,
	}

	totalTokensBefore := EstimateGeminiRequestTokens(req)
	assert.Greater(t, totalTokensBefore, GeminiSafeTargetTokens)

	info := &relaycommon.RelayInfo{}
	pruned := AutoPruneGeminiChatRequest(c, info, req)

	assert.True(t, pruned)
	assert.True(t, c.GetBool("context_pruned"))
	// 验证系统指令未变
	assert.Equal(t, "你是一个智能助理", req.SystemInstructions.Parts[0].Text)
	// 验证第一条消息必须是 user
	assert.NotEmpty(t, req.Contents)
	assert.Equal(t, "user", req.Contents[0].Role)
	// 验证最后一条消息为最新的用户提问
	assert.Equal(t, latestUserQuestion, req.Contents[len(req.Contents)-1].Parts[0].Text)
	// 验证裁剪后总 Tokens 低于安全阈值
	totalTokensAfter := EstimateGeminiRequestTokens(req)
	assert.LessOrEqual(t, totalTokensAfter, GeminiSafeTargetTokens)
	assert.Equal(t, totalTokensAfter, info.GetEstimatePromptTokens())
}

func TestAutoPruneGeminiChatRequest_FirstContentMustBeUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 构造特定结构：如果刚好裁剪掉第 0 项后，第 1 项是 model，裁剪算法必须继续顺延剔除该 model 直到首项为 user
	chunk := strings.Repeat("A", 1500000) // ~500,000 tokens
	req := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{
				Role:  "user",
				Parts: []dto.GeminiPart{{Text: chunk}},
			},
			{
				Role:  "model",
				Parts: []dto.GeminiPart{{Text: chunk}},
			},
			{
				Role:  "user",
				Parts: []dto.GeminiPart{{Text: "最新用户问题"}},
			},
		},
	}

	info := &relaycommon.RelayInfo{}
	pruned := AutoPruneGeminiChatRequest(c, info, req)
	assert.True(t, pruned)
	// 裁剪后必须只剩最后一项 user
	assert.Equal(t, 1, len(req.Contents))
	assert.Equal(t, "user", req.Contents[0].Role)
	assert.Equal(t, "最新用户问题", req.Contents[0].Parts[0].Text)
}

func TestAutoPruneGeminiChatRequest_SingleExcessiveTextTruncated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 单条消息超过 3MB
	hugeText := strings.Repeat("0123456789", 350000) // 3.5MB
	req := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{
				Role:  "user",
				Parts: []dto.GeminiPart{{Text: hugeText}},
			},
		},
	}

	info := &relaycommon.RelayInfo{}
	pruned := AutoPruneGeminiChatRequest(c, info, req)
	assert.True(t, pruned)
	assert.True(t, c.GetBool("context_pruned"))
	assert.Contains(t, req.Contents[0].Parts[0].Text, "已自动截断超限内容以符合")
	assert.LessOrEqual(t, EstimateGeminiRequestTokens(req), GeminiDefaultSafeTargetTokens)
}

func TestAutoPruneGeminiChatRequest_128KModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// 针对 gemini-3.7-flash (或 gemini-3.7-flash-high) 模型，安全上限为 120,000 tokens
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3.7-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3.7-flash-high",
		},
	}

	// 构造 200,000 tokens 的多轮对话（超过 128k 上限 131,072，但小于 1M）
	chunk := strings.Repeat("长对话测试文本内容，", 5000) // ~50,000 字符 = ~17,000 tokens
	contents := make([]dto.GeminiChatContent, 0, 15)
	for i := 0; i < 6; i++ {
		contents = append(contents, dto.GeminiChatContent{
			Role:  "user",
			Parts: []dto.GeminiPart{{Text: fmt.Sprintf("轮次 %d: %s", i, chunk)}},
		})
		contents = append(contents, dto.GeminiChatContent{
			Role:  "model",
			Parts: []dto.GeminiPart{{Text: fmt.Sprintf("回答 %d: %s", i, chunk)}},
		})
	}
	latestUser := "最新用户针对 3.7-flash 的提问"
	contents = append(contents, dto.GeminiChatContent{
		Role:  "user",
		Parts: []dto.GeminiPart{{Text: latestUser}},
	})

	req := &dto.GeminiChatRequest{
		Contents: contents,
	}

	totalBefore := EstimateGeminiRequestTokens(req)
	assert.Greater(t, totalBefore, 131072) // 确实超过 128k 上限

	pruned := AutoPruneGeminiChatRequest(c, info, req)
	assert.True(t, pruned)
	assert.True(t, c.GetBool("context_pruned"))

	totalAfter := EstimateGeminiRequestTokens(req)
	assert.LessOrEqual(t, totalAfter, Gemini128KSafeTargetTokens) // 裁剪到了 120,000 以内！
	assert.Equal(t, "user", req.Contents[0].Role)
	assert.Equal(t, latestUser, req.Contents[len(req.Contents)-1].Parts[0].Text)
}

func TestParseExceedTokenLimitFromError(t *testing.T) {
	err1 := "status_code=400, The input token count exceeds the maximum number of tokens allowed 131072."
	limit1 := ParseExceedTokenLimitFromError(err1)
	assert.Equal(t, 131072, limit1)

	err2 := "status_code=400, The input token count exceeds the maximum number of tokens allowed (1048576)."
	limit2 := ParseExceedTokenLimitFromError(err2)
	assert.Equal(t, 1048576, limit2)

	err3 := "bad response status code 400 with empty error message"
	limit3 := ParseExceedTokenLimitFromError(err3)
	assert.Equal(t, 0, limit3)
}

