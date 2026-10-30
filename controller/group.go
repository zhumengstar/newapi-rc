package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetChannelGroups(c *gin.Context) {
	groups, err := model.GetChannelGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, groups)
}

func getAllConfiguredAndRankedGroups() []string {
	groupNames := model.GetRankedGroups()
	groupSet := make(map[string]struct{}, len(groupNames))
	for _, name := range groupNames {
		if name != "" {
			groupSet[name] = struct{}{}
		}
	}
	// 实时合并分组定价与可用分组中的所有分组，确保管理员在分组定价中配置后，渠道编辑等处立即立即可选
	for group := range ratio_setting.GetGroupRatioCopy() {
		if group != "" {
			if _, exists := groupSet[group]; !exists {
				groupNames = append(groupNames, group)
				groupSet[group] = struct{}{}
			}
		}
	}
	for group := range setting.GetUserUsableGroupsCopy() {
		if group != "" {
			if _, exists := groupSet[group]; !exists {
				groupNames = append(groupNames, group)
				groupSet[group] = struct{}{}
			}
		}
	}
	return groupNames
}

func GetGroups(c *gin.Context) {
	groupNames := getAllConfiguredAndRankedGroups()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]any)
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userSetting, _ := model.GetUserSetting(userId, false)
	userUsableGroups := service.GetUserUsableGroupsWithSetting(userSetting, userGroup)
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			usableGroups[groupName] = map[string]any{
				"ratio": service.GetUserGroupRatioWithSetting(userSetting, userGroup, groupName),
				"desc":  desc,
			}
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]any{
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}

func GetGroupDetails(c *gin.Context) {
	groupNames := getAllConfiguredAndRankedGroups()
	groupRatios := ratio_setting.GetGroupRatioCopy()
	userUsableGroups := setting.GetUserUsableGroupsCopy()
	userId := c.GetInt("id")
	userGroup, _ := model.GetUserGroup(userId, false)
	groupModelsMap, _ := model.GetGroupModelsMap()
	if groupModelsMap == nil {
		groupModelsMap = make(map[string][]string)
	}
	groupChannelsMap, _ := model.GetGroupChannelsMap()
	if groupChannelsMap == nil {
		groupChannelsMap = make(map[string][]model.ChannelGroupItem)
	}
	groupMeta := make(map[string]map[string]any, len(groupNames))
	for _, groupName := range groupNames {
		ratio := groupRatios[groupName]
		_, isPublic := userUsableGroups[groupName]
		models := groupModelsMap[groupName]
		if models == nil {
			models = []string{}
		}
		channels := groupChannelsMap[groupName]
		if channels == nil {
			channels = []model.ChannelGroupItem{}
		}
		groupMeta[groupName] = map[string]any{
			"ratio":       ratio,
			"admin_ratio": service.GetUserGroupRatioForUser(userId, userGroup, groupName),
			"is_public":   isPublic,
			"desc":        setting.GetUsableGroupDescription(groupName),
			"models":      models,
			"channels":    channels,
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"groups": groupNames,
			"meta":   groupMeta,
		},
	})
}

type UpdateGroupRequest struct {
	OldGroup string   `json:"old_group" binding:"required"`
	NewGroup string   `json:"new_group" binding:"required"`
	Ratio    *float64 `json:"ratio"`
	Desc     *string  `json:"desc"`
}

func UpdateGroup(c *gin.Context) {
	var req UpdateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数无效: " + err.Error(),
		})
		return
	}

	ratio := 1.0
	if req.Ratio != nil {
		ratio = *req.Ratio
	} else {
		ratio = ratio_setting.GetGroupRatio(req.OldGroup)
	}

	desc := ""
	if req.Desc != nil {
		desc = *req.Desc
	}

	err := model.UpdateGroupGlobal(req.OldGroup, req.NewGroup, ratio, desc)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	recordManageAudit(c, "group.update", map[string]any{
		"old_group": req.OldGroup,
		"new_group": req.NewGroup,
		"ratio":     ratio,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "分组配置已更新并全局同步",
		"data": gin.H{
			"old_group": req.OldGroup,
			"new_group": req.NewGroup,
			"ratio":     ratio,
		},
	})
}

