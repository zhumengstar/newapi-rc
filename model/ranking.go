package model

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

var (
	rankedGroupsCache    []string
	rankedGroupsQuotaMap map[string]int64
	rankedGroupsCacheMu  sync.RWMutex
	rankedGroupsCacheExp time.Time
	rankedGroupsUpdating int32

	rankedModelsCache    []string
	rankedModelsCacheMu  sync.RWMutex
	rankedModelsCacheExp time.Time
	rankedModelsUpdating int32
)

const rankingUpdateInterval = 3 * time.Minute

type groupQuotaRow struct {
	Group string `gorm:"column:group"`
	Quota int64  `gorm:"column:total_quota"`
}

type modelCountRow struct {
	Model string `gorm:"column:model_name"`
	Count int64  `gorm:"column:request_count"`
}

// SortGroupsSliceByQuotaAndRatio 优先按当天消费额降序，其次按分组倍率降序，最后按名称升序
func SortGroupsSliceByQuotaAndRatio(groups []string, quotaMap map[string]int64, ratioMap map[string]float64) {
	sort.Slice(groups, func(i, j int) bool {
		gI, gJ := groups[i], groups[j]
		var qI, qJ int64
		if quotaMap != nil {
			qI = quotaMap[gI]
			qJ = quotaMap[gJ]
		}
		if qI != qJ {
			return qI > qJ
		}
		if ratioMap != nil {
			rI, hasI := ratioMap[gI]
			rJ, hasJ := ratioMap[gJ]
			if hasI && hasJ && rI != rJ {
				return rI > rJ
			}
			if hasI != hasJ {
				return hasI
			}
		}
		return gI < gJ
	})
}

func calculateRankedGroups() ([]string, map[string]int64, error) {
	todayStart := shanghaiTodayStartUnix()
	configuredGroups := ratio_setting.GetGroupRatioCopy()

	var quotaDB *gorm.DB = LOG_DB
	if quotaDB == nil {
		quotaDB = DB
	}
	if quotaDB == nil {
		return getDefaultGroups(), map[string]int64{}, nil
	}

	var rows []groupQuotaRow
	col := logGroupCol
	if col == "" {
		col = "\"group\""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := quotaDB.WithContext(ctx).Table("logs").
		Select(col + ", COALESCE(SUM(quota), 0) AS total_quota").
		Where("type = ? AND created_at >= ? AND "+col+" != ''", LogTypeConsume, todayStart).
		Group(col).
		Having("SUM(quota) > 0").
		Order("total_quota DESC").
		Scan(&rows).Error
	if err != nil {
		common.SysError("failed to calculate ranked groups from logs: " + err.Error())
		return getDefaultGroups(), map[string]int64{}, err
	}

	groupQuotaMap := make(map[string]int64, len(rows)+32)
	for _, row := range rows {
		trimmed := strings.TrimSpace(row.Group)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, ",") {
			parts := strings.Split(trimmed, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					groupQuotaMap[part] += row.Quota
				}
			}
		} else {
			groupQuotaMap[trimmed] += row.Quota
		}
	}

	allGroupsSet := make(map[string]struct{}, len(configuredGroups)+len(groupQuotaMap))
	for g := range configuredGroups {
		if g != "" {
			allGroupsSet[g] = struct{}{}
		}
	}
	for g := range groupQuotaMap {
		if g != "" {
			allGroupsSet[g] = struct{}{}
		}
	}

	allGroups := make([]string, 0, len(allGroupsSet))
	for g := range allGroupsSet {
		allGroups = append(allGroups, g)
	}

	SortGroupsSliceByQuotaAndRatio(allGroups, groupQuotaMap, configuredGroups)

	return allGroups, groupQuotaMap, nil
}

func calculateRankedModels() ([]string, error) {
	todayStart := shanghaiTodayStartUnix()

	var quotaDB *gorm.DB = LOG_DB
	if quotaDB == nil {
		quotaDB = DB
	}
	if quotaDB == nil {
		return GetAllChannelModels(), nil
	}

	var rows []modelCountRow
	err := quotaDB.Table("logs").
		Select("model_name, COUNT(*) AS request_count").
		Where("created_at >= ? AND model_name != ''", todayStart).
		Group("model_name").
		Having("COUNT(*) > 0").
		Order("request_count DESC").
		Scan(&rows).Error
	if err != nil {
		common.SysError("failed to calculate ranked models from logs: " + err.Error())
		return GetAllChannelModels(), err
	}

	modelCountMap := make(map[string]int64, len(rows)+32)
	for _, row := range rows {
		trimmed := strings.TrimSpace(row.Model)
		if trimmed != "" {
			modelCountMap[trimmed] += row.Count
		}
	}

	configuredModels := GetAllChannelModels()
	allModelsSet := make(map[string]struct{}, len(configuredModels)+len(modelCountMap))
	for _, m := range configuredModels {
		if m != "" {
			allModelsSet[m] = struct{}{}
		}
	}
	for m := range modelCountMap {
		if m != "" {
			allModelsSet[m] = struct{}{}
		}
	}

	allModels := make([]string, 0, len(allModelsSet))
	for m := range allModelsSet {
		allModels = append(allModels, m)
	}

	// 排序规则：
	// 1. 优先按当前请求调用量降序（调用量大的置顶）；
	// 2. 调用量相同时（包括为 0），按模型名升序，保持稳定排列。
	sort.Slice(allModels, func(i, j int) bool {
		cI := modelCountMap[allModels[i]]
		cJ := modelCountMap[allModels[j]]
		if cI != cJ {
			return cI > cJ
		}
		return allModels[i] < allModels[j]
	})

	return allModels, nil
}

