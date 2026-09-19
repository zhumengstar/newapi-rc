package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if err := relayconvert.ApplyGeminiThinkingConfigChecked(request, info); err != nil {
		return nil, err
	}
	if len(request.Contents) > 0 {
		for i, content := range request.Contents {
			if i == 0 {
				if request.Contents[0].Role == "" {
					request.Contents[0].Role = "user"
				}
			}
			for _, part := range content.Parts {
				if part.FileData != nil {
					if part.FileData.MimeType == "" && strings.Contains(part.FileData.FileUri, "www.youtube.com") {
						part.FileData.MimeType = "video/webm"
					}
				}
			}
		}
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	result, err := service.ConvertRequest(c, info, types.RelayFormatGemini, req)
	if err != nil {
		return nil, err
	}
	geminiRequest, ok := result.Value.(*dto.GeminiChatRequest)
	if !ok {
		return nil, fmt.Errorf("expected Gemini generateContent request, got %T", result.Value)
	}
	return geminiRequest, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		// convert size to aspect ratio but allow user to specify aspect ratio
		aspectRatio := "1:1" // default aspect ratio
		size := strings.TrimSpace(request.Size)
		if size != "" {
			if strings.Contains(size, ":") {
				aspectRatio = size
			} else {
				switch size {
				case "256x256", "512x512", "1024x1024":
					aspectRatio = "1:1"
				case "1536x1024":
					aspectRatio = "3:2"
				case "1024x1536":
					aspectRatio = "2:3"
				case "1024x1792":
					aspectRatio = "9:16"
				case "1792x1024":
					aspectRatio = "16:9"
				}
			}
		}

		// build gemini imagen request
		geminiRequest := dto.GeminiImageRequest{
			Instances: []dto.GeminiImageInstance{
				{
					Prompt: request.Prompt,
				},
			},
			Parameters: dto.GeminiImageParameters{
				SampleCount:      int(lo.FromPtrOr(request.N, uint(1))),
				AspectRatio:      aspectRatio,
				PersonGeneration: "allow_adult", // default allow adult
			},
		}

		// Set imageSize when quality parameter is specified
		// Map quality parameter to imageSize (only supported by Standard and Ultra models)
		// quality values: auto, high, medium, low (for gpt-image-1), hd, standard (for dall-e-3)
		// imageSize values: 1K (default), 2K, 4K
		// https://ai.google.dev/gemini-api/docs/imagen
		// https://platform.openai.com/docs/api-reference/images/create
		if request.Quality != "" {
			imageSize := "1K" // default
			q := strings.ToLower(strings.TrimSpace(request.Quality))
			switch q {
			case "hd", "high", "4k":
				imageSize = "4K"
			case "2k", "medium":
				imageSize = "2K"
			case "standard", "low", "auto", "1k":
				imageSize = "1K"
			default:
				// unknown quality value, default to 1K
				imageSize = "1K"
			}
			geminiRequest.Parameters.ImageSize = imageSize
		}

		return geminiRequest, nil
	}

	// For Gemini multimodal generation models (gemini-3-pro-image-preview, gemini-3.1-flash-image-preview, gemini-2.0-flash-exp, etc.)
	aspectRatio := "1:1"
	size := strings.TrimSpace(request.Size)
	if size != "" {
		if strings.Contains(size, ":") {
			aspectRatio = size
		} else {
			switch size {
			case "256x256", "512x512", "1024x1024", "2048x2048", "4096x4096":
				aspectRatio = "1:1"
			case "1536x1024", "2528x1696", "5056x3392":
				aspectRatio = "3:2"
			case "1024x1536", "1696x2528", "3392x5056":
				aspectRatio = "2:3"
			case "1024x1792", "2160x3840", "3072x5504":
				aspectRatio = "9:16"
			case "1792x1024", "3840x2160", "5504x3072":
				aspectRatio = "16:9"
			case "1200x896", "2400x1792", "4800x3584":
				aspectRatio = "4:3"
			case "896x1200", "1792x2400", "3584x4800":
				aspectRatio = "3:4"
			case "1152x928", "2304x1856", "4608x3712":
				aspectRatio = "5:4"
			case "928x1152", "1856x2304", "3712x4608":
				aspectRatio = "4:5"
			case "1584x672", "3168x1344", "6336x2688", "2560x1080":
				aspectRatio = "21:9"
			}
		}
	}

	imageSize := "1K"
	q := strings.ToLower(strings.TrimSpace(request.Quality))
	sizeLower := strings.ToLower(size)
	modelLower := strings.ToLower(info.UpstreamModelName)
	if q == "4k" || q == "hd" || q == "high" || strings.Contains(sizeLower, "4k") || strings.Contains(size, "3840") || strings.Contains(size, "2160") || strings.Contains(size, "4096") || strings.Contains(size, "5504") || strings.Contains(modelLower, "4k") {
		imageSize = "4K"
	} else if q == "2k" || q == "medium" || strings.Contains(sizeLower, "2k") || strings.Contains(size, "2048") || strings.Contains(size, "2560") || strings.Contains(modelLower, "2k") {
		imageSize = "2K"
	} else if q == "512" || strings.Contains(size, "512") {
		imageSize = "512"
	}

	imgConfigObj := map[string]string{
		"aspectRatio": aspectRatio,
		"imageSize":   imageSize,
	}
	imgConfigBytes, _ := json.Marshal(imgConfigObj)

	prompt := request.Prompt
	var specs []string
	if request.Size != "" && request.Size != "auto" {
		specs = append(specs, fmt.Sprintf("resolution %s", request.Size))
		if imageSize == "4K" {
			specs = append(specs, "4K UHD ultra-high definition")
		}
		if aspectRatio != "" {
			specs = append(specs, fmt.Sprintf("%s aspect ratio", aspectRatio))
		}
	}
	if imageSize == "4K" {
		specs = append(specs, "masterpiece, ultra-detailed, high fidelity")
	}
	if len(specs) > 0 {
		prompt = fmt.Sprintf("%s [Specification: %s]", prompt, strings.Join(specs, ", "))
	}

	geminiRequest := dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{
				Role: "user",
				Parts: []dto.GeminiPart{
					{
						Text: prompt,
					},
				},
			},
		},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
			ImageConfig:        imgConfigBytes,
		},
	}
	return geminiRequest, nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {

}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {

	version := model_setting.GetGeminiVersionSetting(info.UpstreamModelName)

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return fmt.Sprintf("%s/%s/models/%s:predict", info.ChannelBaseUrl, version, info.UpstreamModelName), nil
	}

	if info.RelayMode == constant.RelayModeImagesGenerations || info.RelayMode == constant.RelayModeImagesEdits {
		return fmt.Sprintf("%s/%s/models/%s:generateContent", info.ChannelBaseUrl, version, info.UpstreamModelName), nil
	}

	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		action := "embedContent"
		if info.IsGeminiBatchEmbedding {
			action = "batchEmbedContents"
		}
		return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
	}

	action := "generateContent"
	if info.IsStream {
		action = "streamGenerateContent?alt=sse"
		if info.RelayMode == constant.RelayModeGemini {
			info.DisablePing = true
		}
	}
	return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-goog-api-key", info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	result, err := service.ConvertRequest(c, info, types.RelayFormatGemini, request)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	if request.Input == nil {
		return nil, errors.New("input is required")
	}

	inputs := request.ParseInput()
	if len(inputs) == 0 {
		return nil, errors.New("input is empty")
	}
	// We always build a batch-style payload with `requests`, so ensure we call the
	// batch endpoint upstream to avoid payload/endpoint mismatches.
	info.IsGeminiBatchEmbedding = true
	// process all inputs
	geminiRequests := make([]map[string]any, 0, len(inputs))
	for _, input := range inputs {
		geminiRequest := map[string]any{
			"model": fmt.Sprintf("models/%s", info.UpstreamModelName),
			"content": dto.GeminiChatContent{
				Parts: []dto.GeminiPart{
					{
						Text: input,
					},
				},
			},
		}

		// set specific parameters for different models
		// https://ai.google.dev/api/embeddings?hl=zh-cn#method:-models.embedcontent
		switch info.UpstreamModelName {
		case "text-embedding-004", "gemini-embedding-exp-03-07", "gemini-embedding-001":
			// Only newer models introduced after 2024 support OutputDimensionality
			dimensions := lo.FromPtrOr(request.Dimensions, 0)
			if dimensions > 0 {
				geminiRequest["outputDimensionality"] = dimensions
			}
		}
		geminiRequests = append(geminiRequests, geminiRequest)
	}

	return map[string]any{
		"requests": geminiRequests,
	}, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	result, err := service.ConvertRequest(c, info, types.RelayFormatGemini, &request)
	if err != nil {
		return nil, err
	}
	geminiRequest, ok := result.Value.(*dto.GeminiChatRequest)
	if !ok {
		return nil, fmt.Errorf("expected Gemini generateContent request, got %T", result.Value)
	}
	return geminiRequest, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == constant.RelayModeResponses {
		if info.IsStream {
			return GeminiResponsesStreamHandler(c, info, resp)
		}
		return GeminiResponsesHandler(c, info, resp)
	}

	if info.RelayMode == constant.RelayModeGemini {
		if strings.Contains(info.RequestURLPath, ":embedContent") ||
			strings.Contains(info.RequestURLPath, ":batchEmbedContents") {
			return NativeGeminiEmbeddingHandler(c, resp, info)
		}
		if info.IsStream {
			return GeminiTextGenerationStreamHandler(c, info, resp)
		} else {
			return GeminiTextGenerationHandler(c, info, resp)
		}
	}

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return GeminiImageHandler(c, info, resp)
	}

	// check if the model is an embedding model
	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		return GeminiEmbeddingHandler(c, info, resp)
	}

	if info.IsStream {
		return GeminiChatStreamHandler(c, info, resp)
	} else {
		return GeminiChatHandler(c, info, resp)
	}

}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
