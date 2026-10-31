package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var group2model2channels map[string]map[string][]int // enabled channel
var channelsIDM map[int]*Channel                     // all channels include disabled
// group2userRestricted tracks whether a user has designated/restricted channels in group.
// group -> userId -> bool
var group2userRestricted map[string]map[int]bool
// group2userAllowedChannels tracks which channels a restricted user is allowed to access in group.
// group -> userId -> map[channelId]bool
var group2userAllowedChannels map[string]map[int]map[int]bool
// group2userModel2channels tracks dedicated model channels for restricted users in group (regardless of channel status).
// group -> userId -> model -> sorted []channelId
var group2userModel2channels map[string]map[int]map[string][]int
// groupRestrictedChannels tracks all channel IDs in group that have AllowedUserIds configured.
// group -> map[channelId]bool
var groupRestrictedChannels map[string]map[int]bool
// channel2advancedCustomConfig caches parsed Advanced Custom (type 58) configs so
// path-aware selection avoids re-parsing JSON per request. Refreshed on full sync.
var channel2advancedCustomConfig map[int]*kitdto.AdvancedCustomConfig
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		InvalidatePricingCache()
		rebuildTaskAliasView()
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	newChannel2advancedCustomConfig := make(map[int]*kitdto.AdvancedCustomConfig)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
		if channel.Type == constant.ChannelTypeAdvancedCustom {
			if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
				newChannel2advancedCustomConfig[channel.Id] = config
			}
		}
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	newGroup2userRestricted := make(map[string]map[int]bool)
	newGroup2userAllowedChannels := make(map[string]map[int]map[int]bool)
	newGroup2userModel2channels := make(map[string]map[int]map[string][]int)
	newGroupRestrictedChannels := make(map[string]map[int]bool)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
		newGroup2userRestricted[group] = make(map[int]bool)
		newGroup2userAllowedChannels[group] = make(map[int]map[int]bool)
		newGroup2userModel2channels[group] = make(map[int]map[string][]int)
		newGroupRestrictedChannels[group] = make(map[int]bool)
	}
	for _, channel := range channels {
		allowedUsers := channel.GetOtherSettings().AllowedUserIds
		groups := strings.SplitSeq(channel.Group, ",")
		for group := range groups {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}
			// 1. 公共渠道池：启用状态的渠道均进入公共池供普通客户正常使用
			if channel.Status == common.ChannelStatusEnabled {
				if newGroup2model2channels[group] == nil {
					newGroup2model2channels[group] = make(map[string][]int)
				}
				models := channel.GetModels()
				for _, model := range models {
					if _, ok := newGroup2model2channels[group][model]; !ok {
						newGroup2model2channels[group][model] = make([]int, 0)
					}
					newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
				}
			}

			// 2. 指定受限用户池：配置了专属指定用户的渠道，无论是否启用，均建立受限映射与专属池
			if len(allowedUsers) > 0 {
				if newGroup2userRestricted[group] == nil {
					newGroup2userRestricted[group] = make(map[int]bool)
					newGroup2userAllowedChannels[group] = make(map[int]map[int]bool)
					newGroup2userModel2channels[group] = make(map[int]map[string][]int)
					newGroupRestrictedChannels[group] = make(map[int]bool)
				}
				newGroupRestrictedChannels[group][channel.Id] = true
				models := channel.GetModels()
				for _, uid := range allowedUsers {
					if uid > 0 {
						newGroup2userRestricted[group][uid] = true
						if newGroup2userAllowedChannels[group][uid] == nil {
							newGroup2userAllowedChannels[group][uid] = make(map[int]bool)
							newGroup2userModel2channels[group][uid] = make(map[string][]int)
						}
						newGroup2userAllowedChannels[group][uid][channel.Id] = true

						// 指定后，无论是否禁用，都只能走指定的渠道
						// 将该专属渠道的模型加入到该受限用户的专属渠道池中
						for _, model := range models {
							newGroup2userModel2channels[group][uid][model] = append(newGroup2userModel2channels[group][uid][model], channel.Id)
						}
					}
				}
			}
		}
	}

	// sort public channels by routing priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetRoutingPriority() > newChannelId2channel[channels[j]].GetRoutingPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	// sort restricted user channels by routing priority
	for group, user2model2channels := range newGroup2userModel2channels {
		for uid, model2channels := range user2model2channels {
			for model, channels := range model2channels {
				sort.Slice(channels, func(i, j int) bool {
					return newChannelId2channel[channels[i]].GetRoutingPriority() > newChannelId2channel[channels[j]].GetRoutingPriority()
				})
				newGroup2userModel2channels[group][uid][model] = channels
			}
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	group2userRestricted = newGroup2userRestricted
	group2userAllowedChannels = newGroup2userAllowedChannels
	group2userModel2channels = newGroup2userModel2channels
	groupRestrictedChannels = newGroupRestrictedChannels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channel2advancedCustomConfig = newChannel2advancedCustomConfig
	channelSyncLock.Unlock()
	// Lock ordering: InvalidatePricingCache acquires updatePricingLock, and
	// GetPricing (holding updatePricingLock) nests channelSyncLock.RLock via
	// loadPricingAdvancedCustomConfigs. channelSyncLock MUST be released before
	// invalidating the pricing cache, otherwise the reversed order deadlocks.
	InvalidatePricingCache()
	rebuildTaskAliasView()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

// IsUserRestrictedInGroup returns true if userId is configured on any channels in group.
func IsUserRestrictedInGroup(group string, userId int) bool {
	if userId <= 0 {
		return false
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	if group2userRestricted == nil || group2userRestricted[group] == nil {
		return false
	}
	return group2userRestricted[group][userId]
}

// GetUserAllowedChannelsInGroup returns a map of channel IDs allowed for userId in group.
func GetUserAllowedChannelsInGroup(group string, userId int) map[int]bool {
	if userId <= 0 {
		return nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	if group2userAllowedChannels == nil || group2userAllowedChannels[group] == nil {
		return nil
	}
	userMap := group2userAllowedChannels[group][userId]
	if len(userMap) == 0 {
		return nil
	}
	res := make(map[int]bool, len(userMap))
	for k, v := range userMap {
		res[k] = v
	}
	return res
}


// IsUserAllowedChannelInGroup returns true if channelId is in userId's allowed channel set in group.
func IsUserAllowedChannelInGroup(group string, userId int, channelId int) bool {
	if userId <= 0 || channelId <= 0 {
		return false
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	if group2userAllowedChannels == nil || group2userAllowedChannels[group] == nil || group2userAllowedChannels[group][userId] == nil {
		return false
	}
	return group2userAllowedChannels[group][userId][channelId]
}

// IsChannelRestrictedInGroup returns true if the channel has AllowedUserIds configured in group.
func IsChannelRestrictedInGroup(group string, channelId int) bool {
	if channelId <= 0 {
		return false
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	if groupRestrictedChannels == nil || groupRestrictedChannels[group] == nil {
		return false
	}
	return groupRestrictedChannels[group][channelId]
}

func GetRandomSatisfiedChannel(
	group string,
	model string,
	retry int,
	filters []dto.ChannelFilter,
) (*Channel, error) {
	return GetRandomSatisfiedChannelForUserExcluding(group, model, 0, retry, filters, nil)
}

// GetRandomSatisfiedChannelExcluding is the cache-backed counterpart of
// GetChannelExcluding. It keeps priority and weight selection consistent for
// retries while removing channels already attempted by this request.
func GetRandomSatisfiedChannelExcluding(
	group string,
	model string,
	retry int,
	filters []dto.ChannelFilter,
	excludedChannelIds map[int]bool,
) (*Channel, error) {
	return GetRandomSatisfiedChannelForUserExcluding(group, model, 0, retry, filters, excludedChannelIds)
}

// GetRandomSatisfiedChannelForUserExcluding selects a channel considering user restriction:
// - Restricted users ONLY pick from their dedicated channel pool (even if disabled).
// - Unrestricted users ONLY pick from the public enabled pool (never touching dedicated channels).
func GetRandomSatisfiedChannelForUserExcluding(
	group string,
	model string,
	userId int,
	retry int,
	filters []dto.ChannelFilter,
	excludedChannelIds map[int]bool,
) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannelExcluding(group, model, retry, filters, excludedChannelIds)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	var candidatePool []int
	isRestricted := userId > 0 && group2userRestricted != nil && group2userRestricted[group] != nil && group2userRestricted[group][userId]

	if isRestricted {
		if group2userModel2channels != nil && group2userModel2channels[group] != nil && group2userModel2channels[group][userId] != nil {
			candidatePool = group2userModel2channels[group][userId][model]
			if len(candidatePool) == 0 {
				normalizedModel := ratio_setting.RoutingMatchModelName(model)
				if normalizedModel != "" && normalizedModel != model {
					candidatePool = group2userModel2channels[group][userId][normalizedModel]
				}
			}
		}
		// 严禁逃逸：受限用户若专属池无该模型，绝对不走公共渠道，直接返回 nil
		if len(candidatePool) == 0 {
			return nil, nil
		}
	} else {
		if group2model2channels != nil && group2model2channels[group] != nil {
			candidatePool = group2model2channels[group][model]
			if len(candidatePool) == 0 {
				normalizedModel := ratio_setting.RoutingMatchModelName(model)
				if normalizedModel != "" && normalizedModel != model {
					candidatePool = group2model2channels[group][normalizedModel]
				}
			}
		}
		if len(candidatePool) == 0 {
			return nil, nil
		}
	}

	// First, filter candidates by constraints
	channels, _ := filterCandidateIDs(candidatePool, model, filters)
	channels = filterChannelsByExcludedChannelIds(channels, excludedChannelIds)

	if len(channels) == 0 {
		return nil, nil
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetRoutingPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	// get the priority for the given retry number
	var sumWeight = 0
	var targetChannels []*Channel
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetRoutingPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority))
	}
	if len(excludedChannelIds) > 0 {
		// A retry must be predictable: after excluding failed channels, try
		// the highest remaining enabled weight in the current priority.
		// Keep the first request on the existing weighted-random path.
		sort.SliceStable(targetChannels, func(i, j int) bool {
			if targetChannels[i].GetWeight() != targetChannels[j].GetWeight() {
				return targetChannels[i].GetWeight() > targetChannels[j].GetWeight()
			}
			return targetChannels[i].Id < targetChannels[j].Id
		})
		return targetChannels[0], nil
	}

	allZeroWeights := sumWeight == 0
	if allZeroWeights {
		// All-zero weights retain the historical equal-share behavior.
		sumWeight = len(targetChannels)
	}

	randomWeight := rand.Intn(sumWeight)

	// Find a channel based on its weight
	for _, channel := range targetChannels {
		weight := channel.GetWeight()
		if allZeroWeights {
			weight = 1
		}
		randomWeight -= weight
		if randomWeight < 0 {
			return channel, nil
		}
	}
	// return null if no channel is not found
	return nil, errors.New("channel not found")
}

func filterChannelsByExcludedChannelIds(channelIds []int, excludedChannelIds map[int]bool) []int {
	if len(excludedChannelIds) == 0 {
		return channelIds
	}
	filtered := make([]int, 0, len(channelIds))
	for _, channelId := range channelIds {
		if !excludedChannelIds[channelId] {
			filtered = append(filtered, channelId)
		}
	}
	return filtered
}

// BuildEffectiveRetryExcludedChannelIds 根据渠道已尝试次数与优先级计算当前重试应排除的渠道集合：
// 1. 同优先级存在多个不同渠道（len >= 2）时，在降级到下一优先级前支持同优先级多轮交替重试（每周期 2 轮）；
// 2. 所有优先级完成一轮尝试后，若仍未达到最大重试次数（allowFullWrap=true），自动开启下一周期从最高优先级继续轮询重试；
// 3. 同一优先级存在多个候选时，自动规避刚失败的 lastTriedId，防止连续两次打到同一渠道。
func BuildEffectiveRetryExcludedChannelIds(
	group string,
	modelName string,
	userId int,
	filters []dto.ChannelFilter,
	hardExcluded map[int]bool,
	triedCounts map[int]int,
	lastTriedId int,
	allowMultiPass bool,
	allowFullWrap bool,
) map[int]bool {
	mergeAllTried := func() map[int]bool {
		if len(hardExcluded) == 0 && len(triedCounts) == 0 {
			return nil
		}
		res := make(map[int]bool, len(hardExcluded)+len(triedCounts))
		for id, v := range hardExcluded {
			if v {
				res[id] = true
			}
		}
		for id, cnt := range triedCounts {
			if cnt > 0 {
				res[id] = true
			}
		}
		return res
	}

	if len(triedCounts) == 0 {
		return hardExcluded
	}
	if !allowMultiPass {
		return mergeAllTried()
	}

	type candidateMeta struct {
		id       int
		priority int64
		enabled  bool
	}

	var candidates []candidateMeta
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		var candidatePool []int
		isRestricted := userId > 0 && group2userRestricted != nil && group2userRestricted[group] != nil && group2userRestricted[group][userId]
		if isRestricted {
			if group2userModel2channels != nil && group2userModel2channels[group] != nil && group2userModel2channels[group][userId] != nil {
				candidatePool = group2userModel2channels[group][userId][modelName]
				if len(candidatePool) == 0 {
					normalizedModel := ratio_setting.RoutingMatchModelName(modelName)
					if normalizedModel != "" && normalizedModel != modelName {
						candidatePool = group2userModel2channels[group][userId][normalizedModel]
					}
				}
			}
		} else {
			if group2model2channels != nil && group2model2channels[group] != nil {
				candidatePool = group2model2channels[group][modelName]
				if len(candidatePool) == 0 {
					normalizedModel := ratio_setting.RoutingMatchModelName(modelName)
					if normalizedModel != "" && normalizedModel != modelName {
						candidatePool = group2model2channels[group][normalizedModel]
					}
				}
			}
		}
		filteredIDs, _ := filterCandidateIDs(candidatePool, modelName, filters)
		filteredIDs = filterChannelsByExcludedChannelIds(filteredIDs, hardExcluded)
		for _, cid := range filteredIDs {
			if ch, ok := channelsIDM[cid]; ok && ch != nil {
				candidates = append(candidates, candidateMeta{
					id:       cid,
					priority: ch.GetRoutingPriority(),
					enabled:  ch.Status == common.ChannelStatusEnabled,
				})
			}
		}
		channelSyncLock.RUnlock()
	} else {
		var abilities []Ability
		if err := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, modelName, true).Order("priority DESC, weight DESC").Find(&abilities).Error; err == nil {
			abilities = filterAbilitiesByConstraints(abilities, modelName, filters)
			abilities = applyChannelRoutingPriorities(abilities)
			for _, ab := range abilities {
				if hardExcluded != nil && hardExcluded[ab.ChannelId] {
					continue
				}
				prio := int64(0)
				if ab.Priority != nil {
					prio = *ab.Priority
				}
				candidates = append(candidates, candidateMeta{
					id:       ab.ChannelId,
					priority: prio,
					enabled:  ab.Enabled,
				})
			}
		}
	}

	// 过滤掉已禁用且已尝试过的渠道（禁用渠道不允许二次重试）
	validCandidates := make([]candidateMeta, 0, len(candidates))
	for _, c := range candidates {
		if !c.enabled && triedCounts[c.id] > 0 {
			continue
		}
		validCandidates = append(validCandidates, c)
	}
	if len(validCandidates) == 0 {
		return mergeAllTried()
	}

	prioMap := make(map[int64][]int)
	var sortedPriorities []int64
	for _, c := range validCandidates {
		if _, exists := prioMap[c.priority]; !exists {
			sortedPriorities = append(sortedPriorities, c.priority)
		}
		prioMap[c.priority] = append(prioMap[c.priority], c.id)
	}
	sort.Slice(sortedPriorities, func(i, j int) bool {
		return sortedPriorities[i] > sortedPriorities[j]
	})

	maxCycles := 1
	if allowFullWrap {
		maxCycles = 64
	}

	for cycle := 1; cycle <= maxCycles; cycle++ {
		for _, prio := range sortedPriorities {
			tierIds := prioMap[prio]
			baseRounds := 1
			if len(tierIds) >= 2 {
				// 同优先级有多个不同渠道时，支持在同优先级内多轮重试（2轮）后再降级
				baseRounds = 2
			}
			maxTries := cycle * baseRounds

			minTries := int(^uint(0) >> 1)
			for _, cid := range tierIds {
				t := triedCounts[cid]
				if t < minTries {
					minTries = t
				}
			}

			if minTries < maxTries {
				eligible := make([]int, 0, len(tierIds))
				for _, cid := range tierIds {
					if triedCounts[cid] == minTries {
						eligible = append(eligible, cid)
					}
				}
				if len(eligible) > 1 && lastTriedId > 0 {
					withoutLast := make([]int, 0, len(eligible))
					for _, cid := range eligible {
						if cid != lastTriedId {
							withoutLast = append(withoutLast, cid)
						}
					}
					if len(withoutLast) > 0 {
						eligible = withoutLast
					}
				}

				eligibleSet := make(map[int]bool, len(eligible))
				for _, cid := range eligible {
					eligibleSet[cid] = true
				}

				effectiveExcluded := make(map[int]bool, len(hardExcluded)+len(validCandidates)+len(triedCounts))
				for id, v := range hardExcluded {
					if v {
						effectiveExcluded[id] = true
					}
				}
				for _, c := range validCandidates {
					if !eligibleSet[c.id] {
						effectiveExcluded[c.id] = true
					}
				}
				for id, cnt := range triedCounts {
					if cnt > 0 && !eligibleSet[id] {
						effectiveExcluded[id] = true
					}
				}
				return effectiveExcluded
			}
		}
	}

	return mergeAllTried()
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// delete the channel from group2model2channels
		for group, model2channels := range group2model2channels {
			for model, channels := range model2channels {
				for i, channelId := range channels {
					if channelId == id {
						// remove the channel from the slice
						group2model2channels[group][model] = append(channels[:i], channels[i+1:]...)
						break
					}
				}
			}
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	if channel == nil {
		channelSyncLock.Unlock()
		return
	}

	if channelsIDM == nil {
		channelsIDM = make(map[int]*Channel)
	}
	if oldChannel, ok := channelsIDM[channel.Id]; ok {
		logger.LogDebug(nil, "CacheUpdateChannel before: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, oldChannel.ChannelInfo.MultiKeyPollingIndex)
	}
	channelsIDM[channel.Id] = channel
	if channel2advancedCustomConfig == nil {
		channel2advancedCustomConfig = make(map[int]*kitdto.AdvancedCustomConfig)
	}
	delete(channel2advancedCustomConfig, channel.Id)
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
			channel2advancedCustomConfig[channel.Id] = config
		}
	}
	logger.LogDebug(nil, "CacheUpdateChannel after: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)
	// Lock ordering: do NOT hold channelSyncLock while calling
	// InvalidatePricingCache. GetPricing acquires updatePricingLock first and then
	// channelSyncLock.RLock (via loadPricingAdvancedCustomConfigs); acquiring
	// updatePricingLock while holding channelSyncLock would be an AB-BA deadlock.
	channelSyncLock.Unlock()
	InvalidatePricingCache()
}
