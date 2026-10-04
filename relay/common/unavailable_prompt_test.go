package common

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsClaudeUnavailablePrompt(t *testing.T) {
	assert.True(t, IsClaudeUnavailablePrompt("Claude Opus 4.6 is no longer available. Please switch to Claude Opus 5.5."))
	assert.True(t, IsClaudeUnavailablePrompt("error: claude opus 4.6 is no longer available"))
	assert.True(t, IsClaudeUnavailablePrompt("Please switch to Claude Opus 5.5 now."))
	assert.False(t, IsClaudeUnavailablePrompt("Hello, how can I help you today?"))
}

func TestCleanUnavailablePromptFromText(t *testing.T) {
	orig := "Hello. Claude Opus 4.6 is no longer available. Please switch to Claude Opus 5.5. What is 1+1?"
	cleaned, modified := CleanUnavailablePromptFromText(orig)
	assert.True(t, modified)
	assert.Equal(t, "Hello. What is 1+1?", cleaned)

	normal := "What is 1+1?"
	cleaned2, modified2 := CleanUnavailablePromptFromText(normal)
	assert.False(t, modified2)
	assert.Equal(t, normal, cleaned2)
}

func TestCheckAndInterceptUnavailableResponse(t *testing.T) {
	// 1. Intercept test
	body1 := io.NopCloser(bytes.NewReader([]byte("data: {\"type\":\"message_start\"}\ndata: Claude Opus 4.6 is no longer available. Please switch to Claude Opus 5.5.")))
	resp1 := &http.Response{
		StatusCode: http.StatusOK,
		Body:       body1,
	}
	err1 := CheckAndInterceptUnavailableResponse(resp1)
	require.NotNil(t, err1)
	assert.Equal(t, http.StatusServiceUnavailable, err1.StatusCode)

	// 2. Normal response passthrough test
	normalContent := []byte("data: {\"type\":\"message_start\"}\ndata: {\"delta\":{\"text\":\"Hello\"}}")
	body2 := io.NopCloser(bytes.NewReader(normalContent))
	resp2 := &http.Response{
		StatusCode: http.StatusOK,
		Body:       body2,
	}
	err2 := CheckAndInterceptUnavailableResponse(resp2)
	require.Nil(t, err2)

	// Verify that the body can still be read completely and accurately
	readAll, rErr := io.ReadAll(resp2.Body)
	require.NoError(t, rErr)
	assert.Equal(t, normalContent, readAll)
}
