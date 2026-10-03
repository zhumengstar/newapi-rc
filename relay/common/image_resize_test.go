package common

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsImageDimensionExceededError(t *testing.T) {
	testCases := []struct {
		errMsg   string
		expected bool
	}{
		{
			errMsg:   `status_code=400, {"type":"error","error":{"type":"invalid_request_error","message":"***.***.***.***.***.***.***.data: At least one of the image dimensions exceed max allowed size: 8000 pixels"},"request_id":"req_vrtx_011Cffs2jdRtgeU2a99dkw8S"}`,
			expected: true,
		},
		{
			errMsg:   `At least one of the image dimensions exceed max allowed size: 8000 pixels`,
			expected: true,
		},
		{
			errMsg:   `Image dimension exceeds the maximum allowed size`,
			expected: true,
		},
		{
			errMsg:   `The input token count exceeds the maximum number of tokens allowed`,
			expected: false,
		},
		{
			errMsg:   `status_code=401, Invalid API key`,
			expected: false,
		},
	}

	for _, tc := range testCases {
		assert.Equal(t, tc.expected, IsImageDimensionExceededError(tc.errMsg), "failed for: "+tc.errMsg)
	}
}

func createTestPNG(width, height int) string {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// 填色
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestDownscaleImageBase64IfNeeded(t *testing.T) {
	// 1. 创建超大图片：9000 x 4500
	t.Run("oversized image gets downscaled proportionally", func(t *testing.T) {
		b64 := createTestPNG(9000, 4500)
		dataURL := "data:image/png;base64," + b64

		newURL, modified, err := DownscaleImageBase64IfNeeded(dataURL, 7680)
		require.NoError(t, err)
		assert.True(t, modified)
		assert.Contains(t, newURL, "data:image/png;base64,")

		// 验证缩放后尺寸
		clean := newURL[len("data:image/png;base64,"):]
		decoded, decErr := base64.StdEncoding.DecodeString(clean)
		require.NoError(t, decErr)

		cfg, _, cfgErr := image.DecodeConfig(bytes.NewReader(decoded))
		require.NoError(t, cfgErr)
		assert.Equal(t, 7680, cfg.Width)
		assert.Equal(t, 3840, cfg.Height)
	})

	// 2. 正常尺寸图片不触发缩放
	t.Run("normal size image remains unchanged", func(t *testing.T) {
		b64 := createTestPNG(800, 600)
		dataURL := "data:image/png;base64," + b64

		newURL, modified, err := DownscaleImageBase64IfNeeded(dataURL, 7680)
		require.NoError(t, err)
		assert.False(t, modified)
		assert.Equal(t, dataURL, newURL)
	})
}

func TestDownscaleOversizedImagesInClaudeRequest(t *testing.T) {
	b64 := createTestPNG(8500, 4250)
	req := &dto.ClaudeRequest{
		Model: "claude-3-7-sonnet-20250219",
		Messages: []dto.ClaudeMessage{
			{
				Role: "user",
				Content: []dto.ClaudeMediaMessage{
					{
						Type: "image",
						Source: &dto.ClaudeMessageSource{
							Type:      "base64",
							MediaType: "image/png",
							Data:      b64,
						},
					},
				},
			},
		},
	}

	modified := DownscaleOversizedImagesInClaudeRequest(nil, req, 7680)
	assert.True(t, modified)

	mediaMsg := req.Messages[0].Content.([]dto.ClaudeMediaMessage)[0]
	newB64 := mediaMsg.Source.Data.(string)
	assert.NotEqual(t, b64, newB64)

	decoded, _ := base64.StdEncoding.DecodeString(newB64)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(decoded))
	require.NoError(t, err)
	assert.Equal(t, 7680, cfg.Width)
	assert.Equal(t, 3840, cfg.Height)
}

func TestEnsureImageSizeCleanInRawJSON(t *testing.T) {
	b64 := createTestPNG(9000, 4500)
	inputJSON := `{
		"model": "claude-3-7-sonnet-20250219",
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "image",
						"source": {
							"type": "base64",
							"media_type": "image/png",
							"data": "` + b64 + `"
						}
					}
				]
			}
		]
	}`

	cleaned, modified := EnsureImageSizeCleanInRawJSON([]byte(inputJSON), 7680)
	assert.True(t, modified)
	assert.NotContains(t, string(cleaned), b64)
}
