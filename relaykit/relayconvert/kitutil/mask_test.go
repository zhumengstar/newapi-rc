package kitutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskSensitiveInfo_NoAvailableChannel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Upstream no available channel with distributor and request id",
			input:    "status_code=503, No available channel for model gemini-4-argon under group Gemini老客折扣0.1x (distributor) (request id: 202610310117494397305458268d9d6yFErqulo)",
			expected: "status_code=503, The model is temporarily overloaded or unavailable. Please try again later. (request id: 202610310117494397305458268d9d6yFErqulo)",
		},
		{
			name:     "Plain error without status_code",
			input:    "No available channel for model gemini-4-argon under group Gemini老客折扣0.1x (distributor)",
			expected: "The model is temporarily overloaded or unavailable. Please try again later.",
		},
		{
			name:     "English channel missing with standard group",
			input:    "No available channel for model gpt-4 under group default",
			expected: "The model is temporarily overloaded or unavailable. Please try again later.",
		},
		{
			name:     "Internal Chinese channel missing error",
			input:    "分组 Gemini4按量组 下模型 gemini-4-argon 的可用渠道不存在（retry）",
			expected: "模型当前服务能力暂不可用，请稍后重试",
		},
		{
			name:     "Internal Chinese channel fetch failure error",
			input:    "获取分组 Gemini4按量组 下模型 gemini-4-argon 的可用渠道失败（retry）",
			expected: "模型当前服务能力暂不可用，请稍后重试",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := MaskSensitiveInfo(tt.input)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
