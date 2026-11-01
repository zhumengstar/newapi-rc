package model

import (
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func IsChannelEnabledForGroupModel(group string, modelName string, channelID int) bool {
	return IsChannelEnabledForUserGroupModel(group, modelName, channelID, 0)
}

func IsChannelEnabledForUserGroupModel(group string, modelName string, channelID int, userId int) bool {
	if group == "" || modelName == "" || channelID <= 0 {
		return false
	}
	if !common.MemoryCacheEnabled {
		return isChannelEnabledForGroupModelDB(group, modelName, channelID)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// If userId is restricted in this group, check the user's dedicated pool
	if userId > 0 && group2userRestricted != nil && group2userRestricted[group] != nil && group2userRestricted[group][userId] {
		if group2userModel2channels == nil || group2userModel2channels[group] == nil || group2userModel2channels[group][userId] == nil {
			return false
		}
		if isChannelIDInList(group2userModel2channels[group][userId][modelName], channelID) {
			return true
		}
		normalized := ratio_setting.RoutingMatchModelName(modelName)
		if normalized != "" && normalized != modelName {
			return isChannelIDInList(group2userModel2channels[group][userId][normalized], channelID)
		}
		return false
	}

	if group2model2channels == nil {
		return false
	}

	if isChannelIDInList(group2model2channels[group][modelName], channelID) {
		return true
	}
	normalized := ratio_setting.RoutingMatchModelName(modelName)
	if normalized != "" && normalized != modelName {
		return isChannelIDInList(group2model2channels[group][normalized], channelID)
	}
	return false
}

func IsChannelEnabledForAnyGroupModel(groups []string, modelName string, channelID int) bool {
	if len(groups) == 0 {
		return false
	}
	for _, g := range groups {
		if IsChannelEnabledForGroupModel(g, modelName, channelID) {
			return true
		}
	}
	return false
}

func isChannelEnabledForGroupModelDB(group string, modelName string, channelID int) bool {
	var count int64
	err := DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ? and enabled = ?", group, modelName, channelID, true).
		Count(&count).Error
	if err == nil && count > 0 {
		return true
	}
	normalized := ratio_setting.RoutingMatchModelName(modelName)
	if normalized == "" || normalized == modelName {
		return false
	}
	count = 0
	err = DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ? and enabled = ?", group, normalized, channelID, true).
		Count(&count).Error
	return err == nil && count > 0
}

func isChannelIDInList(list []int, channelID int) bool {
	return slices.Contains(list, channelID)
}

func HasHigherPriorityEnabledChannelForUserGroupModel(group string, modelName string, channelID int, userId int) bool {
	if group == "" || modelName == "" || channelID <= 0 || !common.MemoryCacheEnabled {
		return false
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	targetCh, ok := channelsIDM[channelID]
	if !ok || targetCh == nil {
		return false
	}
	targetPrio := targetCh.GetRoutingPriority()

	var list []int
	if userId > 0 && group2userRestricted != nil && group2userRestricted[group] != nil && group2userRestricted[group][userId] {
		if group2userModel2channels != nil && group2userModel2channels[group] != nil && group2userModel2channels[group][userId] != nil {
			list = group2userModel2channels[group][userId][modelName]
			if len(list) == 0 {
				normalized := ratio_setting.RoutingMatchModelName(modelName)
				if normalized != "" && normalized != modelName {
					list = group2userModel2channels[group][userId][normalized]
				}
			}
		}
	} else if group2model2channels != nil && group2model2channels[group] != nil {
		list = group2model2channels[group][modelName]
		if len(list) == 0 {
			normalized := ratio_setting.RoutingMatchModelName(modelName)
			if normalized != "" && normalized != modelName {
				list = group2model2channels[group][normalized]
			}
		}
	}

	for _, cid := range list {
		if cid == channelID {
			continue
		}
		if ch, exists := channelsIDM[cid]; exists && ch != nil && ch.Status == common.ChannelStatusEnabled {
			if ch.GetRoutingPriority() > targetPrio {
				return true
			}
		}
	}
	return false
}
