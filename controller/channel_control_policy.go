package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type channelGroupAdaptiveRequest struct {
	Group           string `json:"group"`
	AdaptiveEnabled *bool  `json:"adaptive_enabled"`
}

type channelAdaptiveRequest struct {
	AdaptiveEnabled *bool `json:"adaptive_enabled"`
}

// UpdateChannelAdaptiveEnabled updates a single channel without invoking the
// general channel-edit validation path.
func UpdateChannelAdaptiveEnabled(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	var request channelAdaptiveRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.AdaptiveEnabled == nil {
		common.ApiError(c, fmt.Errorf("adaptive_enabled is required"))
		return
	}
	if err := model.UpdateChannelAdaptiveEnabled(id, *request.AdaptiveEnabled); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	recordManageAudit(c, "channel.adaptive.update", map[string]interface{}{
		"id": id, "adaptive_enabled": *request.AdaptiveEnabled,
	})
	common.ApiSuccess(c, true)
}

type channelGroupPriorityOrderRequest struct {
	Groups []string `json:"groups"`
}

// UpdateChannelGroupAdaptiveEnabled toggles adaptive routing for every
// channel belonging to one exact group.
func UpdateChannelGroupAdaptiveEnabled(c *gin.Context) {
	var request channelGroupAdaptiveRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	request.Group = strings.TrimSpace(request.Group)
	if request.Group == "" || request.AdaptiveEnabled == nil {
		common.ApiError(c, fmt.Errorf("group and adaptive_enabled are required"))
		return
	}
	updated, err := model.UpdateChannelGroupAdaptiveEnabled(request.Group, *request.AdaptiveEnabled)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	recordManageAudit(c, "channel.group_adaptive.update", map[string]interface{}{
		"group": request.Group, "adaptive_enabled": *request.AdaptiveEnabled, "updated": updated,
	})
	common.ApiSuccess(c, updated)
}

// ApplyChannelGroupPriorityOrder persists the visible group order as routing
// priorities in increments of ten, starting at 9 (9, 19, 29...).
func ApplyChannelGroupPriorityOrder(c *gin.Context) {
	var request channelGroupPriorityOrderRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(request.Groups) == 0 {
		common.ApiError(c, fmt.Errorf("groups are required"))
		return
	}
	updated, err := model.ApplyChannelGroupPriorityOrder(request.Groups)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	recordManageAudit(c, "channel.group_priority_order.update", map[string]interface{}{
		"groups": request.Groups, "updated": updated,
	})
	common.ApiSuccess(c, updated)
}

// GetChannelControlPolicies exposes the persisted group policies used by
// adaptive routing, scheduled probes, and automatic recovery.
func GetChannelControlPolicies(c *gin.Context) {
	policies, err := model.GetChannelControlPolicies(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, policies)
}

func GetChannelControlMetrics(c *gin.Context) {
	window := 24 * 3600
	if raw := strings.TrimSpace(c.Query("window_seconds")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			window = parsed
		}
	}
	metrics, err := service.GetChannelControlMetrics(c.Request.Context(), window)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, metrics)
}

func UpsertChannelControlPolicy(c *gin.Context) {
	policy := model.ChannelControlPolicy{}
	if err := common.DecodeJson(c.Request.Body, &policy); err != nil {
		common.ApiError(c, err)
		return
	}
	policy.Group = strings.TrimSpace(policy.Group)
	stored, err := model.UpsertChannelControlPolicy(c.Request.Context(), policy)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.control_policy.upsert", map[string]interface{}{
		"group": stored.Group,
	})
	common.ApiSuccess(c, stored)
}

func DeleteChannelControlPolicy(c *gin.Context) {
	group := strings.TrimSpace(c.Param("group"))
	deleted, err := model.DeleteChannelControlPolicy(c.Request.Context(), group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "channel control policy not found",
		})
		return
	}
	recordManageAudit(c, "channel.control_policy.delete", map[string]interface{}{
		"group": group,
	})
	common.ApiSuccess(c, true)
}
