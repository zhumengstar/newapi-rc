package gemini

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

const (
	// GeminiDefaultMaxAllowedTokens Google Gemini 官方标准单次上下文硬上限 1,048,576 (1M)
	GeminiDefaultMaxAllowedTokens = 1048576
	// GeminiDefaultSafeTargetTokens 1M 模型滑动裁剪安全目标上限（预留约 6.8 万 tokens 给系统提示词、输出与分词差异）
	GeminiDefaultSafeTargetTokens = 980000
	// Gemini128KSafeTargetTokens 128K 模型（如 gemini-3.7-flash, flash-high 等）安全裁剪上限（预留约 1.1 万 tokens 冗余）
	Gemini128KSafeTargetTokens = 120000
	// GeminiSafeTargetTokens 向下兼容旧常量名
	GeminiSafeTargetTokens = GeminiDefaultSafeTargetTokens
)

var (
	modelTokenLimitCache sync.Map
	// 匹配 "The input token count exceeds the maximum number of tokens allowed 131072."
	// 或 "The input token count exceeds the maximum number of tokens allowed (1048576)."
	tokenLimitRegex = regexp.MustCompile(`(?i)exceeds the maximum number of tokens allowed\s*\(?(\d+)\)?`)
)

// ParseExceedTokenLimitFromError 从上游 400 错误文本中提取具体的 token 上限数字
func ParseExceedTokenLimitFromError(errMsg string) int {
	matches := tokenLimitRegex.FindStringSubmatch(errMsg)
	if len(matches) >= 2 {
		if limit, err := strconv.Atoi(matches[1]); err == nil && limit > 0 {
			return limit
		}
	}
	return 0
}

// RecordModelTokenLimit 动态记录模型的实际 Token 硬上限
func RecordModelTokenLimit(modelName string, limit int) {
	if modelName != "" && limit > 0 {
		modelTokenLimitCache.Store(strings.ToLower(strings.TrimSpace(modelName)), limit)
	}
}

// calculateSafeTarget 根据 hardLimit 计算安全裁剪目标
func calculateSafeTarget(limit int) int {
	if limit <= 0 {
		return GeminiDefaultSafeTargetTokens
	}
	// 预留约 8%~10% 作为系统指令、多模态与分词误差安全冗余
	margin := limit / 12
	if margin < 5000 {
		margin = 5000
	}
	if margin > 68576 {
		margin = 68576
	}
	target := limit - margin
	if target <= 0 {
		target = limit / 2
	}
	return target
}

