/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

// UpdateGroupGlobal 全局修改分组名称及默认模型倍率
// 当 oldGroup != newGroup 时：
// 1. 同步更新 channels 表中包含该分组的渠道 (comma-separated Group 字段)
// 2. 同步更新 abilities 表中对应的 abilities.group
// 3. 同步更新 users 表中的 user.group 以及 user.setting (包含 user_group_ratios, user_model_price_rules, disabled_groups, user_exclusive_channels, user_group_channels)
// 4. 清理受影响用户的 Redis 缓存
// 5. 同步更新 tokens 表中的 token.group 和 token.auto_groups
// 6. 同步更新相关联的 user_group_memberships, channel_control_policies, channel_control_metrics, user_pricing_rules 等表
// 7. 更新全局 GroupRatio, UserUsableGroups, GroupGroupRatio, TopupGroupRatio 等配置
// 8. 刷新渠道缓存与分组排序缓存
func UpdateGroupGlobal(oldGroup, newGroup string, ratio float64, desc string) error {
	oldGroup = strings.TrimSpace(oldGroup)
	newGroup = strings.TrimSpace(newGroup)

	if oldGroup == "" {
		return errors.New("原分组名称不能为空")
	}
	if newGroup == "" {
		return errors.New("新分组名称不能为空")
	}
	if oldGroup == "auto" || newGroup == "auto" {
		return errors.New("不能编辑系统内置的 auto 分组")
	}
	if ratio < 0 {
		return errors.New("分组倍率不能小于 0")
	}

	isRename := oldGroup != newGroup

	if isRename {
		// 检查目标分组是否已存在，防止误合并冲突
		if ratio_setting.ContainsGroupRatio(newGroup) {
			return errors.New("目标分组名称已存在，请输入一个未占用的新名称")
		}
		var channelCount int64
		_ = DB.Model(&Channel{}).Where(commonGroupCol+" = ? OR "+commonGroupCol+" LIKE ? OR "+commonGroupCol+" LIKE ? OR "+commonGroupCol+" LIKE ?",
			newGroup, newGroup+",%", "%,"+newGroup, "%,"+newGroup+",%").Count(&channelCount).Error
		if channelCount > 0 {
			return errors.New("目标分组名称已在渠道中存在，请输入一个未占用的新名称")
		}
	}

	if isRename {
		err := DB.Transaction(func(tx *gorm.DB) error {
			// A. Channels 表: 更新逗号分隔的 group 字段
			var channels []Channel
			if err := tx.Find(&channels).Error; err != nil {
				return err
			}
			for _, ch := range channels {
				parts := strings.Split(ch.Group, ",")
				changed := false
				for i, p := range parts {
					if strings.TrimSpace(p) == oldGroup {
						parts[i] = newGroup
						changed = true
					}
				}
				if changed {
					newGroupStr := strings.Join(parts, ",")
					if err := tx.Model(&Channel{}).Where("id = ?", ch.Id).Update(commonGroupCol, newGroupStr).Error; err != nil {
						return err
					}
				}
			}

			// B. Abilities 表: 精准匹配 group
			if err := tx.Table("abilities").Where(commonGroupCol+" = ?", oldGroup).Update(commonGroupCol, newGroup).Error; err != nil {
				return err
			}

			// C. Users 表: 更新 user.group 逗号分隔字段与 user.setting JSON
			var users []User
			if err := tx.Where(commonGroupCol+" LIKE ? OR setting LIKE ?", "%"+oldGroup+"%", "%"+oldGroup+"%").Find(&users).Error; err != nil {
				return err
			}
			for _, u := range users {
				userChanged := false
				updates := map[string]any{}

				// 1. group 字段
				parts := strings.Split(u.Group, ",")
				groupChanged := false
				for i, p := range parts {
					if strings.TrimSpace(p) == oldGroup {
						parts[i] = newGroup
						groupChanged = true
					}
				}
				if groupChanged {
					updates[commonGroupCol] = strings.Join(parts, ",")
					userChanged = true
				}

				// 2. setting JSON 字段
				if u.Setting != "" && strings.Contains(u.Setting, oldGroup) {
					var settingMap map[string]any
					if err := json.Unmarshal([]byte(u.Setting), &settingMap); err == nil {
						settingChanged := false

						// user_group_ratios: { [group]: ratio }
						if ugr, ok := settingMap["user_group_ratios"].(map[string]any); ok {
							if val, exists := ugr[oldGroup]; exists {
								delete(ugr, oldGroup)
								ugr[newGroup] = val
								settingChanged = true
							}
						}

						// user_model_price_rules: [ { group, models, price } ]
						if rules, ok := settingMap["user_model_price_rules"].([]any); ok {
							for _, r := range rules {
								if ruleMap, ok := r.(map[string]any); ok {
									if ruleMap["group"] == oldGroup {
										ruleMap["group"] = newGroup
										settingChanged = true
									}
								}
							}
						}

						// disabled_groups: [ group1, group2 ]
						if dg, ok := settingMap["disabled_groups"].([]any); ok {
							for i, g := range dg {
								if strG, ok := g.(string); ok && strG == oldGroup {
									dg[i] = newGroup
									settingChanged = true
								}
							}
						}

						// user_exclusive_channels: { [group]: [channelIds] }
						if uec, ok := settingMap["user_exclusive_channels"].(map[string]any); ok {
							if val, exists := uec[oldGroup]; exists {
								delete(uec, oldGroup)
								uec[newGroup] = val
								settingChanged = true
							}
						}

						// user_group_channels: { [group]: [channelIds] }
						if ugc, ok := settingMap["user_group_channels"].(map[string]any); ok {
							if val, exists := ugc[oldGroup]; exists {
								delete(ugc, oldGroup)
								ugc[newGroup] = val
								settingChanged = true
							}
						}

						if settingChanged {
							newSettingBytes, err := json.Marshal(settingMap)
							if err == nil {
								updates["setting"] = string(newSettingBytes)
								userChanged = true
							}
						}
					}
				}

				if userChanged {
					if err := tx.Model(&User{}).Where("id = ?", u.Id).Updates(updates).Error; err != nil {
						return err
					}
				}
				_ = invalidateUserCache(u.Id)
			}

			// D. Tokens 表: group 与 auto_groups
			if err := tx.Model(&Token{}).Where(commonGroupCol+" = ?", oldGroup).Update(commonGroupCol, newGroup).Error; err != nil {
				return err
			}
			var tokens []Token
			if err := tx.Where("auto_groups LIKE ?", "%"+oldGroup+"%").Find(&tokens).Error; err == nil {
				for _, tok := range tokens {
					var ag []string
					if err := json.Unmarshal([]byte(tok.AutoGroups), &ag); err == nil {
						tokChanged := false
						for i, g := range ag {
							if g == oldGroup {
								ag[i] = newGroup
								tokChanged = true
							}
						}
						if tokChanged {
							newBytes, _ := json.Marshal(ag)
							_ = tx.Model(&Token{}).Where("id = ?", tok.Id).Update("auto_groups", string(newBytes)).Error
						}
					}
				}
			}

			// E. 其他可能存在分组引用的辅助表（若表已迁移创建）
			if tx.Migrator().HasTable("user_group_memberships") {
				_ = tx.Table("user_group_memberships").Where(commonGroupCol+" = ?", oldGroup).Update(commonGroupCol, newGroup).Error
			}
			if tx.Migrator().HasTable("channel_control_policies") {
				_ = tx.Table("channel_control_policies").Where(commonGroupCol+" = ?", oldGroup).Update(commonGroupCol, newGroup).Error
			}
			if tx.Migrator().HasTable("channel_control_metrics") {
				_ = tx.Table("channel_control_metrics").Where(commonGroupCol+" = ?", oldGroup).Update(commonGroupCol, newGroup).Error
			}
			if tx.Migrator().HasTable("user_pricing_rules") {
				_ = tx.Table("user_pricing_rules").Where("user_group = ?", oldGroup).Update("user_group", newGroup).Error
				_ = tx.Table("user_pricing_rules").Where("billing_group = ?", oldGroup).Update("billing_group", newGroup).Error
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	// 更新 GroupRatio 配置
	groupRatios := ratio_setting.GetGroupRatioCopy()
	if isRename {
		delete(groupRatios, oldGroup)
	}
	groupRatios[newGroup] = ratio
	groupRatioBytes, err := json.Marshal(groupRatios)
	if err != nil {
		return err
	}
	if err := UpdateOption("GroupRatio", string(groupRatioBytes)); err != nil {
		return err
	}

	// 更新 UserUsableGroups 配置 (描述信息及公共分组展示)
	userUsableGroups := setting.GetUserUsableGroupsCopy()
	oldDesc, hadOldDesc := userUsableGroups[oldGroup]
	finalDesc := desc
	if finalDesc == "" && hadOldDesc {
		finalDesc = oldDesc
	}
	if isRename {
		delete(userUsableGroups, oldGroup)
	}
	if finalDesc != "" || hadOldDesc {
		if finalDesc == "" {
			finalDesc = newGroup
		}
		userUsableGroups[newGroup] = finalDesc
		uugBytes, err := json.Marshal(userUsableGroups)
		if err == nil {
			_ = UpdateOption("UserUsableGroups", string(uugBytes))
		}
	}

	// 更新 GroupGroupRatio 配置
	if isRename {
		ggr := ratio_setting.GetGroupRatioSetting().GroupGroupRatio.ReadAll()
		ggrChanged := false
		if innerMap, exists := ggr[oldGroup]; exists {
			delete(ggr, oldGroup)
			ggr[newGroup] = innerMap
			ggrChanged = true
		}
		for _, innerMap := range ggr {
			if val, exists := innerMap[oldGroup]; exists {
				delete(innerMap, oldGroup)
				innerMap[newGroup] = val
				ggrChanged = true
			}
		}
		if ggrChanged {
			ggrBytes, err := json.Marshal(ggr)
			if err == nil {
				_ = UpdateOption("GroupGroupRatio", string(ggrBytes))
			}
		}

		// 更新 TopupGroupRatio 配置
		topupStr := common.TopupGroupRatio2JSONString()
		var topupMap map[string]float64
		if err := json.Unmarshal([]byte(topupStr), &topupMap); err == nil {
			if topRatio, exists := topupMap[oldGroup]; exists {
				delete(topupMap, oldGroup)
				topupMap[newGroup] = topRatio
				trBytes, _ := json.Marshal(topupMap)
				_ = UpdateOption("TopupGroupRatio", string(trBytes))
			}
		}
	}

	// 重新加载渠道缓存与排名
	InitChannelCache()
	RefreshRankedGroups()

	return nil
}
