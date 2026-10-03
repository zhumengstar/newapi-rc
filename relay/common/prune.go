package common

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

var (
	// GlobalModelTokenLimitCache 全局动态学习模型 Token 上限缓存（无需指定模型，自动感知并缓存）
	GlobalModelTokenLimitCache sync.Map

	// 正则匹配主流模型（Gemini, OpenAI, Claude, DeepSeek, Qwen 等）报错中的上下文超限数字
	universalTokenLimitRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)exceeds the maximum number of tokens allowed\s*\(?(\d+)\)?`),
		regexp.MustCompile(`(?i)maximum context length is\s*(\d+)`),
		regexp.MustCompile(`(?i)max(?:imum)?\s*(?:context|token|input)\s*(?:length|limit)\s*(?:is|of)?\s*(\d+)`),
		regexp.MustCompile(`(?i)>\s*(\d+)\s*maximum`),
		regexp.MustCompile(`(?i)limit of\s*(\d+)\s*tokens`),
		regexp.MustCompile(`(?i)exceeds?\s*(?:the\s*)?(?:token\s*)?limit\s*(?:of\s*)?(\d+)`),
	}

	universalTokenLimitKeywords = []string{
		"context_length_exceeded",
		"maximum number of tokens allowed",
		"exceeds the maximum",
		"the input token count exceeds",
		"prompt is too long",
		"maximum context length",
		"too many tokens",
		"input tokens exceed",
		"prompt tokens exceed",
	}
)

// ParseUniversalContextLimit 从任意上游报错信息中识别上下文超限并提取上限数值
func ParseUniversalContextLimit(errMsg string) (limit int, isExceeded bool) {
	if errMsg == "" {
		return 0, false
	}
	for _, reg := range universalTokenLimitRegexes {
		matches := reg.FindStringSubmatch(errMsg)
		if len(matches) >= 2 {
			if parsed, err := strconv.Atoi(matches[1]); err == nil && parsed > 0 {
				return parsed, true
			}
		}
	}
	lower := strings.ToLower(errMsg)
	for _, kw := range universalTokenLimitKeywords {
		if strings.Contains(lower, kw) {
			return 0, true
		}
	}
	return 0, false
}

// RecordGlobalModelTokenLimit 记录任意模型的动态上限
func RecordGlobalModelTokenLimit(modelName string, limit int) {
	if modelName != "" && limit > 0 {
		GlobalModelTokenLimitCache.Store(strings.ToLower(strings.TrimSpace(modelName)), limit)
	}
}

// GetGlobalModelTargetLimit 获取任意模型的安全裁剪目标
func GetGlobalModelTargetLimit(c *gin.Context, info *RelayInfo, defaultTarget int) int {
	if c != nil {
		if forced, exists := c.Get("forced_max_tokens"); exists {
			if limit, ok := forced.(int); ok && limit > 0 {
				return calculateUniversalSafeTarget(limit)
			}
		}
	}

	checkModel := func(name string) int {
		if name == "" {
			return 0
		}
		lower := strings.ToLower(strings.TrimSpace(name))
		if val, ok := GlobalModelTokenLimitCache.Load(lower); ok {
			if limit, ok := val.(int); ok && limit > 0 {
				return calculateUniversalSafeTarget(limit)
			}
		}
		// 通用规则：包含 128k、3.7-flash、flash-high 等关键词自动识别为 128K 上限
		if strings.Contains(lower, "128k") ||
			strings.Contains(lower, "3.7-flash") ||
			strings.Contains(lower, "3.6-flash") ||
			strings.Contains(lower, "3.8-flash") ||
			strings.Contains(lower, "flash-high") ||
			strings.Contains(lower, "pro-low") {
			return 120000
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

	if defaultTarget > 0 {
		return defaultTarget
	}
	return 980000
}

func calculateUniversalSafeTarget(limit int) int {
	if limit <= 0 {
		return 980000
	}
	margin := limit / 12
	if margin < 4000 {
		margin = 4000
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

// EstimateGenericTextTokens 文本 Token 估算器
func EstimateGenericTextTokens(text string) int {
	if text == "" {
		return 0
	}
	t := len(text) / 3
	if t == 0 {
		return 1
	}
	return t
}

// TruncateUTF8Safe 安全截断字符串，确保不破坏 UTF-8 字符边界
func TruncateUTF8Safe(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// AutoPruneOpenAIRequest 对通用 OpenAI 格式请求执行历史对话智能滑动裁剪
func AutoPruneOpenAIRequest(c *gin.Context, info *RelayInfo, req *dto.GeneralOpenAIRequest, targetLimit int) bool {
	if req == nil || len(req.Messages) <= 1 {
		return false
	}

	if targetLimit <= 0 {
		targetLimit = GetGlobalModelTargetLimit(c, info, 980000)
	}

	estimateMessageTokens := func(m *dto.Message) int {
		tokens := 4
		if m.Content != nil {
			switch v := m.Content.(type) {
			case string:
				tokens += EstimateGenericTextTokens(v)
			default:
				tokens += EstimateGenericTextTokens(fmt.Sprintf("%v", v))
			}
		}
		if m.ReasoningContent != nil {
			tokens += EstimateGenericTextTokens(*m.ReasoningContent)
		}
		return tokens
	}

	totalTokens := 0
	for i := range req.Messages {
		totalTokens += estimateMessageTokens(&req.Messages[i])
	}

	if totalTokens <= targetLimit {
		return false
	}

	originTokens := totalTokens
	originCount := len(req.Messages)
	prunedCount := 0

	// 提取并保留所有系统消息与最新提问
	// 从前向后滑动裁剪早期历史对话（跳过 system 消息和最后一条提问）
	for totalTokens > targetLimit && len(req.Messages) > 1 {
		removeIdx := -1
		for i := 0; i < len(req.Messages)-1; i++ {
			if req.Messages[i].Role != "system" {
				removeIdx = i
				break
			}
		}
		if removeIdx == -1 {
			break
		}
		removedTokens := estimateMessageTokens(&req.Messages[removeIdx])
		req.Messages = append(req.Messages[:removeIdx], req.Messages[removeIdx+1:]...)
		totalTokens -= removedTokens
		prunedCount++
	}

	// 规范校准：保证第一条非 system 消息为 user
	for len(req.Messages) > 1 {
		firstNonSystem := -1
		for i := 0; i < len(req.Messages)-1; i++ {
			if req.Messages[i].Role != "system" {
				firstNonSystem = i
				break
			}
		}
		if firstNonSystem != -1 && req.Messages[firstNonSystem].Role != "user" {
			removedTokens := estimateMessageTokens(&req.Messages[firstNonSystem])
			req.Messages = append(req.Messages[:firstNonSystem], req.Messages[firstNonSystem+1:]...)
			totalTokens -= removedTokens
			prunedCount++
		} else {
			break
		}
	}

	if prunedCount > 0 {
		msg := fmt.Sprintf("历史上下文超限（原估算 %d tokens，安全上限 %d tokens），已自动滑动裁剪早期历史对话 %d 条（原 %d 条，现 %d 条，剩余估算 %d tokens）",
			originTokens, targetLimit, prunedCount, originCount, len(req.Messages), totalTokens)
		if c != nil {
			logger.LogWarn(c, msg)
			c.Set("context_pruned", true)
			common.SetContextKey(c, "context_pruned", true)
		}
		if info != nil {
			info.SetEstimatePromptTokens(totalTokens)
		}
		return true
	}

	return false
}

// AutoPruneClaudeRequest 对 Claude 格式请求执行历史对话智能滑动裁剪
func AutoPruneClaudeRequest(c *gin.Context, info *RelayInfo, req *dto.ClaudeRequest, targetLimit int) bool {
	if req == nil || len(req.Messages) <= 1 {
		return false
	}

	if targetLimit <= 0 {
		targetLimit = GetGlobalModelTargetLimit(c, info, 980000)
	}

	estimateClaudeMessageTokens := func(m *dto.ClaudeMessage) int {
		tokens := 4
		if m.Content != nil {
			switch v := m.Content.(type) {
			case string:
				tokens += EstimateGenericTextTokens(v)
			default:
				tokens += EstimateGenericTextTokens(fmt.Sprintf("%v", v))
			}
		}
		return tokens
	}

	totalTokens := 0
	if req.System != nil {
		switch v := req.System.(type) {
		case string:
			totalTokens += EstimateGenericTextTokens(v)
		default:
			totalTokens += EstimateGenericTextTokens(fmt.Sprintf("%v", v))
		}
	}
	for i := range req.Messages {
		totalTokens += estimateClaudeMessageTokens(&req.Messages[i])
	}

	if totalTokens <= targetLimit {
		return false
	}

	originTokens := totalTokens
	originCount := len(req.Messages)
	prunedCount := 0

	// 保留最新提问（最后一条），从前往后滑动修剪较早的历史轮次
	for len(req.Messages) > 1 && totalTokens > targetLimit {
		removedTokens := estimateClaudeMessageTokens(&req.Messages[0])
		req.Messages = req.Messages[1:]
		totalTokens -= removedTokens
		prunedCount++
	}

	// Claude 规范约束：第一条消息必须是 user
	for len(req.Messages) > 1 && req.Messages[0].Role != "user" {
		removedTokens := estimateClaudeMessageTokens(&req.Messages[0])
		req.Messages = req.Messages[1:]
		totalTokens -= removedTokens
		prunedCount++
	}

	if prunedCount > 0 {
		msg := fmt.Sprintf("历史上下文超限（原估算 %d tokens，安全上限 %d tokens），已自动滑动裁剪早期历史对话 %d 条（原 %d 条，现 %d 条，剩余估算 %d tokens）",
			originTokens, targetLimit, prunedCount, originCount, len(req.Messages), totalTokens)
		if c != nil {
			logger.LogWarn(c, msg)
			c.Set("context_pruned", true)
			common.SetContextKey(c, "context_pruned", true)
		}
		if info != nil {
			info.SetEstimatePromptTokens(totalTokens)
		}
		return true
	}

	return false
}

// UniversalPruneAnyRequest 支持裁剪任意对象（无论 info.Request 还是 convertedRequest）
func UniversalPruneAnyRequest(c *gin.Context, info *RelayInfo, reqObj any, targetLimit int) bool {
	if reqObj == nil {
		return false
	}
	switch req := reqObj.(type) {
	case *dto.GeneralOpenAIRequest:
		return AutoPruneOpenAIRequest(c, info, req, targetLimit)
	case *dto.ClaudeRequest:
		return AutoPruneClaudeRequest(c, info, req, targetLimit)
	case *dto.GeminiChatRequest:
		return AutoPruneGeminiChatRequest(c, info, req, targetLimit)
	default:
		return false
	}
}

// UniversalPruneRelayRequest 全局通用自愈裁剪器：自动识别当前请求协议类型并执行智能滑动裁剪
func UniversalPruneRelayRequest(c *gin.Context, info *RelayInfo, targetLimit int) bool {
	if info == nil || info.Request == nil {
		return false
	}
	return UniversalPruneAnyRequest(c, info, info.Request, targetLimit)
}

// AutoPruneGeminiChatRequest 对 Gemini 请求执行滑动裁剪
func AutoPruneGeminiChatRequest(c *gin.Context, info *RelayInfo, req *dto.GeminiChatRequest, targetLimit int) bool {
	if req == nil || len(req.Contents) <= 1 {
		return false
	}

	if targetLimit <= 0 {
		targetLimit = GetGlobalModelTargetLimit(c, info, 980000)
	}

	estimateGeminiTokens := func(content *dto.GeminiChatContent) int {
		if content == nil {
			return 0
		}
		tokens := 0
		for _, part := range content.Parts {
			if part.Text != "" {
				tokens += EstimateGenericTextTokens(part.Text)
			}
			if part.InlineData != nil || part.FileData != nil {
				tokens += 258
			}
		}
		return tokens
	}

	totalTokens := 0
	if req.SystemInstructions != nil {
		totalTokens += estimateGeminiTokens(req.SystemInstructions)
	}
	for i := range req.Contents {
		totalTokens += estimateGeminiTokens(&req.Contents[i])
	}

	if totalTokens <= targetLimit {
		return false
	}

	originTokens := totalTokens
	originCount := len(req.Contents)
	prunedCount := 0

	for len(req.Contents) > 1 && totalTokens > targetLimit {
		removedTokens := estimateGeminiTokens(&req.Contents[0])
		req.Contents = req.Contents[1:]
		totalTokens -= removedTokens
		prunedCount++
	}

	for len(req.Contents) > 1 && req.Contents[0].Role != "user" {
		removedTokens := estimateGeminiTokens(&req.Contents[0])
		req.Contents = req.Contents[1:]
		totalTokens -= removedTokens
		prunedCount++
	}

	if prunedCount > 0 {
		msg := fmt.Sprintf("历史上下文超限（原估算 %d tokens，安全上限 %d tokens），已自动滑动裁剪早期历史对话 %d 条（原 %d 条，现 %d 条，剩余估算 %d tokens）",
			originTokens, targetLimit, prunedCount, originCount, len(req.Contents), totalTokens)
		if c != nil {
			logger.LogWarn(c, msg)
			c.Set("context_pruned", true)
			common.SetContextKey(c, "context_pruned", true)
		}
		if info != nil {
			info.SetEstimatePromptTokens(totalTokens)
		}
		return true
	}

	return false
}

// UpdatePrunedRequestBody 将裁剪后的 info.Request 序列化并更新到 Context 的 BodyStorage 中
func UpdatePrunedRequestBody(c *gin.Context, info *RelayInfo) error {
	if c == nil || info == nil || info.Request == nil {
		return nil
	}
	jsonData, err := common.Marshal(info.Request)
	if err != nil {
		return err
	}
	newStorage, err := common.CreateBodyStorage(jsonData)
	if err != nil {
		return err
	}
	if oldStorage, exists := c.Get(common.KeyBodyStorage); exists && oldStorage != nil {
		if bs, ok := oldStorage.(common.BodyStorage); ok {
			_ = bs.Close()
		}
	}
	c.Set(common.KeyBodyStorage, newStorage)
	c.Set(common.KeyRequestBody, jsonData)
	common.SetContextKey(c, common.KeyRequestBody, jsonData)
	if c.Request != nil {
		c.Request.Body = io.NopCloser(newStorage)
		c.Request.ContentLength = int64(len(jsonData))
		if c.Request.Header != nil {
			c.Request.Header.Set("Content-Length", strconv.Itoa(len(jsonData)))
		}
	}
	return nil
}

// UniversalPruneAndRefreshRequest 通用裁剪并刷新请求体（可在任何重试或拦截处直接调用）
func UniversalPruneAndRefreshRequest(c *gin.Context, info *RelayInfo, targetLimit int) bool {
	if info == nil || info.Request == nil {
		return false
	}
	pruned := UniversalPruneRelayRequest(c, info, targetLimit)
	if pruned {
		_ = UpdatePrunedRequestBody(c, info)
		return true
	}
	return false
}

