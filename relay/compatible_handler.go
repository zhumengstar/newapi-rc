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
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

func TextHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	textReq, ok := info.Request.(*dto.GeneralOpenAIRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.GeneralOpenAIRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(textReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	if request.WebSearchOptions != nil {
		c.Set("chat_completion_web_search_context_size", request.WebSearchOptions.SearchContextSize)
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return newConvertRequestFailedError(c, info, err)
	}

	// 清洗输入消息中的模型不可用提示废话，避免历史对话诱发模型复读
	for i := range request.Messages {
		if request.Messages[i].IsStringContent() {
			if cleaned, modified := relaycommon.CleanUnavailablePromptFromText(request.Messages[i].StringContent()); modified {
				request.Messages[i].SetStringContent(cleaned)
			}
		}
	}

	clientIsStream := lo.FromPtrOr(request.Stream, false)
	if info.ShouldConvertNonStreamToStream(clientIsStream) {
		info.ConvertNonStreamToStream = true
		info.IsStream = true
		request.Stream = lo.ToPtr(true)
	}

	includeUsage := true
	// 判断用户是否需要返回使用情况
	if request.StreamOptions != nil {
		includeUsage = request.StreamOptions.IncludeUsage
	}

	// 如果不支持StreamOptions，将StreamOptions设置为nil
	if !info.SupportStreamOptions || !lo.FromPtrOr(request.Stream, false) {
		request.StreamOptions = nil
	} else {
		// 如果支持StreamOptions，且请求中没有设置StreamOptions，根据配置文件设置StreamOptions
		if constant.ForceStreamOption {
			request.StreamOptions = &dto.StreamOptions{
				IncludeUsage: true,
			}
		}
	}

	info.ShouldIncludeUsage = includeUsage

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	passThroughGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	if info.RelayMode == relayconstant.RelayModeChatCompletions &&
		!passThroughGlobal &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		applySystemPromptIfNeeded(c, info, request)
		usage, newApiErr := textRequestViaResponses(c, info, adaptor, request)
		if newApiErr != nil {
			service.SettleInterruptedRequestIfNeeded(c, info, usage, newApiErr)
			return newApiErr
		}

		var containAudioTokens = usage.CompletionTokenDetails.AudioTokens > 0 || usage.PromptTokensDetails.AudioTokens > 0
		var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

		if containAudioTokens && containsAudioRatios {
			service.PostAudioConsumeQuota(c, info, usage, "")
		} else {
			service.PostTextConsumeQuota(c, info, usage, nil)
		}
		return nil
	}

	var requestBody io.Reader
	var convertedRequest any

	if !info.ConvertNonStreamToStream && (passThroughGlobal || info.ChannelSetting.PassThroughBodyEnabled) {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if common.DebugEnabled {
			if debugBytes, bErr := storage.Bytes(); bErr == nil {
				logger.LogDebug(c, "requestBody: %s", debugBytes)
			}
		}
		requestBody = common.NewReplayableBodyReader(storage)
	} else {
		var cErr error
		convertedRequest, cErr = adaptor.ConvertOpenAIRequest(c, info, request)
		if cErr != nil {
			return newConvertRequestFailedError(c, info, cErr)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

		if req, ok := convertedRequest.(*dto.GeneralOpenAIRequest); ok {
			applySystemPromptIfNeeded(c, info, req)
		}

		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for OpenAI API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
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

		// 安全防护：确保发送给 Claude/反重力渠道的所有 tool_use 块均携带合法非空 id，避免上游 400 Field required
		if repaired, repErr := relaycommon.EnsureClaudeToolIDs(jsonData); repErr == nil && len(repaired) > 0 {
			jsonData = repaired
		}

		logger.LogDebug(c, "text request body: %s", jsonData)

		body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		jsonData = nil
		requestBody = body
	}

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)

			// 只要出现 Token 超限报错，则就进行智能裁剪重试请求（通用自愈，覆盖 OpenAI、Gemini、Claude、DeepSeek 等所有模型）！
			if !c.GetBool("universal_token_limit_retried") {
				limit, isExceeded := relaycommon.ParseUniversalContextLimit(newApiErr.Error())
				if isExceeded {
					c.Set("universal_token_limit_retried", true)
					if limit > 0 {
						c.Set("forced_max_tokens", limit)
						common.SetContextKey(c, "forced_max_tokens", limit)
						relaycommon.RecordGlobalModelTokenLimit(info.GetOriginModelName(), limit)
						relaycommon.RecordGlobalModelTokenLimit(info.GetUpstreamModelName(), limit)
					}
					pruned := false
					if convertedRequest != nil {
						pruned = relaycommon.UniversalPruneAnyRequest(c, info, convertedRequest, limit)
					}
					if request != nil && any(request) != convertedRequest {
						if relaycommon.AutoPruneOpenAIRequest(c, info, request, limit) {
							pruned = true
						}
					}
					if pruned {
						logger.LogWarn(c, fmt.Sprintf("收到上游 Token 超限报错 (%s)，已自动通用智能滑动裁剪历史对话，正在立即重试请求...", newApiErr.Error()))
						targetReq := convertedRequest
						if targetReq == nil {
							targetReq = request
						}
						newJsonData, mErr := common.Marshal(targetReq)
						if mErr == nil {
							newJsonData, _ = relaycommon.RemoveDisabledFields(newJsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
							if len(info.ParamOverride) > 0 {
								newJsonData, _ = relaycommon.ApplyParamOverrideWithRelayInfo(newJsonData, info)
							}
							if repaired, repErr := relaycommon.EnsureClaudeToolIDs(newJsonData); repErr == nil && len(repaired) > 0 {
								newJsonData = repaired
							}
							_ = relaycommon.UpdatePrunedRequestBody(c, info)
							newBody, closer, bErr := relaycommon.NewOutboundJSONBody(newJsonData)
							if bErr == nil {
								defer closer.Close()
								retryResp, rErr := adaptor.DoRequest(c, info, newBody)
								if rErr == nil && retryResp != nil {
									retryHttpResp := retryResp.(*http.Response)
									if retryHttpResp.StatusCode == http.StatusOK {
										logger.LogInfo(c, "Token 超限就地通用滑动裁剪重试成功，已拿到上游 200 OK 响应")
										usage, resErr := adaptor.DoResponse(c, retryHttpResp, info)
										if resErr != nil {
											service.ResetStatusCode(resErr, statusCodeMappingStr)
											service.SettleInterruptedRequestIfNeeded(c, info, usage, resErr)
											return resErr
										}
										var containAudioTokens = usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0
										var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)
										if containAudioTokens && containsAudioRatios {
											service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "")
										} else {
											service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
										}
										return nil
									}
									newApiErr = service.RelayErrorHandler(c.Request.Context(), retryHttpResp, false)
									service.ResetStatusCode(newApiErr, statusCodeMappingStr)
								}
							}
						}
					}
				}
			}

			// 只要出现图片尺寸超限报错，则自动等比例缩放图片并立即重试！
			if !c.GetBool("image_dimension_retried") && relaycommon.IsImageDimensionExceededError(newApiErr.Error()) {
				c.Set("image_dimension_retried", true)
				safeDim := relaycommon.ParseMaxImageDimensionFromError(newApiErr.Error())
				pruned := false
				if req, ok := convertedRequest.(*dto.ClaudeRequest); ok && req != nil {
					pruned = relaycommon.DownscaleOversizedImagesInClaudeRequest(c, req, safeDim)
				} else if req, ok := convertedRequest.(*dto.GeneralOpenAIRequest); ok && req != nil {
					pruned = relaycommon.DownscaleOversizedImagesInOpenAIRequest(c, req, safeDim)
				}
				if request != nil && any(request) != convertedRequest {
					if relaycommon.DownscaleOversizedImagesInOpenAIRequest(c, request, safeDim) {
						pruned = true
					}
				}
				if pruned {
					logger.LogWarn(c, fmt.Sprintf("收到上游图片尺寸超限报错 (%s)，已自动等比例缩放图片并立即重试...", newApiErr.Error()))
					targetReq := convertedRequest
					if targetReq == nil {
						targetReq = request
					}
					newJsonData, mErr := common.Marshal(targetReq)
					if mErr == nil {
						newJsonData, _ = relaycommon.RemoveDisabledFields(newJsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
						if len(info.ParamOverride) > 0 {
							newJsonData, _ = relaycommon.ApplyParamOverrideWithRelayInfo(newJsonData, info)
						}
						if repaired, repErr := relaycommon.EnsureClaudeToolIDs(newJsonData); repErr == nil && len(repaired) > 0 {
							newJsonData = repaired
						}
						_ = relaycommon.UpdatePrunedRequestBody(c, info)
						newBody, closer, bErr := relaycommon.NewOutboundJSONBody(newJsonData)
						if bErr == nil {
							defer closer.Close()
							retryResp, rErr := adaptor.DoRequest(c, info, newBody)
							if rErr == nil && retryResp != nil {
								retryHttpResp := retryResp.(*http.Response)
								if retryHttpResp.StatusCode == http.StatusOK {
									logger.LogInfo(c, "图片尺寸超限就地缩放重试成功，已拿到上游 200 OK 响应")
									usage, resErr := adaptor.DoResponse(c, retryHttpResp, info)
									if resErr != nil {
										service.ResetStatusCode(resErr, statusCodeMappingStr)
										service.SettleInterruptedRequestIfNeeded(c, info, usage, resErr)
										return resErr
									}
									var containAudioTokens = usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0
									var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)
									if containAudioTokens && containsAudioRatios {
										service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "")
									} else {
										service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
									}
									return nil
								}
								newApiErr = service.RelayErrorHandler(c.Request.Context(), retryHttpResp, false)
								service.ResetStatusCode(newApiErr, statusCodeMappingStr)
							}
						}
					}
				}
			}

			// 只要出现缺少 tool_use.id / tool_result.tool_use_id 报错，则深度自愈工具调用 ID 并立即重试！
			if !c.GetBool("tool_id_retried") && relaycommon.IsToolUseIDError(newApiErr.Error()) {
				c.Set("tool_id_retried", true)
				rawBytes, _ := relaycommon.GetPrunedRequestBody(c, info)
				if len(rawBytes) == 0 {
					targetReq := convertedRequest
					if targetReq == nil {
						targetReq = request
					}
					rawBytes, _ = common.Marshal(targetReq)
				}
				if repaired, repOk := relaycommon.RepairToolIDsInRawJSON(rawBytes); repOk {
					logger.LogWarn(c, fmt.Sprintf("收到上游缺少工具ID报错 (%s)，已自动深度自愈补齐 tool_use.id 并立即重试...", newApiErr.Error()))
					_ = relaycommon.UpdatePrunedRequestBody(c, info)
					newBody, closer, bErr := relaycommon.NewOutboundJSONBody(repaired)
					if bErr == nil {
						defer closer.Close()
						retryResp, rErr := adaptor.DoRequest(c, info, newBody)
						if rErr == nil && retryResp != nil {
							retryHttpResp := retryResp.(*http.Response)
							if retryHttpResp.StatusCode == http.StatusOK {
								logger.LogInfo(c, "缺少工具ID就地自愈重试成功，已拿到上游 200 OK 响应")
								usage, resErr := adaptor.DoResponse(c, retryHttpResp, info)
								if resErr != nil {
									service.ResetStatusCode(resErr, statusCodeMappingStr)
									service.SettleInterruptedRequestIfNeeded(c, info, usage, resErr)
									return resErr
								}
								var containAudioTokens = usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0
								var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)
								if containAudioTokens && containsAudioRatios {
									service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "")
								} else {
									service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
								}
								return nil
							}
							newApiErr = service.RelayErrorHandler(c.Request.Context(), retryHttpResp, false)
							service.ResetStatusCode(newApiErr, statusCodeMappingStr)
						}
					}
				}
			}

			return newApiErr
		}
	}

	if unavailErr := relaycommon.CheckAndInterceptUnavailableResponse(httpResp); unavailErr != nil {
		logger.LogWarn(c, "上游响应检测到模型不可用提示 (Claude Opus 4.6 is no longer available)，拦截并触发渠道重试...")
		return unavailErr
	}

	usage, newApiErr := adaptor.DoResponse(c, httpResp, info)
	if newApiErr != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		service.SettleInterruptedRequestIfNeeded(c, info, usage, newApiErr)
		return newApiErr
	}

	var containAudioTokens = usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0
	var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

	if containAudioTokens && containsAudioRatios {
		service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "")
	} else {
		service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
	}
	return nil
}