func getDefaultGroups() []string {
	configuredGroups := ratio_setting.GetGroupRatioCopy()
	groups := make([]string, 0, len(configuredGroups))
	for g := range configuredGroups {
		if g != "" {
			groups = append(groups, g)
		}
	}
	SortGroupsSliceByQuotaAndRatio(groups, nil, configuredGroups)
	return groups
}

// RefreshRankedGroups 手动或定时强制刷新分组排序缓存
func RefreshRankedGroups() []string {
	ranked, quotaMap, err := calculateRankedGroups()
	if err != nil || len(ranked) == 0 {
		ranked = getDefaultGroups()
		quotaMap = map[string]int64{}
	}

	rankedGroupsCacheMu.Lock()
	rankedGroupsCache = ranked
	rankedGroupsQuotaMap = quotaMap
	rankedGroupsCacheExp = time.Now().Add(rankingUpdateInterval)
	rankedGroupsCacheMu.Unlock()

	return ranked
}

// RefreshRankedModels 手动或定时强制刷新模型排序缓存
func RefreshRankedModels() []string {
	ranked, err := calculateRankedModels()
	if err != nil || len(ranked) == 0 {
		ranked = GetAllChannelModels()
	}

	rankedModelsCacheMu.Lock()
	rankedModelsCache = ranked
	rankedModelsCacheExp = time.Now().Add(rankingUpdateInterval)
	rankedModelsCacheMu.Unlock()

	return ranked
}

// GetRankedGroupsWithQuotas 获取按照当天消费金额优先、倍率次之排序的分组列表及今日消费额度
func GetRankedGroupsWithQuotas() ([]string, map[string]int64) {
	now := time.Now()
	rankedGroupsCacheMu.RLock()
	if rankedGroupsCache != nil && now.Before(rankedGroupsCacheExp) {
		res := make([]string, len(rankedGroupsCache))
		copy(res, rankedGroupsCache)
		qMap := make(map[string]int64, len(rankedGroupsQuotaMap))
		for k, v := range rankedGroupsQuotaMap {
			qMap[k] = v
		}
		rankedGroupsCacheMu.RUnlock()
		return res, qMap
	}
	rankedGroupsCacheMu.RUnlock()

	rankedGroupsCacheMu.RLock()
	isNil := rankedGroupsCache == nil
	rankedGroupsCacheMu.RUnlock()

	if isNil {
		RefreshRankedGroups()
	} else if atomic.CompareAndSwapInt32(&rankedGroupsUpdating, 0, 1) {
		go func() {
			defer atomic.StoreInt32(&rankedGroupsUpdating, 0)
			RefreshRankedGroups()
		}()
	}

	rankedGroupsCacheMu.RLock()
	res := make([]string, len(rankedGroupsCache))
	copy(res, rankedGroupsCache)
	qMap := make(map[string]int64, len(rankedGroupsQuotaMap))
	for k, v := range rankedGroupsQuotaMap {
		qMap[k] = v
	}
	rankedGroupsCacheMu.RUnlock()
	return res, qMap
}

// GetRankedGroups 获取按照当天消费金额优先、倍率次之排序的分组列表
func GetRankedGroups() []string {
	res, _ := GetRankedGroupsWithQuotas()
	return res
}

// GetRankedModels 获取按照请求量排序的模型列表（每2小时更新一次）
func GetRankedModels() []string {
	now := time.Now()
	rankedModelsCacheMu.RLock()
	if rankedModelsCache != nil && now.Before(rankedModelsCacheExp) {
		res := make([]string, len(rankedModelsCache))
		copy(res, rankedModelsCache)
		rankedModelsCacheMu.RUnlock()
		return res
	}
	rankedModelsCacheMu.RUnlock()

	rankedModelsCacheMu.RLock()
	isNil := rankedModelsCache == nil
	rankedModelsCacheMu.RUnlock()

	if isNil {
		return RefreshRankedModels()
	}

	if atomic.CompareAndSwapInt32(&rankedModelsUpdating, 0, 1) {
		go func() {
			defer atomic.StoreInt32(&rankedModelsUpdating, 0)
			RefreshRankedModels()
		}()
	}

	rankedModelsCacheMu.RLock()
	res := make([]string, len(rankedModelsCache))
	copy(res, rankedModelsCache)
	rankedModelsCacheMu.RUnlock()
	return res
}

// InitRankingCron 启动每两小时一次的后台定时刷新协程
func InitRankingCron() {
	go func() {
		// 启动后稍作延迟预热执行一次
		time.Sleep(5 * time.Second)
		RefreshRankedGroups()
		RefreshRankedModels()

		ticker := time.NewTicker(rankingUpdateInterval)
		defer ticker.Stop()
		for range ticker.C {
			RefreshRankedGroups()
			RefreshRankedModels()
		}
	}()
}
