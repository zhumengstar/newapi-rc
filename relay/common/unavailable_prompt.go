package common

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/types"
)

var (
	claudeUnavailableRe = regexp.MustCompile(`(?i)Claude\s+Opus\s+4\.6\s+is\s+no\s+longer\s+available[\.\s]*(Please\s+switch\s+to\s+Claude\s+Opus\s+5\.5[\.\s]*)?`)
)

// IsClaudeUnavailablePrompt 判定文本是否包含 Claude Opus 4.6 不可用并要求切换 5.5 的提示
func IsClaudeUnavailablePrompt(text string) bool {
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "claude opus 4.6 is no longer available") ||
		strings.Contains(lower, "switch to claude opus 5.5")
}

// CleanUnavailablePromptFromText 从输入文本中清洗剥离不可用废话提示
func CleanUnavailablePromptFromText(text string) (string, bool) {
	if !IsClaudeUnavailablePrompt(text) {
		return text, false
	}
	cleaned := claudeUnavailableRe.ReplaceAllString(text, "")
	return strings.TrimSpace(cleaned), true
}

type readCloserWithPrefix struct {
	io.Reader
	io.Closer
}

// CheckAndInterceptUnavailableResponse 前置嗅探上游响应内容（无论 HTTP 状态码是否为 200）。
// 若检测到 "Claude Opus 4.6 is no longer available"，直接拦截并返回错误触发渠道重试！
// 若正常，将预读的数据无损拼接回 Response.Body，下游读取不受任何影响。
func CheckAndInterceptUnavailableResponse(httpResp *http.Response) *types.NewAPIError {
	if httpResp == nil || httpResp.Body == nil {
		return nil
	}
	peekBuf := make([]byte, 2048)
	n, _ := httpResp.Body.Read(peekBuf)
	if n <= 0 {
		return nil
	}
	peekData := peekBuf[:n]
	if IsClaudeUnavailablePrompt(string(peekData)) {
		_ = httpResp.Body.Close()
		return types.NewErrorWithStatusCode(
			fmt.Errorf("upstream returned unavailable model prompt: Claude Opus 4.6 is no longer available. Please switch to Claude Opus 5.5."),
			types.ErrorCodeBadResponseBody,
			http.StatusServiceUnavailable,
		)
	}

	httpResp.Body = &readCloserWithPrefix{
		Reader: io.MultiReader(bytes.NewReader(peekData), httpResp.Body),
		Closer: httpResp.Body,
	}
	return nil
}
