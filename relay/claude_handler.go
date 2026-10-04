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

func ClaudeHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {

	info.InitChannelMeta(c)

	claudeReq, ok := info.Request.(*dto.ClaudeRequest)

	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.ClaudeRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(claudeReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ClaudeRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
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
			if cleaned, modified := relaycommon.CleanUnavailablePromptFromText(request.Messages[i].GetStringContent()); modified {
				request.Messages[i].SetStringContent(cleaned)
			}
		}
	}

	clientIsStream := claudeReq.Stream != nil && *claudeReq.Stream
	if info.ShouldConvertNonStreamToStream(clientIsStream) {
		trueVal := true
		info.ConvertNonStreamToStream = true
		info.IsStream = true
		request.Stream = &trueVal
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	if info.ChannelSetting.SystemPrompt != "" {
		if request.System == nil {
			request.SetStringSystem(info.ChannelSetting.SystemPrompt)
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			if request.IsStringSystem() {
				existing := strings.TrimSpace(request.GetStringSystem())
				if existing == "" {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt)
				} else {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt + "\n" + existing)
				}
			} else {
				systemContents := request.ParseSystem()
				newSystem := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
				newSystem.SetText(info.ChannelSetting.SystemPrompt)
				if len(systemContents) == 0 {
					request.System = []dto.ClaudeMediaMessage{newSystem}
				} else {
					request.System = append([]dto.ClaudeMediaMessage{newSystem}, systemContents...)
				}
			}
		}
	}

	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		usage, newApiErr := textRequestViaResponses(c, info, adaptor, request)
		if newApiErr != nil {
			service.SettleInterruptedRequestIfNeeded(c, info, usage, newApiErr)
			return newApiErr
		}

		service.PostTextConsumeQuota(c, info, usage, nil)
		return nil
	}

	var requestBody io.Reader
	if !info.ConvertNonStreamToStream && (model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled) {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		rawBytes, rerr := storage.Bytes()
		if rerr != nil {
			return types.NewErrorWithStatusCode(rerr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if len(info.ParamOverride) > 0 {
			if overridden, oerr := relaycommon.ApplyParamOverrideWithRelayInfo(rawBytes, info); oerr == nil && len(overridden) > 0 {
				rawBytes = overridden
			}
		}
		if repaired, repErr := relaycommon.EnsureClaudeToolIDs(rawBytes); repErr == nil && len(repaired) > 0 {
			rawBytes = repaired
		}
		if cleaned, cerr := relaycommon.EnsureClaudeRequestCleanliness(rawBytes); cerr == nil && len(cleaned) > 0 {
			rawBytes = cleaned
		}
		_ = relaycommon.UpdatePrunedRequestBody(c, info)
		body, closer, berr := relaycommon.NewOutboundJSONBody(rawBytes)
		if berr != nil {
			return types.NewError(berr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		requestBody = body
	} else {
		convertedRequest, err := adaptor.ConvertClaudeRequest(c, info, request)
		if err != nil {
			return newConvertRequestFailedError(c, info, err)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for Claude API
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

		logger.LogDebug(c, "requestBody: %s", jsonData)
		body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		jsonData = nil
		requestBody = body
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)

			// 只要出现 Token 超限报错，则就进行智能裁剪重试请求！
			if !c.GetBool("claude_token_limit_retried") {
				limit, isExceeded := relaycommon.ParseUniversalContextLimit(newAPIError.Error())
				if isExceeded {
					c.Set("claude_token_limit_retried", true)
					if limit > 0 {
						c.Set("forced_max_tokens", limit)
						common.SetContextKey(c, "forced_max_tokens", limit)
						relaycommon.RecordGlobalModelTokenLimit(info.GetOriginModelName(), limit)
						relaycommon.RecordGlobalModelTokenLimit(info.GetUpstreamModelName(), limit)
					}
					if relaycommon.AutoPruneClaudeRequest(c, info, request, limit) {
						logger.LogWarn(c, fmt.Sprintf("收到 Claude 上游 Token 超限报错 (%s)，已自动滑动裁剪历史对话并立即重试...", newAPIError.Error()))
						newJsonData, mErr := common.Marshal(request)
						if mErr == nil {
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
										logger.LogInfo(c, "Claude Token 超限就地滑动裁剪重试成功，已拿到上游 200 OK 响应")
										usage, resErr := adaptor.DoResponse(c, retryHttpResp, info)
										if resErr != nil {
											service.ResetStatusCode(resErr, statusCodeMappingStr)
											service.SettleInterruptedRequestIfNeeded(c, info, usage, resErr)
											return resErr
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
			if !c.GetBool("claude_image_dimension_retried") && relaycommon.IsImageDimensionExceededError(newAPIError.Error()) {
				c.Set("claude_image_dimension_retried", true)
				safeDim := relaycommon.ParseMaxImageDimensionFromError(newAPIError.Error())
				if relaycommon.DownscaleOversizedImagesInClaudeRequest(c, request, safeDim) {
					logger.LogWarn(c, fmt.Sprintf("收到 Claude 上游图片尺寸超限报错 (%s)，已自动等比例缩放图片并立即重试...", newAPIError.Error()))
					newJsonData, mErr := common.Marshal(request)
					if mErr == nil {
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
									logger.LogInfo(c, "Claude 图片尺寸超限就地缩放重试成功，已拿到上游 200 OK 响应")
									usage, resErr := adaptor.DoResponse(c, retryHttpResp, info)
									if resErr != nil {
										service.ResetStatusCode(resErr, statusCodeMappingStr)
										service.SettleInterruptedRequestIfNeeded(c, info, usage, resErr)
										return resErr
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

			// 只要出现缺少 tool_use.id / tool_result.tool_use_id 或空白 text block 报错，则深度自愈并立即重试！
			if !c.GetBool("claude_tool_id_retried") && (relaycommon.IsToolUseIDError(newAPIError.Error()) || relaycommon.IsClaudeWhitespaceTextError(newAPIError.Error())) {
				c.Set("claude_tool_id_retried", true)
				rawBytes, _ := relaycommon.GetPrunedRequestBody(c, info)
				if len(rawBytes) == 0 {
					if storage, sErr := common.GetBodyStorage(c); sErr == nil {
						rawBytes, _ = storage.Bytes()
					}
				}
				if len(rawBytes) == 0 && request != nil {
					rawBytes, _ = common.Marshal(request)
				}
				repaired, repOk := relaycommon.RepairToolIDsInRawJSON(rawBytes)
				if !repOk {
					repaired = rawBytes
				}
				if cleaned, cerr := relaycommon.EnsureClaudeRequestCleanliness(repaired); cerr == nil && len(cleaned) > 0 {
					repaired = cleaned
				}
				if len(info.ParamOverride) > 0 {
					if overridden, oerr := relaycommon.ApplyParamOverrideWithRelayInfo(repaired, info); oerr == nil && len(overridden) > 0 {
						repaired = overridden
					}
				}
				logger.LogWarn(c, fmt.Sprintf("收到 Claude 上游消息格式报错 (%s)，已自动深度自愈补齐 tool ID 与清洗空白 text block，正在立即重试...", newAPIError.Error()))
				_ = relaycommon.UpdatePrunedRequestBody(c, info)
				newBody, closer, bErr := relaycommon.NewOutboundJSONBody(repaired)
				if bErr == nil {
					defer closer.Close()
					retryResp, rErr := adaptor.DoRequest(c, info, newBody)
					if rErr == nil && retryResp != nil {
						retryHttpResp := retryResp.(*http.Response)
						if retryHttpResp.StatusCode == http.StatusOK {
							logger.LogInfo(c, "Claude 消息格式就地自愈重试成功，已拿到上游 200 OK 响应")
							usage, resErr := adaptor.DoResponse(c, retryHttpResp, info)
							if resErr != nil {
								service.ResetStatusCode(resErr, statusCodeMappingStr)
								service.SettleInterruptedRequestIfNeeded(c, info, usage, resErr)
								return resErr
							}
							service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
							return nil
						}
						newAPIError = service.RelayErrorHandler(c.Request.Context(), retryHttpResp, false)
						service.ResetStatusCode(newAPIError, statusCodeMappingStr)
					}
				}
			}

			return newAPIError
		}
	}

	if unavailErr := relaycommon.CheckAndInterceptUnavailableResponse(httpResp); unavailErr != nil {
		logger.LogWarn(c, "上游响应检测到模型不可用提示 (Claude Opus 4.6 is no longer available)，拦截并触发渠道重试...")
		return unavailErr
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		service.SettleInterruptedRequestIfNeeded(c, info, usage, newAPIError)
		return newAPIError
	}

	service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
	return nil
}
