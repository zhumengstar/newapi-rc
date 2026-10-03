package gemini

import (
	"fmt"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

const (
	// GeminiMaxAllowedTokens Google Gemini 官方单次上下文硬上限 1,048,576 (1M)
	GeminiMaxAllowedTokens = 1048576
	// GeminiSafeTargetTokens 滑动裁剪安全目标上限（预留约 6.8 万 tokens 给系统提示词、输出与分词差异）
	GeminiSafeTargetTokens = 980000
	// GeminiMaxSingleTextBytes 单个文本安全最大字节数（约 900,000 tokens * 3 字节，约 2.7MB）
	GeminiMaxSingleTextBytes = 2700000
)

// EstimateGeminiContentTokens 估算单个 Gemini Content 的 Token 数
func EstimateGeminiContentTokens(content *dto.GeminiChatContent) int {
	if content == nil {
		return 0
	}
	tokens := 0
	for _, part := range content.Parts {
		if part.Text != "" {
			// 中英混合文本：平均约 3 个字节 1 token，至少 1 token
			t := len(part.Text) / 3
			if t == 0 {
				t = 1
			}
			tokens += t
		}
		if part.InlineData != nil || part.FileData != nil {
			// 多模态图片/文件估算约 258 tokens
			tokens += 258
		}
	}
	return tokens
}

// EstimateGeminiRequestTokens 估算整个 GeminiChatRequest 的输入 Token 数
func EstimateGeminiRequestTokens(req *dto.GeminiChatRequest) int {
	if req == nil {
		return 0
	}
	total := 0
	if req.SystemInstructions != nil {
		total += EstimateGeminiContentTokens(req.SystemInstructions)
	}
	for i := range req.Contents {
		total += EstimateGeminiContentTokens(&req.Contents[i])
	}
	return total
}

// AutoPruneGeminiChatRequest 对 Gemini 请求执行历史对话智能滑动裁剪
// 1. 保留 SystemPrompt 不动；
// 2. 保留最新提问（最后一条消息）不动；
// 3. 自前往后滑动剔除早期历史轮次，直至输入低于安全阈值 (980,000 tokens)；
// 4. 保证裁剪后的首条消息 role 为 "user"；
// 5. 若单轮输入文本极端超长（>2.7MB），对单条文本进行尾部安全截断兜底。
func AutoPruneGeminiChatRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.GeminiChatRequest) bool {
	if req == nil || len(req.Contents) == 0 {
		return false
	}

	currentTokens := EstimateGeminiRequestTokens(req)
	// 如果估算未超过安全目标，不需要裁剪
	if currentTokens <= GeminiSafeTargetTokens {
		return false
	}

	originTokens := currentTokens
	originCount := len(req.Contents)
	prunedHistoryCount := 0

	// 阶段一：滑动裁剪早期历史对话（保留最新的一条提问）
	for len(req.Contents) > 1 && currentTokens > GeminiSafeTargetTokens {
		removedTokens := EstimateGeminiContentTokens(&req.Contents[0])
		req.Contents = req.Contents[1:]
		currentTokens -= removedTokens
		prunedHistoryCount++
	}

	// 阶段二：Gemini API 规范要求：对话历史第一条消息的 role 必须是 "user"
	for len(req.Contents) > 1 && req.Contents[0].Role != "user" {
		removedTokens := EstimateGeminiContentTokens(&req.Contents[0])
		req.Contents = req.Contents[1:]
		currentTokens -= removedTokens
		prunedHistoryCount++
	}

	singleTruncated := false
	// 阶段三：兜底防御——如果历史消息已经全部裁剪只剩最新消息，但单条最新消息依然超过安全上限
	if currentTokens > GeminiSafeTargetTokens && len(req.Contents) > 0 {
		lastIdx := len(req.Contents) - 1
		for pIdx := range req.Contents[lastIdx].Parts {
			part := &req.Contents[lastIdx].Parts[pIdx]
			if len(part.Text) > GeminiMaxSingleTextBytes {
				// 保留前 GeminiMaxSingleTextBytes 字节，并确保 utf8 截断不破坏多字节字符
				truncated := truncateUTF8Safe(part.Text, GeminiMaxSingleTextBytes)
				part.Text = truncated + "\n\n[... 输入超长，已自动截断超限内容以符合 1M 上下文上限 ...]"
				singleTruncated = true
			}
		}
		// 重新计算 Token
		currentTokens = EstimateGeminiRequestTokens(req)
	}

	if prunedHistoryCount > 0 || singleTruncated {
		msg := fmt.Sprintf("历史上下文超限（原估算 %d tokens），已自动滑动裁剪早期历史对话 %d 条（原 %d 条，现 %d 条，剩余估算 %d tokens）",
			originTokens, prunedHistoryCount, originCount, len(req.Contents), currentTokens)
		if singleTruncated {
			msg += "，并对单条超大消息执行了安全截断"
		}
		if c != nil {
			logger.LogWarn(c, msg)
			c.Set("context_pruned", true)
			common.SetContextKey(c, "context_pruned", true)
		}
		if info != nil {
			info.SetEstimatePromptTokens(currentTokens)
		}
		return true
	}

	return false
}

// truncateUTF8Safe 安全截断字符串，确保不会切断 UTF-8 多字节字符
func truncateUTF8Safe(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}
