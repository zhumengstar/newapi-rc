package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	isAdmin := c.GetInt("role") >= common.RoleAdminUser
	var targetGroups []string
	if isAdmin {
		targetGroups = append(lo.Keys(ratio_setting.GetGroupRatioCopy()), "auto")
	} else {
		var userGroup string
		if userId := c.GetInt("id"); userId > 0 {
			userGroup, _ = model.GetUserGroup(userId, false)
		}
		usableGroups := service.GetUserUsableGroups(userGroup)
		targetGroups = append(lo.Keys(usableGroups), "auto")
	}

	result, err := perfmetrics.QuerySummaryAll(hours, targetGroups)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetPerfMetrics(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	requestedGroup := c.Query("group")
	isAdmin := c.GetInt("role") >= common.RoleAdminUser
	var usableGroup map[string]string
	if !isAdmin {
		var userGroup string
		if userId := c.GetInt("id"); userId > 0 {
			userGroup, _ = model.GetUserGroup(userId, false)
		}
		usableGroup = service.GetUserUsableGroups(userGroup)
		if requestedGroup != "" && requestedGroup != "auto" {
			if _, ok := usableGroup[requestedGroup]; !ok {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"data": perfmetrics.QueryResult{
						ModelName: modelName,
						Groups:    []perfmetrics.GroupResult{},
					},
				})
				return
			}
		}
	}

	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model: modelName,
		Group: requestedGroup,
		Hours: hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	result.Groups = filterActiveGroups(result.Groups, usableGroup, isAdmin)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func filterActiveGroups(groups []perfmetrics.GroupResult, usableGroup map[string]string, isAdmin bool) []perfmetrics.GroupResult {
	activeRatios := ratio_setting.GetGroupRatioCopy()
	return lo.Filter(groups, func(g perfmetrics.GroupResult, _ int) bool {
		if !isAdmin && usableGroup != nil {
			if _, ok := usableGroup[g.Group]; !ok && g.Group != "auto" {
				return false
			}
		}
		_, ok := activeRatios[g.Group]
		return ok || g.Group == "auto"
	})
}
