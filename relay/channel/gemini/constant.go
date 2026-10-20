package gemini

import (
	"fmt"
	"strings"
)

var ModelList = []string{
	// stable version
	"gemini-2.5-flash", "gemini-2.5-pro", "gemini-2.0-flash",
	"gemini-2.0-flash-001", "gemini-2.0-flash-lite-001", "gemini-2.0-flash-lite",
	"gemini-2.5-flash-lite",
	"gemini-3-pro-image", "gemini-3.1-flash-image",
	// latest version
	"gemini-flash-latest", "gemini-flash-lite-latest", "gemini-pro-latest",
	"gemini-2.5-flash-native-audio-latest",
	// preview version
	"gemini-2.5-flash-preview-tts", "gemini-2.5-pro-preview-tts",
	"gemini-2.5-flash-image", "gemini-2.5-flash-lite-preview-09-2025",
	"gemini-3-pro-preview", "gemini-3-flash-preview", "gemini-3.1-pro-preview",
	"gemini-3.1-pro-preview-customtools", "gemini-3.1-flash-lite-preview",
	"gemini-3-pro-image-preview", "nano-banana-pro-preview",
	"gemini-3.1-flash-image-preview", "gemini-robotics-er-1.5-preview",
	"gemini-2.5-computer-use-preview-10-2025", "deep-research-pro-preview-12-2025",
	"gemini-2.5-flash-native-audio-preview-09-2025", "gemini-2.5-flash-native-audio-preview-12-2025",
	// gemma models
	"gemma-3-1b-it", "gemma-3-4b-it", "gemma-3-12b-it",
	"gemma-3-27b-it", "gemma-3n-e4b-it", "gemma-3n-e2b-it",
	// embedding models
	"gemini-embedding-001", "gemini-embedding-2-preview",
	// imagen models
	"imagen-4.0-generate-001", "imagen-4.0-ultra-generate-001",
	"imagen-4.0-fast-generate-001",
	// veo models
	"veo-2.0-generate-001", "veo-3.0-generate-001", "veo-3.0-fast-generate-001",
	"veo-3.1-generate-preview", "veo-3.1-fast-generate-preview",
	// other models
	"aqa",
}

var SafetySettingList = []string{
	"HARM_CATEGORY_HARASSMENT",
	"HARM_CATEGORY_HATE_SPEECH",
	"HARM_CATEGORY_SEXUALLY_EXPLICIT",
	"HARM_CATEGORY_DANGEROUS_CONTENT",
	//"HARM_CATEGORY_CIVIC_INTEGRITY", This item is deprecated!
}

var ChannelName = "google gemini"

func IsGeminiBlockReason(reason string) bool {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "SAFETY", "PROHIBITED_CONTENT", "RECITATION", "BLOCKLIST", "SPII", "OTHER":
		return true
	default:
		return false
	}
}

func FormatGeminiBlockRefusalText(reason string, finishMessage *string) string {
	msg := ""
	if finishMessage != nil && strings.TrimSpace(*finishMessage) != "" {
		msg = strings.TrimSpace(*finishMessage)
	}
	if msg != "" {
		return fmt.Sprintf("抱歉，由于触发了 Google 内容安全审查策略 (%s)，模型未生成回答：%s", reason, msg)
	}
	return fmt.Sprintf("抱歉，由于触发了 Google Gemini 内容安全审查策略 (%s)，请求已被拦截，无法生成回答。请调整您的提示词后重试。", reason)
}
