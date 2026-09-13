package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
)

const (
	generatedImageAssetsContextKey = "generated_image_log_assets"
	generatedImageRetention        = 24 * time.Hour
	generatedImageMaxBytes         = 20 << 20
	generatedImageDownloadTimeout  = 15 * time.Second
)

var generatedImageAssetsDir = func() string {
	if dir := os.Getenv("GENERATED_IMAGES_DIR"); dir != "" {
		return dir
	}
	if _, err := os.Stat("/data"); err == nil {
		return "/data/generated-images"
	}
	return filepath.Join(os.TempDir(), "new-api-generated-images")
}()

type GeneratedImageLogAsset struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
	Size      int64  `json:"size,omitempty"`
	MimeType  string `json:"mime_type,omitempty"`
}

func AttachGeneratedImageLogAssets(c *gin.Context, images []dto.ImageData) {
	assets := persistGeneratedImageAssets(c.Request.Context(), images)
	if len(assets) == 0 {
		return
	}
	c.Set(generatedImageAssetsContextKey, assets)
}

func AttachGeneratedImageLogAssetsFromResponse(c *gin.Context, responseBody []byte) {
	if len(responseBody) == 0 {
		return
	}
	var imageResponse dto.ImageResponse
	if err := common.Unmarshal(responseBody, &imageResponse); err != nil || len(imageResponse.Data) == 0 {
		return
	}
	AttachGeneratedImageLogAssets(c, imageResponse.Data)
}

func GetGeneratedImageLogAssets(c *gin.Context) []GeneratedImageLogAsset {
	value, exists := c.Get(generatedImageAssetsContextKey)
	if !exists {
		return nil
	}
	assets, ok := value.([]GeneratedImageLogAsset)
	if !ok {
		return nil
	}
	return assets
}

func GeneratedImageAssetFilePath(date string, filename string) string {
	return filepath.Join(generatedImageAssetsDir, date, filename)
}

func GeneratedImageAssetURL(date string, filename string) string {
	return "/api/log/generated-images/" + date + "/" + filename
}

func persistGeneratedImageAssets(ctx context.Context, images []dto.ImageData) []GeneratedImageLogAsset {
	if len(images) == 0 {
		return nil
	}
	cleanupExpiredGeneratedImageAssets()

	date := time.Now().Format("20060102")
	dir := filepath.Join(generatedImageAssetsDir, date)
	if err := os.MkdirAll(dir, 0750); err != nil {
		common.SysLog("failed to create generated image asset dir: " + err.Error())
		return nil
	}

	assets := make([]GeneratedImageLogAsset, 0, len(images))
	for index, image := range images {
		data, mimeType, err := readGeneratedImageData(ctx, image)
		if err != nil {
			common.SysLog("failed to persist generated image asset: " + err.Error())
			continue
		}
		if len(data) == 0 || len(data) > generatedImageMaxBytes {
			common.SysLog(fmt.Sprintf("skip generated image asset with invalid size: %d", len(data)))
			continue
		}
		if mimeType == "" {
			mimeType = http.DetectContentType(data)
		}
		ext := generatedImageExt(mimeType)
		if ext == "" {
			ext = ".bin"
		}
		filename := fmt.Sprintf("%d-%02d-%s%s", time.Now().UnixNano(), index, randomHex(8), ext)
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, data, 0640); err != nil {
			common.SysLog("failed to write generated image asset: " + err.Error())
			continue
		}
		assets = append(assets, GeneratedImageLogAsset{
			URL:       GeneratedImageAssetURL(date, filename),
			ExpiresAt: time.Now().Add(generatedImageRetention).Unix(),
			Size:      int64(len(data)),
			MimeType:  mimeType,
		})
	}
	return assets
}

func readGeneratedImageData(ctx context.Context, image dto.ImageData) ([]byte, string, error) {
	if strings.TrimSpace(image.B64Json) != "" {
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(image.B64Json))
		if err != nil {
			return nil, "", err
		}
		return data, http.DetectContentType(data), nil
	}

	url := strings.TrimSpace(image.Url)
	if url == "" {
		return nil, "", errors.New("empty generated image source")
	}
	if strings.HasPrefix(url, "data:") {
		return decodeGeneratedImageDataURL(url)
	}
	return downloadGeneratedImage(ctx, url)
}

func decodeGeneratedImageDataURL(dataURL string) ([]byte, string, error) {
	comma := strings.Index(dataURL, ",")
	if comma < 0 || comma+1 >= len(dataURL) {
		return nil, "", errors.New("invalid generated image data url")
	}
	header := dataURL[:comma]
	payload := dataURL[comma+1:]
	if !strings.Contains(header, ";base64") {
		return nil, "", errors.New("generated image data url is not base64")
	}
	mimeType := strings.TrimPrefix(strings.Split(header, ";")[0], "data:")
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", err
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	return data, mimeType, nil
}

func downloadGeneratedImage(ctx context.Context, rawURL string) ([]byte, string, error) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return nil, "", errors.New("unsupported generated image url")
	}
	reqCtx, cancel := context.WithTimeout(ctx, generatedImageDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer CloseResponseBodyGracefully(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download generated image returned status %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, generatedImageMaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if len(data) > generatedImageMaxBytes {
		return nil, "", errors.New("generated image is too large")
	}
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	return data, strings.Split(mimeType, ";")[0], nil
}

func generatedImageExt(mimeType string) string {
	switch strings.ToLower(strings.Split(mimeType, ";")[0]) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ""
	}
}

func cleanupExpiredGeneratedImageAssets() {
	cutoff := time.Now().Add(-generatedImageRetention)
	entries, err := os.ReadDir(generatedImageAssetsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(generatedImageAssetsDir, entry.Name())
		if entry.IsDir() {
			_ = filepath.WalkDir(path, func(filePath string, d os.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() {
					return nil
				}
				info, err := d.Info()
				if err == nil && info.ModTime().Before(cutoff) {
					_ = os.Remove(filePath)
				}
				return nil
			})
			_ = os.Remove(path)
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}
	}
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