// GetModelSafeTargetTokens 动态获取当前请求模型对应的安全裁剪阈值
func GetModelSafeTargetTokens(c *gin.Context, info *relaycommon.RelayInfo) int {
	// 1. 如果 context 中显式指定了强制上限（例如重试自愈场景中解析出的 131072）
	if c != nil {
		if forced, exists := c.Get("forced_max_tokens"); exists {
			if limit, ok := forced.(int); ok && limit > 0 {
				return calculateSafeTarget(limit)
			}
		}
	}

	checkModel := func(name string) int {
		if name == "" {
			return 0
		}
		lower := strings.ToLower(strings.TrimSpace(name))
		// 查动态学习缓存
		if val, ok := modelTokenLimitCache.Load(lower); ok {
			if limit, ok := val.(int); ok && limit > 0 {
				return calculateSafeTarget(limit)
			}
		}
		// 已知 128k 上限模型（3.7-flash, 3.6-flash, 3.8-flash, flash-high, pro-low 等）
		if strings.Contains(lower, "3.7-flash") ||
			strings.Contains(lower, "3.6-flash") ||
			strings.Contains(lower, "3.8-flash") ||
			strings.Contains(lower, "flash-high") ||
			strings.Contains(lower, "pro-low") ||
			strings.Contains(lower, "128k") {
			return Gemini128KSafeTargetTokens
		}
		return 0
	}

	if info != nil {
		if target := checkModel(info.GetUpstreamModelName()); target > 0 {
			return target
		}
		if target := checkModel(info.GetOriginModelName()); target > 0 {
			return target
		}
	}
	if c != nil {
		if target := checkModel(c.GetString("original_model")); target > 0 {
			return target
		}
		if target := checkModel(c.GetString("model")); target > 0 {
			return target
		}
	}

	return GeminiDefaultSafeTargetTokens
}

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
// 3. 动态获取当前模型的安全目标（1M 模型为 980,000，128K 模型如 gemini-3.7-flash 为 120,000，或由错误自愈提取的精确上限）；
// 4. 自前往后滑动剔除早期历史轮次，直至输入低于安全阈值；
// 5. 保证裁剪后的首条消息 role 为 "user"；
// 6. 若单轮输入文本极端超长，对单条文本进行尾部安全截断兜底。
func AutoPruneGeminiChatRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.GeminiChatRequest) bool {
	if req == nil || len(req.Contents) == 0 {
		return false
	}

	safeTargetTokens := GetModelSafeTargetTokens(c, info)
	currentTokens := EstimateGeminiRequestTokens(req)
	// 如果估算未超过安全目标，不需要裁剪
	if currentTokens <= safeTargetTokens {
		return false
	}

	originTokens := currentTokens
	originCount := len(req.Contents)
	prunedHistoryCount := 0

	// 阶段一：滑动裁剪早期历史对话（保留最新的一条提问）
	for len(req.Contents) > 1 && currentTokens > safeTargetTokens {
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
	maxSingleTextBytes := safeTargetTokens * 3
	if maxSingleTextBytes > 2700000 {
		maxSingleTextBytes = 2700000
	}

	// 阶段三：兜底防御——如果历史消息已经全部裁剪只剩最新消息，但单条最新消息依然超过安全上限
	if currentTokens > safeTargetTokens && len(req.Contents) > 0 {
		lastIdx := len(req.Contents) - 1
		for pIdx := range req.Contents[lastIdx].Parts {
			part := &req.Contents[lastIdx].Parts[pIdx]
			if len(part.Text) > maxSingleTextBytes {
				// 保留前 maxSingleTextBytes 字节，并确保 utf8 截断不破坏多字节字符
				truncated := truncateUTF8Safe(part.Text, maxSingleTextBytes)
				part.Text = truncated + fmt.Sprintf("\n\n[... 输入超长，已自动截断超限内容以符合 %d 上下文上限 ...]", safeTargetTokens)
				singleTruncated = true
			}
		}
		// 重新计算 Token
		currentTokens = EstimateGeminiRequestTokens(req)
	}

	if prunedHistoryCount > 0 || singleTruncated {
		msg := fmt.Sprintf("历史上下文超限（原估算 %d tokens，目标安全上限 %d tokens），已自动滑动裁剪早期历史对话 %d 条（原 %d 条，现 %d 条，剩余估算 %d tokens）",
			originTokens, safeTargetTokens, prunedHistoryCount, originCount, len(req.Contents), currentTokens)
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

// PruneOnTokenLimitExceeded 当收到上游 token 超限错误时，解析上限并对请求执行紧急强制滑动裁剪
// 返回 true 表示成功裁剪并可以进行重试，返回 false 表示无法进一步裁剪
func PruneOnTokenLimitExceeded(c *gin.Context, info *relaycommon.RelayInfo, req *dto.GeminiChatRequest, errText string) bool {
	if req == nil {
		return false
	}

	limit := ParseExceedTokenLimitFromError(errText)
	if limit > 0 {
		c.Set("forced_max_tokens", limit)
		common.SetContextKey(c, "forced_max_tokens", limit)
		if info != nil {
			RecordModelTokenLimit(info.GetUpstreamModelName(), limit)
			RecordModelTokenLimit(info.GetOriginModelName(), limit)
		}
	} else {
		// 未能解析出数字时，强制将当前估算削减 40% 作为新目标
		curr := EstimateGeminiRequestTokens(req)
		fallbackTarget := int(float64(curr) * 0.6)
		if fallbackTarget < 30000 {
			fallbackTarget = 30000
		}
		c.Set("forced_max_tokens", fallbackTarget)
		common.SetContextKey(c, "forced_max_tokens", fallbackTarget)
	}

	return AutoPruneGeminiChatRequest(c, info, req)
}

