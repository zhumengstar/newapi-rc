package relay

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
)

func GeminiHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	geminiReq, ok := info.Request.(*dto.GeminiChatRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.GeminiChatRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(geminiReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeminiChatRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	// model mapped 模型映射
	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return newConvertRequestFailedError(c, info, err)
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}

	adaptor.Init(info)

	if info.ChannelSetting.SystemPrompt != "" {
		if request.SystemInstructions == nil {
			request.SystemInstructions = &dto.GeminiChatContent{
				Parts: []dto.GeminiPart{
					{Text: info.ChannelSetting.SystemPrompt},
				},
			}
		} else if len(request.SystemInstructions.Parts) == 0 {
			request.SystemInstructions.Parts = []dto.GeminiPart{{Text: info.ChannelSetting.SystemPrompt}}
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			merged := false
			for i := range request.SystemInstructions.Parts {
				if request.SystemInstructions.Parts[i].Text == "" {
					continue
				}
				request.SystemInstructions.Parts[i].Text = info.ChannelSetting.SystemPrompt + "\n" + request.SystemInstructions.Parts[i].Text
				merged = true
				break
			}
			if !merged {
				request.SystemInstructions.Parts = append([]dto.GeminiPart{{Text: info.ChannelSetting.SystemPrompt}}, request.SystemInstructions.Parts...)
			}
		}
	}

	// Clean up empty system instruction
	if request.SystemInstructions != nil {
		hasContent := false
		for _, part := range request.SystemInstructions.Parts {
			if part.Text != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			request.SystemInstructions = nil
		}
	}

	// 使用 ConvertGeminiRequest 转换请求格式（包含 ResponseSchema 清洗与历史对话滑动裁剪）
	convertedRequest, err := adaptor.ConvertGeminiRequest(c, info, request)
	if err != nil {
		return newConvertRequestFailedError(c, info, err)
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

	var requestBody io.Reader
	isPruned := common.GetContextKeyBool(c, "context_pruned") || (c != nil && c.GetBool("context_pruned"))
	if !isPruned && (model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled) {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		rawBytes, rerr := storage.Bytes()
		if rerr != nil {
			return types.NewErrorWithStatusCode(rerr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if cleaned, cerr := relaycommon.EnsureGeminiSchemaCleanliness(rawBytes); cerr == nil && len(cleaned) > 0 {
			rawBytes = cleaned
		}
		body, closer, berr := relaycommon.NewOutboundJSONBody(rawBytes)
		if berr != nil {
			return types.NewError(berr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		requestBody = body
	} else {
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
			if err != nil {
				return newAPIErrorFromParamOverride(err)
			}
		}

		// 确保清洗空 enum 和非标准 schema
		if cleaned, cerr := relaycommon.EnsureGeminiSchemaCleanliness(jsonData); cerr == nil && len(cleaned) > 0 {
			jsonData = cleaned
		}

		logger.LogDebug(c, "Gemini request body: %s", jsonData)

		body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		jsonData = nil
		requestBody = body
	}

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		logger.LogError(c, "Do gemini request failed: "+err.Error())
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)

			// 只要出现 Token 超限报错，则就进行智能裁剪重试请求（通用自愈）！
			if !c.GetBool("gemini_token_limit_retried") {
				limit, isExceeded := relaycommon.ParseUniversalContextLimit(newAPIError.Error())
				if isExceeded {
					c.Set("gemini_token_limit_retried", true)
					if limit > 0 {
						c.Set("forced_max_tokens", limit)
						common.SetContextKey(c, "forced_max_tokens", limit)
						relaycommon.RecordGlobalModelTokenLimit(info.GetOriginModelName(), limit)
						relaycommon.RecordGlobalModelTokenLimit(info.GetUpstreamModelName(), limit)
					}
					if relaycommon.AutoPruneGeminiChatRequest(c, info, request, limit) {
						logger.LogWarn(c, fmt.Sprintf("收到 Gemini 上游 Token 超限报错 (%s)，已自动通用智能滑动裁剪历史对话，正在立即重试请求...", newAPIError.Error()))
						newJsonData, mErr := common.Marshal(request)
						if mErr == nil {
							if len(info.ParamOverride) > 0 {
								newJsonData, _ = relaycommon.ApplyParamOverrideWithRelayInfo(newJsonData, info)
							}
							_ = relaycommon.UpdatePrunedRequestBody(c, info)
							newBody, closer, bErr := relaycommon.NewOutboundJSONBody(newJsonData)
							if bErr == nil {
								defer closer.Close()
								retryResp, rErr := adaptor.DoRequest(c, info, newBody)
								if rErr == nil && retryResp != nil {
									retryHttpResp := retryResp.(*http.Response)
									if retryHttpResp.StatusCode == http.StatusOK {
										logger.LogInfo(c, "Gemini Token 超限就地滑动裁剪重试成功，已拿到上游 200 OK 响应")
										usage, openaiErr := adaptor.DoResponse(c, retryHttpResp, info)
										if openaiErr != nil {
											service.ResetStatusCode(openaiErr, statusCodeMappingStr)
											service.SettleInterruptedRequestIfNeeded(c, info, usage, openaiErr)
											return openaiErr
										}
										service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
										return nil
									}
									newAPIError = service.RelayErrorHandler(c.Request.Context(), retryHttpResp, false)
									service.ResetStatusCode(newAPIError, statusCodeMappingStr)
								}
							}
						}
					}
				}
			}

			// 只要出现图片尺寸超限报错，则自动等比例缩放图片并立即重试！
			if !c.GetBool("gemini_image_dimension_retried") && relaycommon.IsImageDimensionExceededError(newAPIError.Error()) {
				c.Set("gemini_image_dimension_retried", true)
				safeDim := relaycommon.ParseMaxImageDimensionFromError(newAPIError.Error())
				if relaycommon.DownscaleOversizedImagesInGeminiRequest(c, request, safeDim) {
					logger.LogWarn(c, fmt.Sprintf("收到 Gemini 上游图片尺寸超限报错 (%s)，已自动等比例缩放图片并立即重试...", newAPIError.Error()))
					newJsonData, mErr := common.Marshal(request)
					if mErr == nil {
						if len(info.ParamOverride) > 0 {
							newJsonData, _ = relaycommon.ApplyParamOverrideWithRelayInfo(newJsonData, info)
						}
						_ = relaycommon.UpdatePrunedRequestBody(c, info)
						newBody, closer, bErr := relaycommon.NewOutboundJSONBody(newJsonData)
						if bErr == nil {
							defer closer.Close()
							retryResp, rErr := adaptor.DoRequest(c, info, newBody)
							if rErr == nil && retryResp != nil {
								retryHttpResp := retryResp.(*http.Response)
								if retryHttpResp.StatusCode == http.StatusOK {
									logger.LogInfo(c, "Gemini 图片尺寸超限就地缩放重试成功，已拿到上游 200 OK 响应")
									usage, openaiErr := adaptor.DoResponse(c, retryHttpResp, info)
									if openaiErr != nil {
										service.ResetStatusCode(openaiErr, statusCodeMappingStr)
										service.SettleInterruptedRequestIfNeeded(c, info, usage, openaiErr)
										return openaiErr
									}
									service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
									return nil
								}
								newAPIError = service.RelayErrorHandler(c.Request.Context(), retryHttpResp, false)
								service.ResetStatusCode(newAPIError, statusCodeMappingStr)
							}
						}
					}
				}
			}

			// 只要出现缺少 tool_use.id / functionCall.id 报错，则深度自愈工具调用 ID 并立即重试！
			if !c.GetBool("gemini_tool_id_retried") && relaycommon.IsToolUseIDError(newAPIError.Error()) {
				c.Set("gemini_tool_id_retried", true)
				rawBytes, _ := relaycommon.GetPrunedRequestBody(c, info)
				if len(rawBytes) == 0 {
					rawBytes, _ = common.Marshal(request)
				}
				if repaired, repOk := relaycommon.RepairToolIDsInRawJSON(rawBytes); repOk {
					logger.LogWarn(c, fmt.Sprintf("收到 Gemini 上游缺少工具ID报错 (%s)，已自动深度自愈补齐 tool ID 并立即重试...", newAPIError.Error()))
					_ = relaycommon.UpdatePrunedRequestBody(c, info)
					newBody, closer, bErr := relaycommon.NewOutboundJSONBody(repaired)
					if bErr == nil {
						defer closer.Close()
						retryResp, rErr := adaptor.DoRequest(c, info, newBody)
						if rErr == nil && retryResp != nil {
							retryHttpResp := retryResp.(*http.Response)
							if retryHttpResp.StatusCode == http.StatusOK {
								logger.LogInfo(c, "Gemini 缺少工具ID就地自愈重试成功，已拿到上游 200 OK 响应")
								usage, openaiErr := adaptor.DoResponse(c, retryHttpResp, info)
								if openaiErr != nil {
									service.ResetStatusCode(openaiErr, statusCodeMappingStr)
									service.SettleInterruptedRequestIfNeeded(c, info, usage, openaiErr)
									return openaiErr
								}
								service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
								return nil
							}
							newAPIError = service.RelayErrorHandler(c.Request.Context(), retryHttpResp, false)
							service.ResetStatusCode(newAPIError, statusCodeMappingStr)
						}
					}
				}
			}

			// 只要出现 Gemini Part 数据未初始化或非标准 Part 顶层字段报错，则深度自愈并立即重试！
			if !c.GetBool("gemini_part_data_retried") && relaycommon.IsGeminiPartDataError(newAPIError.Error()) {
				c.Set("gemini_part_data_retried", true)
				rawBytes, _ := relaycommon.GetPrunedRequestBody(c, info)
				if len(rawBytes) == 0 {
					rawBytes, _ = common.Marshal(request)
				}
				cleanedBytes, cErr := relaycommon.EnsureGeminiSchemaCleanliness(rawBytes)
				if cErr == nil && len(cleanedBytes) > 0 {
					logger.LogWarn(c, fmt.Sprintf("收到 Gemini 上游 Part 数据格式报错 (%s)，已自动深度自愈并填充有效 data 字段，正在立即重试请求...", newAPIError.Error()))
					if len(info.ParamOverride) > 0 {
						cleanedBytes, _ = relaycommon.ApplyParamOverrideWithRelayInfo(cleanedBytes, info)
					}
					_ = relaycommon.UpdatePrunedRequestBody(c, info)
					newBody, closer, bErr := relaycommon.NewOutboundJSONBody(cleanedBytes)
					if bErr == nil {
						defer closer.Close()
						retryResp, rErr := adaptor.DoRequest(c, info, newBody)
						if rErr == nil && retryResp != nil {
							retryHttpResp := retryResp.(*http.Response)
							if retryHttpResp.StatusCode == http.StatusOK {
								logger.LogInfo(c, "Gemini Part 数据深度自愈重试成功，已拿到上游 200 OK 响应")
								usage, openaiErr := adaptor.DoResponse(c, retryHttpResp, info)
								if openaiErr != nil {
									service.ResetStatusCode(openaiErr, statusCodeMappingStr)
									service.SettleInterruptedRequestIfNeeded(c, info, usage, openaiErr)
									return openaiErr
								}
								service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
								return nil
							}
							newAPIError = service.RelayErrorHandler(c.Request.Context(), retryHttpResp, false)
							service.ResetStatusCode(newAPIError, statusCodeMappingStr)
						}
					}
				}
			}

			return newAPIError
		}
	}

	usage, openaiErr := adaptor.DoResponse(c, resp.(*http.Response), info)
	if openaiErr != nil {
		service.ResetStatusCode(openaiErr, statusCodeMappingStr)
		service.SettleInterruptedRequestIfNeeded(c, info, usage, openaiErr)
		return openaiErr
	}

	service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
	return nil
}

func GeminiEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	isBatch := strings.HasSuffix(c.Request.URL.Path, "batchEmbedContents")
	info.IsGeminiBatchEmbedding = isBatch

	var req dto.Request
	var err error
	var inputTexts []string

	if isBatch {
		batchRequest := &dto.GeminiBatchEmbeddingRequest{}
		err = common.UnmarshalBodyReusable(c, batchRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		req = batchRequest
		for _, r := range batchRequest.Requests {
			for _, part := range r.Content.Parts {
				if part.Text != "" {
					inputTexts = append(inputTexts, part.Text)
				}
			}
		}
	} else {
		singleRequest := &dto.GeminiEmbeddingRequest{}
		err = common.UnmarshalBodyReusable(c, singleRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		req = singleRequest
		for _, part := range singleRequest.Content.Parts {
			if part.Text != "" {
				inputTexts = append(inputTexts, part.Text)
			}
		}
	}

	err = helper.ModelMappedHelper(c, info, req)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, req); err != nil {
		return newConvertRequestFailedError(c, info, err)
	}

	req.SetModelName("models/" + info.UpstreamModelName)

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	var requestBody io.Reader
	jsonData, err := common.Marshal(req)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	// apply param override
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return newAPIErrorFromParamOverride(err)
		}
	}
	logger.LogDebug(c, "Gemini embedding request body: %s", jsonData)
	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()
	jsonData = nil
	requestBody = body

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		logger.LogError(c, "Do gemini request failed: "+err.Error())
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	usage, openaiErr := adaptor.DoResponse(c, resp.(*http.Response), info)
	if openaiErr != nil {
		service.ResetStatusCode(openaiErr, statusCodeMappingStr)
		service.SettleInterruptedRequestIfNeeded(c, info, usage, openaiErr)
		return openaiErr
	}

	service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
	return nil
}

