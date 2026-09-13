package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/webp"
)

type GeneratedImageInfo struct {
	URL      string `json:"url"`
	Size     int64  `json:"size,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

func RecordGeneratedImages(c *gin.Context, info *relaycommon.RelayInfo, images []dto.ImageData, usage *dto.Usage) []GeneratedImageInfo {
	if len(images) == 0 || info == nil {
		return nil
	}
	if recorded, exists := c.Get("generated_images_recorded"); exists && recorded == true {
		return nil
	}
	c.Set("generated_images_recorded", true)

	quality, size := generatedImageCallMeta(info)
	c.Set("image_generation_call", true)
	c.Set("image_generation_call_quality", quality)
	c.Set("image_generation_call_size", size)
	if usage != nil {
		imageTokens := usage.CompletionTokenDetails.ImageTokens
		if imageTokens == 0 {
			imageTokens = usage.CompletionTokens
		}
		usage.CompletionTokenDetails.ImageTokens = imageTokens
	}

	generated := make([]GeneratedImageInfo, 0, len(images))
	for _, imageData := range images {
		imageInfo := saveGeneratedImage(c, imageData.B64Json)
		if imageInfo.URL == "" {
			continue
		}
		generated = append(generated, imageInfo)
	}
	if len(generated) == 0 {
		return nil
	}

	c.Set("generated_images", generated)

	now := time.Now().UnixMilli()
	submitTime := info.StartTime.UnixMilli()
	if submitTime <= 0 {
		submitTime = now
	}
	taskID := "generated-image-" + c.GetString(common.RequestIdKey)
	prompt := GeneratedImagePrompt(info)
	task := &model.Midjourney{
		Code:        1,
		UserId:      info.UserId,
		Action:      "IMAGE_GENERATION",
		MjId:        taskID,
		Prompt:      prompt,
		PromptEn:    prompt,
		Description: "Generated image via chat completions",
		State:       info.OriginModelName,
		SubmitTime:  submitTime,
		StartTime:   submitTime,
		FinishTime:  now,
		ImageUrl:    generated[0].URL,
		Status:      "SUCCESS",
		Progress:    "100%",
		ChannelId:   info.ChannelId,
	}
	upsertGeneratedImageTask(c, task)
	return generated
}

func upsertGeneratedImageTask(c *gin.Context, task *model.Midjourney) {
	if task == nil {
		return
	}

	var existing model.Midjourney
	err := model.DB.
		Where("user_id = ? AND channel_id = ? AND action = ? AND prompt = ? AND status = ?",
			task.UserId, task.ChannelId, task.Action, task.Prompt, task.Status).
		Order("id desc").
		First(&existing).Error
	if err == nil && existing.Id > 0 {
		task.Id = existing.Id
		if err := task.Update(); err != nil {
			logger.LogError(c, "failed to update generated image task: "+err.Error())
		}
	} else {
		if err := task.Insert(); err != nil {
			logger.LogError(c, "failed to record generated image task: "+err.Error())
			return
		}
	}

	if task.Id > 0 {
		if err := model.DB.
			Where("user_id = ? AND channel_id = ? AND action = ? AND prompt = ? AND status = ? AND id <> ?",
				task.UserId, task.ChannelId, task.Action, task.Prompt, task.Status, task.Id).
			Delete(&model.Midjourney{}).Error; err != nil {
			logger.LogError(c, "failed to cleanup duplicated generated image tasks: "+err.Error())
		}
	}
}

func GeneratedImagePrompt(info *relaycommon.RelayInfo) string {
	if info == nil {
		return ""
	}
	request, ok := info.Request.(*dto.GeneralOpenAIRequest)
	if !ok || request == nil {
		if imageRequest, ok := info.Request.(*dto.ImageRequest); ok && imageRequest != nil {
			return strings.TrimSpace(imageRequest.Prompt)
		}
		return ""
	}
	texts := make([]string, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.IsStringContent() {
			if text := strings.TrimSpace(message.StringContent()); text != "" {
				texts = append(texts, text)
			}
			continue
		}
		for _, content := range message.ParseContent() {
			if content.Type == dto.ContentTypeText && strings.TrimSpace(content.Text) != "" {
				texts = append(texts, strings.TrimSpace(content.Text))
			}
		}
	}
	return strings.Join(texts, "\n")
}

func generatedImageCallMeta(info *relaycommon.RelayInfo) (string, string) {
	if info == nil {
		return "high", "4k"
	}
	if request, ok := info.Request.(*dto.ImageRequest); ok && request != nil {
		quality := strings.TrimSpace(request.Quality)
		size := strings.TrimSpace(request.Size)
		if quality == "" {
			quality = "high"
		}
		if size == "" {
			size = "4k"
		}
		return quality, size
	}
	return "high", "4k"
}

func saveGeneratedImage(c *gin.Context, b64 string) GeneratedImageInfo {
	if idx := strings.Index(b64, ","); idx >= 0 {
		b64 = b64[idx+1:]
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		logger.LogError(c, "failed to decode generated image: "+err.Error())
		return GeneratedImageInfo{}
	}
	mimeType := http.DetectContentType(data)
	config, _, configErr := image.DecodeConfig(bytes.NewReader(data))
	if configErr != nil {
		logger.LogError(c, "failed to read generated image size: "+configErr.Error())
	}
	dateDir := time.Now().Format("20060102")
	ext := generatedImageExtension(mimeType)
	fileName := fmt.Sprintf("%s%s", common.GetUUID(), ext)
	dir := filepath.Join(generatedImageAssetsDir, dateDir)
	if err = os.MkdirAll(dir, 0750); err != nil {
		logger.LogError(c, "failed to create generated image dir: "+err.Error())
		return GeneratedImageInfo{}
	}
	if err = os.WriteFile(filepath.Join(dir, fileName), data, 0640); err != nil {
		logger.LogError(c, "failed to write generated image: "+err.Error())
		return GeneratedImageInfo{}
	}
	return GeneratedImageInfo{
		URL:      "/api/log/generated-images/" + dateDir + "/" + fileName,
		Size:     int64(len(data)),
		MimeType: mimeType,
		Width:    config.Width,
		Height:   config.Height,
	}
}

func generatedImageExtension(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".jpg"
	}
}
