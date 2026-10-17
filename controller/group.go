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

func GetGroups(c *gin.Context) {
	groupNames := model.GetRankedGroups()
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
	groupNames := model.GetRankedGroups()
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
