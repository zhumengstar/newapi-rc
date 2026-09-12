package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func abortWithOpenAiMessage(c *gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	codeStr := ""
	if len(code) > 0 {
		codeStr = string(code[0])
	}
	userId := c.GetInt("id")
	_, preparedPluginRoute := c.Get(pluginruntime.ContextKeyRouteRequest)
	if !preparedPluginRoute || !RespondTaskPluginError(c, &dto.TaskError{
		Code:       codeStr,
		Message:    message,
		StatusCode: statusCode,
	}) {
		c.JSON(statusCode, gin.H{
			"error": gin.H{
				"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
				"type":    "new_api_error",
				"code":    codeStr,
			},
		})
	}
	c.Abort()
	logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", userId, message))
	recordMiddlewareAbortErrorLog(c, statusCode, message, codeStr)
}

func abortWithMidjourneyMessage(c *gin.Context, statusCode int, code int, description string) {
	c.JSON(statusCode, gin.H{
		"description": description,
		"type":        "new_api_error",
		"code":        code,
	})
	c.Abort()
	logger.LogError(c.Request.Context(), description)
	recordMiddlewareAbortErrorLog(c, statusCode, description, fmt.Sprintf("%d", code))
}

func recordMiddlewareAbortErrorLog(c *gin.Context, statusCode int, message string, codeStr string) {
	if !constant.ErrorLogEnabled {
		return
	}
	if common.GetContextKeyBool(c, constant.ContextKeyErrorLogRecorded) {
		return
	}

	path := ""
	if c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}

	tag := c.GetString(RouteTagKey)
	isRelayTag := tag == "relay"
	isModelPath := strings.HasPrefix(path, "/v1/") ||
		strings.HasPrefix(path, "/v1beta/") ||
		strings.HasPrefix(path, "/canvas/v1/") ||
		strings.HasPrefix(path, "/mj/") ||
		strings.HasPrefix(path, "/suno/")

	if !isRelayTag && !isModelPath {
		return
	}

	// 排除非模型调用的 401 扫描（如 GET /v1/models、GET /v1beta/models 纯列出模型）
	if statusCode == http.StatusUnauthorized && c.Request != nil {
		if c.Request.Method != http.MethodPost && !strings.Contains(path, "models/") {
			return
		}
	}

	userId := c.GetInt("id")
	tokenName := c.GetString("token_name")
	if tokenName == "" && statusCode == http.StatusUnauthorized {
		tokenName = "Invalid Token"
	}
	tokenId := c.GetInt("token_id")
	userGroup := c.GetString("group")
	channelId := c.GetInt("channel_id")

	modelName := c.GetString("original_model")
	if modelName == "" {
		modelName = c.GetString("model")
	}
	if modelName == "" && strings.HasPrefix(path, "/v1beta/models/") {
		modelName = extractModelNameFromGeminiPath(path)
	}
	if modelName == "" && c.Request != nil && c.Request.Method == http.MethodPost {
		var req struct {
			Model string `json:"model"`
		}
		if err := common.UnmarshalBodyReusable(c, &req); err == nil && req.Model != "" {
			modelName = req.Model
		}
	}

	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	useTimeSeconds := 0
	if !startTime.IsZero() {
		useTimeSeconds = int(time.Since(startTime).Seconds())
	}

	other := model.NewLogOther()
	if path != "" {
		other.SetPublic("request_path", path)
	}
	other.SetPublic("error_type", "middleware_abort")
	if codeStr != "" {
		other.SetPublic("error_code", codeStr)
	}
	other.SetPublic("status_code", statusCode)

	content := fmt.Sprintf("status_code=%d, %s", statusCode, message)
	model.RecordErrorLog(c, userId, channelId, modelName, tokenName, content, tokenId, useTimeSeconds, false, userGroup, other)
	common.SetContextKey(c, constant.ContextKeyErrorLogRecorded, true)
}
