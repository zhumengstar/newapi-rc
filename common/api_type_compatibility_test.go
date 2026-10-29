package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestIsAPITypeCompatible(t *testing.T) {
	// OpenAI compatible types
	assert.True(t, IsAPITypeOpenAICompatible(constant.APITypeOpenAI))
	assert.True(t, IsAPITypeOpenAICompatible(constant.APITypeNewAPI))
	assert.True(t, IsAPITypeOpenAICompatible(constant.APITypeDeepSeek))
	assert.True(t, IsAPITypeOpenAICompatible(constant.APITypeSiliconFlow))
	assert.True(t, IsAPITypeOpenAICompatible(constant.APITypeOpenRouter))

	// Gemini and Claude must NOT be treated as OpenAI compatible
	assert.False(t, IsAPITypeOpenAICompatible(constant.APITypeGemini))
	assert.False(t, IsAPITypeOpenAICompatible(constant.APITypeAnthropic))
	assert.False(t, IsAPITypeOpenAICompatible(constant.APITypeAws))

	// Gemini compatibility
	assert.True(t, IsAPITypeGeminiCompatible(constant.APITypeGemini))
	assert.False(t, IsAPITypeGeminiCompatible(constant.APITypeOpenAI))
	assert.False(t, IsAPITypeGeminiCompatible(constant.APITypeAnthropic))

	// Claude compatibility
	assert.True(t, IsAPITypeClaudeCompatible(constant.APITypeAnthropic))
	assert.False(t, IsAPITypeClaudeCompatible(constant.APITypeOpenAI))
	assert.False(t, IsAPITypeClaudeCompatible(constant.APITypeGemini))
}
