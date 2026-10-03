package common

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	// MaxClaudeAllowedImageDimension 官方/Vertex AI Claude 限制的最大单边像素为 8000
	MaxClaudeAllowedImageDimension = 8000
	// DefaultSafeImageDimension 安全目标尺寸，缩放到 7680px（标准 8K 分辨率），既保留极高清晰度又绝对不超限
	DefaultSafeImageDimension = 7680
)

var (
	imageDimensionExceededRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)image dimensions? exceed(?:s)? (?:the )?max(?:imum)? allowed size`),
		regexp.MustCompile(`(?i)exceed(?:s)? max(?:imum)? allowed size:\s*\d+\s*pixels`),
		regexp.MustCompile(`(?i)image.*exceeds?.*(?:8000|\d{4,5}).*pixels`),
		regexp.MustCompile(`(?i)at least one of the image dimensions exceed`),
	}
)

// IsImageDimensionExceededError 识别报错中是否包含图片宽高超限特征
func IsImageDimensionExceededError(errMsg string) bool {
	if errMsg == "" {
		return false
	}
	for _, reg := range imageDimensionExceededRegexes {
		if reg.MatchString(errMsg) {
			return true
		}
	}
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "8000 pixels") && strings.Contains(lower, "exceed")
}

// DownscaleImageBase64IfNeeded 检查 Base64 图片尺寸，如果宽或高超过 maxAllowedDim，则按比例平滑缩小并重新编码
func DownscaleImageBase64IfNeeded(dataStr string, maxAllowedDim int) (string, bool, error) {
	if len(dataStr) < 32 {
		return dataStr, false, nil
	}
	if maxAllowedDim <= 0 {
		maxAllowedDim = DefaultSafeImageDimension
	}

	prefix := ""
	cleanB64 := dataStr
	if idx := strings.Index(dataStr, ";base64,"); idx != -1 {
		prefix = dataStr[:idx+8]
		cleanB64 = dataStr[idx+8:]
	} else if strings.HasPrefix(dataStr, "data:") {
		if commaIdx := strings.Index(dataStr, ","); commaIdx != -1 {
			prefix = dataStr[:commaIdx+1]
			cleanB64 = dataStr[commaIdx+1:]
		}
	}
	cleanB64 = strings.TrimSpace(cleanB64)

	// 先通过 streaming base64 仅读取头部配置，微秒级检查尺寸，避免无谓解码整张大图
	b64Reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(cleanB64))
	cfg, format, err := image.DecodeConfig(b64Reader)
	if err != nil {
		// 尝试 webp config
		b64ReaderWebp := base64.NewDecoder(base64.StdEncoding, strings.NewReader(cleanB64))
		cfg, err = webp.DecodeConfig(b64ReaderWebp)
		if err == nil {
			format = "webp"
		}
	}

	if err != nil {
		// 无法识别为标准图片，保持原样
		return dataStr, false, nil
	}

	// 若未超限，直接返回原图
	if cfg.Width <= maxAllowedDim && cfg.Height <= maxAllowedDim {
		return dataStr, false, nil
	}

	// 超过限制，需要解码完整图片并等比例缩放
	imgBytes, decErr := base64.StdEncoding.DecodeString(cleanB64)
	if decErr != nil {
		return dataStr, false, decErr
	}

	var src image.Image
	src, format, err = image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		src, err = webp.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return dataStr, false, fmt.Errorf("failed to decode oversized image: %w", err)
		}
		format = "webp"
	}

	origW := src.Bounds().Dx()
	origH := src.Bounds().Dy()
	if origW <= maxAllowedDim && origH <= maxAllowedDim {
		return dataStr, false, nil
	}

	// 计算等比例缩放后的宽高
	maxEdge := origW
	if origH > maxEdge {
		maxEdge = origH
	}
	scale := float64(maxAllowedDim) / float64(maxEdge)
	targetW := int(float64(origW) * scale)
	targetH := int(float64(origH) * scale)
	if targetW < 1 {
		targetW = 1
	}
	if targetH < 1 {
		targetH = 1
	}

	// 使用高质量双线性插值进行缩放
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	var outBuf bytes.Buffer
	outMime := "image/jpeg"
	if format == "png" {
		outMime = "image/png"
		if encErr := png.Encode(&outBuf, dst); encErr != nil {
			return dataStr, false, encErr
		}
	} else {
		// JPEG 编码，质量 85，保证清晰度的同时大幅降低传输体积
		if encErr := jpeg.Encode(&outBuf, dst, &jpeg.Options{Quality: 85}); encErr != nil {
			return dataStr, false, encErr
		}
	}

	newB64 := base64.StdEncoding.EncodeToString(outBuf.Bytes())
	if prefix != "" {
		// 如果原有前缀指定的 mimeType 与新编码不一致，校准前缀
		if strings.HasPrefix(prefix, "data:image/") {
			prefix = fmt.Sprintf("data:%s;base64,", outMime)
		}
		return prefix + newB64, true, nil
	}

	return newB64, true, nil
}

// DownscaleOversizedImagesInClaudeRequest 对 Claude 请求对象中的图片进行超限检查与等比缩放
func DownscaleOversizedImagesInClaudeRequest(c *gin.Context, req *dto.ClaudeRequest, maxAllowedDim int) bool {
	if req == nil || len(req.Messages) == 0 {
		return false
	}
	modifiedAny := false

	for i := range req.Messages {
		msg := &req.Messages[i]
		if msg.Content == nil {
			continue
		}
		switch content := msg.Content.(type) {
		case []dto.ClaudeMediaMessage:
			for j := range content {
				block := &content[j]
				if block.Type == "image" && block.Source != nil && block.Source.Type == "base64" {
					if dataStr, ok := block.Source.Data.(string); ok && len(dataStr) > 0 {
						if newData, modified, err := DownscaleImageBase64IfNeeded(dataStr, maxAllowedDim); err == nil && modified {
							block.Source.Data = newData
							modifiedAny = true
						}
					}
				}
			}
		case []any:
			for _, item := range content {
				if blockMap, ok := item.(map[string]any); ok {
					bType := strings.ToLower(fmt.Sprintf("%v", blockMap["type"]))
					if bType == "image" {
						if source, ok := blockMap["source"].(map[string]any); ok && source != nil {
							if dataStr, ok := source["data"].(string); ok && len(dataStr) > 0 {
								if newData, modified, err := DownscaleImageBase64IfNeeded(dataStr, maxAllowedDim); err == nil && modified {
									source["data"] = newData
									modifiedAny = true
								}
							}
						}
					}
				}
			}
		}
	}

	if modifiedAny && c != nil {
		logger.LogWarn(c, fmt.Sprintf("检测到 Claude 请求包含超大图片（单边>8000px），已自动等比例缩放至安全尺寸（<=%dpx）", maxAllowedDim))
	}
	return modifiedAny
}

// DownscaleOversizedImagesInOpenAIRequest 对通用 OpenAI 请求对象中的图片进行超限检查与等比缩放
func DownscaleOversizedImagesInOpenAIRequest(c *gin.Context, req *dto.GeneralOpenAIRequest, maxAllowedDim int) bool {
	if req == nil || len(req.Messages) == 0 {
		return false
	}
	modifiedAny := false

	for i := range req.Messages {
		msg := &req.Messages[i]
		if msg.Content == nil {
			continue
		}
		switch content := msg.Content.(type) {
		case []dto.MediaContent:
			for j := range content {
				mc := &content[j]
				if mc.Type == "image_url" && mc.ImageUrl != nil {
					imgMedia := mc.GetImageMedia()
					if imgMedia != nil && imgMedia.Url != "" {
						if newUrl, modified, err := DownscaleImageBase64IfNeeded(imgMedia.Url, maxAllowedDim); err == nil && modified {
							imgMedia.Url = newUrl
							mc.ImageUrl = imgMedia
							modifiedAny = true
						}
					}
				}
			}
		case []any:
			for _, item := range content {
				if blockMap, ok := item.(map[string]any); ok {
					bType := strings.ToLower(fmt.Sprintf("%v", blockMap["type"]))
					if bType == "image_url" {
						if imgUrl, ok := blockMap["image_url"].(map[string]any); ok && imgUrl != nil {
							if urlStr, ok := imgUrl["url"].(string); ok && len(urlStr) > 0 {
								if newUrl, modified, err := DownscaleImageBase64IfNeeded(urlStr, maxAllowedDim); err == nil && modified {
									imgUrl["url"] = newUrl
									modifiedAny = true
								}
							}
						}
					}
				}
			}
		}
	}

	if modifiedAny && c != nil {
		logger.LogWarn(c, fmt.Sprintf("检测到 OpenAI 请求包含超大图片（单边>8000px），已自动等比例缩放至安全尺寸（<=%dpx）", maxAllowedDim))
	}
	return modifiedAny
}

// UniversalDownscaleImagesInRequest 全局通用图片尺寸自愈裁剪：检查任意请求对象中的超大图片并刷新 RequestBody
func UniversalDownscaleImagesInRequest(c *gin.Context, info *RelayInfo, maxAllowedDim int) bool {
	if info == nil || info.Request == nil {
		return false
	}
	if maxAllowedDim <= 0 {
		maxAllowedDim = DefaultSafeImageDimension
	}

	modified := false
	switch req := info.Request.(type) {
	case *dto.ClaudeRequest:
		modified = DownscaleOversizedImagesInClaudeRequest(c, req, maxAllowedDim)
	case *dto.GeneralOpenAIRequest:
		modified = DownscaleOversizedImagesInOpenAIRequest(c, req, maxAllowedDim)
	}

	if modified {
		_ = UpdatePrunedRequestBody(c, info)
		return true
	}
	return false
}

// EnsureImageSizeCleanInRawJSON 在原生 JSON 字节流中扫描并缩放超大图片
func EnsureImageSizeCleanInRawJSON(data []byte, maxAllowedDim int) ([]byte, bool) {
	if len(data) == 0 {
		return data, false
	}
	if maxAllowedDim <= 0 {
		maxAllowedDim = DefaultSafeImageDimension
	}
	// 快速预检：如果不包含 image 或 base64 关键字，跳过解析
	if !bytes.Contains(data, []byte(`"image"`)) && !bytes.Contains(data, []byte(`"image_url"`)) {
		return data, false
	}

	var root any
	if err := common.Unmarshal(data, &root); err != nil {
		return data, false
	}
	rootMap, ok := root.(map[string]any)
	if !ok {
		return data, false
	}

	modified := traverseAndDownscaleImagesInJSONNode(rootMap, maxAllowedDim)
	if !modified {
		return data, false
	}
	newBytes, err := common.Marshal(rootMap)
	if err != nil {
		return data, false
	}
	return newBytes, true
}

func traverseAndDownscaleImagesInJSONNode(node any, maxAllowedDim int) bool {
	if node == nil {
		return false
	}
	modifiedAny := false
	switch val := node.(type) {
	case map[string]any:
		bType := strings.ToLower(fmt.Sprintf("%v", val["type"]))
		if bType == "image" {
			if source, ok := val["source"].(map[string]any); ok && source != nil {
				if dataStr, ok := source["data"].(string); ok && len(dataStr) > 0 {
					if newData, modified, err := DownscaleImageBase64IfNeeded(dataStr, maxAllowedDim); err == nil && modified {
						source["data"] = newData
						modifiedAny = true
					}
				}
			}
		} else if bType == "image_url" {
			if imgUrl, ok := val["image_url"].(map[string]any); ok && imgUrl != nil {
				if urlStr, ok := imgUrl["url"].(string); ok && len(urlStr) > 0 {
					if newUrl, modified, err := DownscaleImageBase64IfNeeded(urlStr, maxAllowedDim); err == nil && modified {
						imgUrl["url"] = newUrl
						modifiedAny = true
					}
				}
			}
		}
		for _, child := range val {
			if traverseAndDownscaleImagesInJSONNode(child, maxAllowedDim) {
				modifiedAny = true
			}
		}
	case []any:
		for _, item := range val {
			if traverseAndDownscaleImagesInJSONNode(item, maxAllowedDim) {
				modifiedAny = true
			}
		}
	}
	return modifiedAny
}
